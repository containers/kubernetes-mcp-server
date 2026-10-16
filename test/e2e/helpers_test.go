//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/distribution/reference"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/yaml"
)

const serverPort = 8080

type serverDeployment struct {
	name      string
	namespace string
	serverURL string
}

// deployOptions configures a deployServer call.
type deployOptions struct {
	configTOML  string
	extraValues map[string]any
	namespace   string
	// preInstall runs after the namespace is created but before Helm install,
	// so a test can create secrets/config the pod must mount on first start.
	preInstall func(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace string)
}

type deployOption func(*deployOptions)

// withConfig sets the server config TOML rendered into the Helm chart.
func withConfig(configTOML string) deployOption {
	return func(o *deployOptions) { o.configTOML = configTOML }
}

// withValues sets extra Helm values merged on top of the defaults.
func withValues(extraValues map[string]any) deployOption {
	return func(o *deployOptions) { o.extraValues = extraValues }
}

func withNamespace(namespace string) deployOption {
	return func(o *deployOptions) { o.namespace = namespace }
}

// withPreInstall registers a hook that runs after namespace creation and before
// Helm install (e.g. to create a CA secret the pod mounts on first start).
func withPreInstall(fn func(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace string)) deployOption {
	return func(o *deployOptions) { o.preInstall = fn }
}

// deployServer deploys the MCP server into a fresh namespace via Helm,
// starts a port-forward, and waits for the health endpoint.
// Cleanup is registered via t.Cleanup.
//
// The generated namespace is used as the Helm release name and fullnameOverride
// so that cluster-scoped resources (e.g. ClusterRoleBindings) are unique per run.
// The name argument is only a namespace prefix and log label.
func deployServer(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	name string,
	opts ...deployOption,
) *serverDeployment {
	t.Helper()

	var o deployOptions
	for _, opt := range opts {
		opt(&o)
	}

	kubeconfig := cfg.KubeconfigFile()
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err, "build rest config")

	clientset, err := kubernetes.NewForConfig(restCfg)
	require.NoError(t, err, "create clientset")

	var namespace string
	if o.namespace == "" {
		namespace = createTestNamespace(ctx, t, clientset, name)
	} else {
		ns, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   o.namespace,
				Labels: map[string]string{"app.kubernetes.io/managed-by": "e2e-test"},
			},
		}, metav1.CreateOptions{})
		require.NoError(t, err, "create namespace %s", o.namespace)
		namespace = ns.Name
	}
	t.Cleanup(func() {
		_ = clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	})
	t.Logf("[%s] Created namespace %s", name, namespace)

	// Release name == namespace keeps cluster-scoped resources unique per run.
	release := namespace

	if o.preInstall != nil {
		t.Logf("[%s] Running pre-install hook...", name)
		o.preInstall(ctx, t, clientset, namespace)
	}

	t.Logf("[%s] Helm installing into %s...", name, namespace)
	helmInstall(t, kubeconfig, namespace, release, o.configTOML, o.extraValues)
	t.Cleanup(func() {
		_ = runHelm(helmPath(), kubeconfig, "uninstall", release, "--namespace", namespace)
	})
	t.Logf("[%s] Helm install complete", name)

	t.Logf("[%s] Starting port-forward to svc/%s in %s...", name, release, namespace)
	serverURL, stopPF := portForwardService(ctx, t, restCfg, clientset, namespace, release, serverPort)
	t.Cleanup(stopPF)
	t.Logf("[%s] Port-forward ready at %s", name, serverURL)

	t.Logf("[%s] Waiting for healthz...", name)
	if err := waitForHealthz(ctx, serverURL, 30*time.Second); err != nil {
		dumpPodDiagnostics(t, kubeconfig, namespace, release)
		require.NoError(t, err, "healthz check")
	}
	t.Logf("[%s] Server healthy", name)

	return &serverDeployment{
		name:      release,
		namespace: namespace,
		serverURL: serverURL,
	}
}

// mergeValues deep-merges Helm value maps left to right. Nested maps are merged
// recursively; slices of maps (e.g. extraVolumes) are concatenated; all other
// collisions are won by the rightmost map.
func mergeValues(maps ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range maps {
		deepMerge(out, m)
	}
	return out
}

func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		dv, exists := dst[k]
		if !exists {
			dst[k] = sv
			continue
		}
		if dm, ok := dv.(map[string]any); ok {
			if sm, ok := sv.(map[string]any); ok {
				deepMerge(dm, sm)
				continue
			}
		}
		if ds, ok := dv.([]map[string]any); ok {
			if ss, ok := sv.([]map[string]any); ok {
				dst[k] = append(ds, ss...)
				continue
			}
		}
		dst[k] = sv
	}
}

// viewClusterRoleBindingValues returns Helm values binding the server's
// ServiceAccount to the built-in "view" ClusterRole.
func viewClusterRoleBindingValues() map[string]any {
	return map[string]any{
		"rbac": map[string]any{
			"create": true,
			"extraClusterRoleBindings": []map[string]any{
				{
					"name": "view",
					"roleRef": map[string]any{
						"name":     "view",
						"external": true,
					},
				},
			},
		},
	}
}

// dynamicToolsRBACValues lets the server's ServiceAccount list the Tool and
// Prompt resources the dynamic toolset exposes.
func dynamicToolsRBACValues() map[string]any {
	// rbac.dynamicTools renders the ClusterRole the dynamic toolset needs:
	// list on tools, prompts and toolsets, update on their status.
	return map[string]any{"rbac": map[string]any{"create": true, "dynamicTools": true}}
}

// requireToolStatus asserts the server wrote the Tool resource's status: a
// Ready condition and the MCP name it is exposed as.
func requireToolStatus(ctx context.Context, t *testing.T, resource, exposed string) {
	t.Helper()
	require.Eventually(t, func() bool {
		cmd := exec.CommandContext(ctx, "kubectl", "get", "tools.tools.kubernetes-mcp-server.io", resource,
			"-o", `jsonpath={.status.name} {.status.conditions[?(@.type=="Ready")].status}`, "--kubeconfig", kubeconfigPath())
		out, err := cmd.CombinedOutput()
		return err == nil && strings.TrimSpace(string(out)) == exposed+" True"
	}, 30*time.Second, 2*time.Second, "Tool %s should report status.name=%s and Ready=True", resource, exposed)
}

// kubernetesInterface is the clientset type pre-install hooks receive.
type kubernetesInterface = kubernetes.Interface

// applyDynamicToolsCRDs applies the Tool, Prompt and ToolSet CRDs.
func applyDynamicToolsCRDs(ctx context.Context, t *testing.T) {
	t.Helper()
	examples, err := filepath.Abs("../../charts/dynamic-tools")
	require.NoError(t, err)
	kubectlApply(ctx, t, "-f", filepath.Join(examples, "crd.yaml"))
}

// kubectlApply runs kubectl apply with the given arguments; what it created
// is deleted on cleanup.
func kubectlApply(ctx context.Context, t *testing.T, args ...string) {
	t.Helper()
	kubeconfig := kubeconfigPath()
	cmd := exec.CommandContext(ctx, "kubectl", append(append([]string{"apply"}, args...), "--kubeconfig", kubeconfig)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kubectl apply %v: %s", args, string(out))
	t.Cleanup(func() {
		del := exec.Command("kubectl", append(append([]string{"delete", "--ignore-not-found", "--wait=false"}, args...), "--kubeconfig", kubeconfig)...)
		_, _ = del.CombinedOutput()
	})
}

// kubectlApplyManifest applies a manifest given as text; it is deleted on cleanup.
func kubectlApplyManifest(ctx context.Context, t *testing.T, manifest string) {
	t.Helper()
	kubeconfig := kubeconfigPath()
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-", "--kubeconfig", kubeconfig)
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kubectl apply: %s", string(out))
	t.Cleanup(func() {
		del := exec.Command("kubectl", "delete", "--ignore-not-found", "--wait=false", "-f", "-", "--kubeconfig", kubeconfig)
		del.Stdin = strings.NewReader(manifest)
		_, _ = del.CombinedOutput()
	})
}

// installDynamicToolsChart returns a pre-install hook that applies the Tool
// and Prompt CRDs and installs one of the charts/dynamic-tools charts with
// the given --set values. The release is uninstalled on cleanup.
func installDynamicToolsChart(chart string, values map[string]string) func(context.Context, *testing.T, kubernetes.Interface, string) {
	return func(ctx context.Context, t *testing.T, _ kubernetes.Interface, namespace string) {
		t.Helper()
		kubeconfig := kubeconfigPath()
		examples, err := filepath.Abs("../../charts/dynamic-tools")
		require.NoError(t, err)
		applyDynamicToolsCRDs(ctx, t)
		release := namespace + "-" + chart
		args := []string{"upgrade", "--install", release, filepath.Join(examples, chart), "--namespace", namespace}
		for key, value := range values {
			args = append(args, "--set", key+"="+value)
		}
		require.NoError(t, runHelm(helmPath(), kubeconfig, args...), "install %s", chart)
		t.Cleanup(func() {
			_ = runHelm(helmPath(), kubeconfig, "uninstall", release, "--namespace", namespace)
		})
		t.Logf("Installed dynamic tools chart %s as release %s", chart, release)
	}
}

func createTestNamespace(ctx context.Context, t *testing.T, clientset kubernetes.Interface, prefix string) string {
	t.Helper()
	ns, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("e2e-%s-", prefix),
			Labels:       map[string]string{"app.kubernetes.io/managed-by": "e2e-test"},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err, "create namespace")
	return ns.Name
}

func helmInstall(t *testing.T, kubeconfig, namespace, name, configTOML string, extraValues map[string]any) {
	t.Helper()

	config := map[string]any{}
	if strings.TrimSpace(configTOML) != "" {
		require.NoError(t, toml.Unmarshal([]byte(configTOML), &config), "parse config TOML")
	}
	// Helm's toToml converts large integers to scientific notation which the
	// TOML parser rejects (helm/helm#32040).
	if _, hasHTTP := config["http"]; hasHTTP {
		t.Fatal("[http] config section not supported in Helm values — Helm's toToml mangles large integers (helm/helm#32040)")
	}

	values := mergeValues(map[string]any{
		"fullnameOverride": name,
		"config":           config,
		"image":            imageSpec(serverImage()),
		"ingress": map[string]any{
			"enabled": false,
		},
	}, extraValues)

	valuesJSON, err := json.Marshal(values)
	require.NoError(t, err, "marshal values")
	valuesYAML, err := yaml.JSONToYAML(valuesJSON)
	require.NoError(t, err, "convert values to YAML")

	f, err := os.Create(filepath.Join(t.TempDir(), "helm-values.yaml"))
	require.NoError(t, err, "create values temp file")
	_, err = f.Write(valuesYAML)
	require.NoError(t, err, "write values file")
	require.NoError(t, f.Close())

	err = runHelm(helmPath(), kubeconfig,
		"install", name, chartPath(),
		"--namespace", namespace,
		"--values", f.Name(),
		"--wait", "--timeout", "2m",
	)
	if err != nil {
		dumpPodDiagnostics(t, kubeconfig, namespace, name)
	}
	require.NoError(t, err, "helm install")
}

func dumpPodDiagnostics(t *testing.T, kubeconfig, namespace, releaseName string) {
	t.Helper()
	cmd := exec.Command(kubectlPath(), "--kubeconfig", kubeconfig,
		"get", "pods", "-n", namespace,
		"-l", fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
		"-o", "wide")
	out, _ := cmd.CombinedOutput()
	t.Logf("[%s] --- Pods in %s ---\n%s", releaseName, namespace, string(out))

	cmd = exec.Command(kubectlPath(), "--kubeconfig", kubeconfig,
		"logs", "-n", namespace,
		"-l", fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
		"--tail=50")
	out, _ = cmd.CombinedOutput()
	t.Logf("[%s] --- Pod logs ---\n%s", releaseName, string(out))

	cmd = exec.Command(kubectlPath(), "--kubeconfig", kubeconfig,
		"get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
	out, _ = cmd.CombinedOutput()
	t.Logf("[%s] --- Events ---\n%s", releaseName, string(out))
}

// portForwardService resolves a service to a pod and starts a client-go port-forward.
// Returns the local URL and a stop function.
func portForwardService(
	ctx context.Context,
	t *testing.T,
	restCfg *rest.Config,
	clientset kubernetes.Interface,
	namespace, serviceName string,
	servicePort int,
) (string, func()) {
	t.Helper()

	podName := findPodForService(ctx, t, clientset, namespace, serviceName)
	localPort, stopFn := startPortForward(ctx, t, restCfg, namespace, podName, servicePort)

	return fmt.Sprintf("http://127.0.0.1:%d", localPort), stopFn
}

func findPodForService(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace, serviceName string) string {
	t.Helper()

	svc, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	require.NoError(t, err, "get service %s", serviceName)
	selector := labels.SelectorFromSet(svc.Spec.Selector)

	var podName string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			require.Fail(t, fmt.Sprintf("context cancelled while waiting for ready pod for service %s/%s", namespace, serviceName))
		default:
		}
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector.String(),
		})
		require.NoError(t, err, "list pods for service %s", serviceName)

		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					podName = pod.Name
					break
				}
			}
			if podName != "" {
				break
			}
		}
		if podName != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NotEmpty(t, podName, "no ready pod found for service %s/%s within 30s", namespace, serviceName)
	return podName
}

func startPortForward(ctx context.Context, t *testing.T, restCfg *rest.Config, namespace, podName string, remotePort int) (int, func()) {
	t.Helper()

	transport, upgrader, err := spdy.RoundTripperFor(restCfg)
	require.NoError(t, err, "create SPDY round tripper")

	hostURL, err := url.Parse(restCfg.Host)
	require.NoError(t, err, "parse restCfg.Host")

	pfURL := &url.URL{
		Scheme: hostURL.Scheme,
		Host:   hostURL.Host,
		Path:   strings.TrimRight(hostURL.Path, "/") + fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName),
	}
	if pfURL.Scheme == "" {
		pfURL.Scheme = "https"
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, pfURL)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	var closeOnce sync.Once
	stop := func() { closeOnce.Do(func() { close(stopCh) }) }

	ports := []string{fmt.Sprintf("0:%d", remotePort)}
	pf, err := portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	require.NoError(t, err, "create port forwarder")

	errCh := make(chan error, 1)
	go func() {
		errCh <- pf.ForwardPorts()
	}()

	readyTimer := time.NewTimer(30 * time.Second)
	defer readyTimer.Stop()
	select {
	case <-readyCh:
	case err := <-errCh:
		require.NoError(t, err, "port-forward failed to start")
	case <-readyTimer.C:
		stop()
		require.Fail(t, "port-forward did not become ready within 30s")
	}

	go func() {
		select {
		case err := <-errCh:
			if err != nil {
				select {
				case <-stopCh:
				default:
					fmt.Fprintf(os.Stderr, "[e2e] port-forward to %s/%s died: %v\n", namespace, podName, err)
				}
			}
		case <-ctx.Done():
		case <-stopCh:
		}
		stop()
	}()

	forwardedPorts, err := pf.GetPorts()
	require.NoError(t, err, "get forwarded ports")
	require.NotEmpty(t, forwardedPorts, "no forwarded ports")

	return int(forwardedPorts[0].Local), stop
}

func waitForHealthz(ctx context.Context, serverURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := serverURL + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server at %s not healthy within %v", healthURL, timeout)
}

func textContent(result *mcp.CallToolResult) string {
	var out string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			out += tc.Text
		}
	}
	return out
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// testState provides type-safe context storage for e2e test state, replacing
// per-test xxxCtxKey+xxxState boilerplate.
type testState[T any] struct{}

func (testState[T]) set(ctx context.Context, v *T) context.Context {
	return context.WithValue(ctx, testState[T]{}, v)
}

func (testState[T]) get(ctx context.Context) *T {
	return ctx.Value(testState[T]{}).(*T)
}

func requireToolCallSuccess(t *testing.T, mcpClient *test.McpClient, tool string, args map[string]any) string {
	t.Helper()
	result, err := mcpClient.CallTool(tool, args)
	require.NoError(t, err, "%s transport error", tool)
	require.False(t, result.IsError, "%s returned tool error: %s", tool, textContent(result))
	return textContent(result)
}

func requireToolCallError(t *testing.T, mcpClient *test.McpClient, tool string, args map[string]any) string {
	t.Helper()
	result, err := mcpClient.CallTool(tool, args)
	require.NoError(t, err, "%s transport error", tool)
	require.True(t, result.IsError, "%s should have returned tool error but succeeded: %s", tool, textContent(result))
	return textContent(result)
}

// imageSpec parses a container image reference and returns Helm values
// for the chart's image section.
func imageSpec(image string) map[string]any {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return map[string]any{
			"registry": "docker.io", "repository": image,
			"version": "latest", "pullPolicy": "IfNotPresent",
		}
	}
	version := "latest"
	if v, ok := named.(reference.Digested); ok {
		version = string(v.Digest())
	} else if v, ok := named.(reference.Tagged); ok {
		version = v.Tag()
	}
	return map[string]any{
		"registry":   reference.Domain(named),
		"repository": reference.Path(named),
		"version":    version,
		"pullPolicy": "IfNotPresent",
	}
}
