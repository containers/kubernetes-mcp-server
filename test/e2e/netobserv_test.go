//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type netobservState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

type flowCollectorConfig struct {
	Spec struct {
		Namespace string `json:"namespace"`
		Loki      struct {
			Enable     *bool  `json:"enable"`
			Mode       string `json:"mode"`
			Monolithic struct {
				InstallDemoLoki bool `json:"installDemoLoki"`
			} `json:"monolithic"`
		} `json:"loki"`
	} `json:"spec"`
}

var (
	netobservTS          testState[netobservState]
	netobservManifestDir = getNetobservManifestDir()
)

// getNetobservManifestDir returns the absolute path to NetObserv manifests directory
func getNetobservManifestDir() string {
	dir, _ := filepath.Abs("../../evals/tasks/netobserv/shared")
	return dir
}

// deployMockNetObservPlugin deploys the mock NetObserv console plugin
func deployMockNetObservPlugin(ctx context.Context, t *testing.T, kubeconfig string, clientset kubernetes.Interface) {
	t.Helper()

	// Path to mock plugin manifest
	manifestPath := filepath.Join(netobservManifestDir, "mock-plugin.yaml")

	// Apply the manifest using kubectl
	t.Logf("Deploying mock NetObserv plugin from %s", manifestPath)
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", manifestPath, "--kubeconfig", kubeconfig)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "kubectl apply failed: %s", string(output))
	t.Logf("Mock plugin manifest applied")

	// Wait for deployment to be ready
	t.Logf("Waiting for netobserv-plugin deployment to be ready...")
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		deploy, err := clientset.AppsV1().Deployments("netobserv").Get(ctx, "netobserv-plugin", metav1.GetOptions{})
		if err == nil && deploy.Status.ReadyReplicas > 0 {
			t.Logf("Mock plugin deployment ready")
			return
		}
		time.Sleep(2 * time.Second)
	}
	require.Fail(t, "Mock plugin deployment did not become ready in time")
}

// cleanupMockNetObservPlugin removes the mock NetObserv plugin
func cleanupMockNetObservPlugin(t *testing.T, kubeconfig string) {
	t.Helper()

	manifestPath := filepath.Join(netobservManifestDir, "mock-plugin.yaml")
	cmd := exec.Command("kubectl", "delete", "-f", manifestPath, "--kubeconfig", kubeconfig, "--ignore-not-found")
	_ = cmd.Run() // Best effort cleanup
}

// findNetObservPluginNamespace finds the namespace used by the NetObserv plugin service.
func findNetObservPluginNamespace(ctx context.Context, t *testing.T, clientset kubernetes.Interface) (string, bool) {
	t.Helper()

	// Check common namespaces where NetObserv plugin runs
	namespaces := []string{"netobserv", "openshift-netobserv"}

	for _, ns := range namespaces {
		svc, err := clientset.CoreV1().Services(ns).Get(ctx, "netobserv-plugin", metav1.GetOptions{})
		if err == nil && svc != nil {
			t.Logf("Found NetObserv plugin service in namespace: %s", ns)
			return ns, true
		}
	}

	t.Logf("NetObserv plugin service not found in namespaces: %v", namespaces)
	return "", false
}

func getFlowCollectorConfig(ctx context.Context, t *testing.T, kubeconfig string) (flowCollectorConfig, error) {
	t.Helper()

	cmd := exec.CommandContext(ctx, "kubectl", "get", "flowcollector", "cluster", "-o", "json", "--kubeconfig", kubeconfig)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return flowCollectorConfig{}, fmt.Errorf("get FlowCollector cluster: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var config flowCollectorConfig
	if err := json.Unmarshal(output, &config); err != nil {
		return flowCollectorConfig{}, fmt.Errorf("parse FlowCollector cluster: %w", err)
	}
	return config, nil
}

func requireFlowCollectorDemoLoki(t *testing.T, config flowCollectorConfig) {
	t.Helper()
	require.NotNil(t, config.Spec.Loki.Enable, "FlowCollector spec.loki.enable must be set to true")
	require.True(t, *config.Spec.Loki.Enable, "FlowCollector spec.loki.enable must be true to run real NetObserv tests")
	require.Equal(t, "Monolithic", config.Spec.Loki.Mode, "FlowCollector spec.loki.mode must be Monolithic to run real NetObserv tests")
	require.True(t, config.Spec.Loki.Monolithic.InstallDemoLoki, "FlowCollector spec.loki.monolithic.installDemoLoki must be true to run real NetObserv tests")
}

func netObservPluginNamespace(ctx context.Context, t *testing.T, clientset kubernetes.Interface, config flowCollectorConfig) string {
	t.Helper()
	if config.Spec.Namespace != "" {
		return config.Spec.Namespace
	}
	namespace, found := findNetObservPluginNamespace(ctx, t, clientset)
	require.True(t, found, "FlowCollector spec.namespace is empty and no netobserv-plugin Service was found")
	return namespace
}

// deployNetObservOperator deploys NetObserv operator and FlowCollector
func deployNetObservOperator(ctx context.Context, t *testing.T, kubeconfig string, clientset kubernetes.Interface) string {
	t.Helper()

	operatorNamespace := "openshift-netobserv-operator"
	pluginNamespace := "netobserv"

	// Reuse an existing FlowCollector only when its required Loki-backed setup is usable.
	config, flowCollectorErr := getFlowCollectorConfig(ctx, t, kubeconfig)
	if flowCollectorErr == nil {
		t.Logf("FlowCollector already exists; reusing its configuration")
		require.True(t, checkNetObservOperatorStatus(ctx, t, kubeconfig, clientset, operatorNamespace),
			"an existing FlowCollector was found, but the NetObserv operator is not ready")
		requireFlowCollectorDemoLoki(t, config)
		waitForFlowCollector(ctx, t, kubeconfig)
		pluginNamespace = netObservPluginNamespace(ctx, t, clientset, config)
		waitForLokiReady(ctx, t, kubeconfig, clientset, config, pluginNamespace)
		return pluginNamespace
	}
	require.True(t, isFlowCollectorMissingError(flowCollectorErr), "failed to check for an existing FlowCollector: %v", flowCollectorErr)

	// Check if operator is already deployed
	if checkNetObservOperatorStatus(ctx, t, kubeconfig, clientset, operatorNamespace) {
		t.Logf("NetObserv operator already deployed and ready, skipping operator deployment")

		// No FlowCollector exists, so create the test configuration.
		t.Logf("Creating FlowCollector")
		cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join(netobservManifestDir, "flowcollector.yaml"), "--kubeconfig", kubeconfig)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to create FlowCollector: %s", string(output))
		t.Cleanup(func() {
			cmd := exec.Command("kubectl", "delete", "-f", filepath.Join(netobservManifestDir, "flowcollector.yaml"), "--kubeconfig", kubeconfig, "--ignore-not-found")
			_ = cmd.Run()
		})
		config, err := getFlowCollectorConfig(ctx, t, kubeconfig)
		require.NoError(t, err, "Failed to read the FlowCollector created by the test")

		requireFlowCollectorDemoLoki(t, config)
		waitForFlowCollector(ctx, t, kubeconfig)
		pluginNamespace = netObservPluginNamespace(ctx, t, clientset, config)
		waitForLokiReady(ctx, t, kubeconfig, clientset, config, pluginNamespace)
		return pluginNamespace
	}
	t.Cleanup(func() {
		cleanupNetObservOperator(t, kubeconfig)
	})

	// Create CatalogSource (y-stream Konflux catalog)
	t.Logf("Creating CatalogSource: netobserv-konflux-fbc")
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join(netobservManifestDir, "operator-catalogsource.yaml"), "--kubeconfig", kubeconfig)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create CatalogSource: %s", string(output))

	// Wait for CatalogSource pod to be ready
	t.Logf("Waiting for CatalogSource pod to be ready...")
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		pods, err := clientset.CoreV1().Pods("openshift-marketplace").List(ctx, metav1.ListOptions{
			LabelSelector: "olm.catalogSource=netobserv-konflux-fbc",
		})
		if err == nil && len(pods.Items) > 0 {
			allReady := true
			for _, pod := range pods.Items {
				if pod.Status.Phase != "Running" {
					allReady = false
					break
				}
			}
			if allReady {
				t.Logf("CatalogSource pod is ready")
				break
			}
		}
		time.Sleep(5 * time.Second)
	}

	// Konflux bundle images are mirrored from registry.redhat.io to quay.io.
	// Match the operator backend tests by installing the cluster-wide IDMS before
	// subscribing to the operator.
	t.Logf("Creating NetObserv ImageDigestMirrorSet")
	cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join(netobservManifestDir, "operator-idms.yaml"), "--kubeconfig", kubeconfig)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create NetObserv ImageDigestMirrorSet: %s", string(output))

	// Create namespaces
	t.Logf("Creating namespaces")
	cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join(netobservManifestDir, "operator-namespace.yaml"), "--kubeconfig", kubeconfig)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create namespaces: %s", string(output))

	// Create OperatorGroup
	t.Logf("Creating OperatorGroup")
	cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join(netobservManifestDir, "operator-group.yaml"), "--kubeconfig", kubeconfig)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create OperatorGroup: %s", string(output))

	// Create Subscription
	t.Logf("Creating Subscription for netobserv-operator")
	cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join(netobservManifestDir, "operator-subscription.yaml"), "--kubeconfig", kubeconfig)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create Subscription: %s", string(output))

	// Wait for operator pod to be ready
	t.Logf("Waiting for operator pod to be ready...")
	deadline = time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		pods, err := clientset.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=netobserv-operator",
		})
		if err == nil && len(pods.Items) > 0 {
			allReady := true
			for _, pod := range pods.Items {
				podReady := false
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						podReady = true
						break
					}
				}
				if pod.Status.Phase != corev1.PodRunning || !podReady {
					allReady = false
					break
				}
			}
			if allReady {
				t.Logf("Operator pod is ready")
				break
			}
		}
		time.Sleep(10 * time.Second)
	}

	// Wait for FlowCollector CRD to be available
	t.Logf("Waiting for FlowCollector CRD...")
	deadline = time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		cmd = exec.CommandContext(ctx, "kubectl", "get", "crd", "flowcollectors.flows.netobserv.io", "--kubeconfig", kubeconfig)
		if cmd.Run() == nil {
			t.Logf("FlowCollector CRD is available")
			break
		}
		time.Sleep(5 * time.Second)
	}

	// Deploy FlowCollector
	t.Logf("Creating FlowCollector")
	cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join(netobservManifestDir, "flowcollector.yaml"), "--kubeconfig", kubeconfig)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create FlowCollector: %s", string(output))

	config, err = getFlowCollectorConfig(ctx, t, kubeconfig)
	require.NoError(t, err, "Failed to read the FlowCollector created by the test")
	requireFlowCollectorDemoLoki(t, config)

	// Wait for FlowCollector and Loki to be ready.
	waitForFlowCollector(ctx, t, kubeconfig)
	pluginNamespace = netObservPluginNamespace(ctx, t, clientset, config)
	waitForLokiReady(ctx, t, kubeconfig, clientset, config, pluginNamespace)
	return pluginNamespace
}

func isFlowCollectorMissingError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "notfound") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "no matches for kind") ||
		strings.Contains(message, "doesn't have a resource type")
}

// checkNetObservOperatorStatus checks if NetObserv operator is already deployed and ready
func checkNetObservOperatorStatus(ctx context.Context, t *testing.T, kubeconfig string, clientset kubernetes.Interface, operatorNamespace string) bool {
	t.Helper()

	// Check if operator namespace exists
	_, err := clientset.CoreV1().Namespaces().Get(ctx, operatorNamespace, metav1.GetOptions{})
	if err != nil {
		t.Logf("Operator namespace %s not found, operator will be deployed", operatorNamespace)
		return false
	}

	// Check if operator pod is running
	pods, err := clientset.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=netobserv-operator",
	})
	if err != nil || len(pods.Items) == 0 {
		t.Logf("Operator pod not found, operator will be deployed")
		return false
	}

	// Check if all operator pods are ready
	for _, pod := range pods.Items {
		if pod.Status.Phase != "Running" {
			t.Logf("Operator pod %s not running (phase: %s), operator will be deployed", pod.Name, pod.Status.Phase)
			return false
		}
		ready := false
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			t.Logf("Operator pod %s is not Ready, operator will be deployed", pod.Name)
			return false
		}
	}

	// Check if FlowCollector CRD exists
	cmd := exec.CommandContext(ctx, "kubectl", "get", "crd", "flowcollectors.flows.netobserv.io", "--kubeconfig", kubeconfig)
	if cmd.Run() != nil {
		t.Logf("FlowCollector CRD not found, operator will be deployed")
		return false
	}

	t.Logf("NetObserv operator is already deployed and ready")
	return true
}

// waitForFlowCollector waits for the FlowCollector and its managed components to be ready.
func waitForFlowCollector(ctx context.Context, t *testing.T, kubeconfig string) {
	t.Helper()

	t.Logf("Waiting for FlowCollector to become Ready...")
	waitCmd := exec.CommandContext(ctx, "kubectl", "wait", "--for=condition=Ready", "flowcollector/cluster", "--timeout=10m", "--kubeconfig", kubeconfig)
	waitOutput, waitErr := waitCmd.CombinedOutput()
	if waitErr == nil {
		t.Logf("FlowCollector is ready: %s", strings.TrimSpace(string(waitOutput)))
		return
	}

	statusCmd := exec.CommandContext(ctx, "kubectl", "get", "flowcollector", "cluster", "-o", "yaml", "--kubeconfig", kubeconfig)
	statusOutput, statusErr := statusCmd.CombinedOutput()
	if statusErr != nil {
		require.NoError(t, waitErr, "FlowCollector did not become Ready: %s; failed to retrieve FlowCollector status: %v: %s",
			strings.TrimSpace(string(waitOutput)), statusErr, strings.TrimSpace(string(statusOutput)))
		return
	}
	require.NoError(t, waitErr, "FlowCollector did not become Ready: %s\nFlowCollector status:\n%s",
		strings.TrimSpace(string(waitOutput)), strings.TrimSpace(string(statusOutput)))
}

// waitForLokiReady waits until the Monolithic demo Loki service accepts requests.
func waitForLokiReady(ctx context.Context, t *testing.T, kubeconfig string, clientset kubernetes.Interface, config flowCollectorConfig, pluginNamespace string) {
	t.Helper()

	serviceNamespace := config.Spec.Namespace
	if serviceNamespace == "" {
		serviceNamespace = pluginNamespace
	}
	const serviceName = "loki"
	const servicePort = 3100

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err, "build rest config for Loki readiness check")

	deadline := time.Now().Add(3 * time.Minute)
	var serviceErr error
	for time.Now().Before(deadline) {
		_, serviceErr = clientset.CoreV1().Services(serviceNamespace).Get(ctx, serviceName, metav1.GetOptions{})
		if serviceErr == nil {
			break
		}
		select {
		case <-ctx.Done():
			require.Fail(t, "context cancelled while waiting for Loki service: "+ctx.Err().Error())
			return
		case <-time.After(2 * time.Second):
		}
	}
	require.NoError(t, serviceErr, "Loki service %s/%s was not created within 3 minutes", serviceNamespace, serviceName)

	localURL, stopPortForward := portForwardServiceWithTimeout(ctx, t, restCfg, clientset, serviceNamespace, serviceName, servicePort, 3*time.Minute)
	defer stopPortForward()
	checkURL, err := url.Parse(localURL)
	require.NoError(t, err, "parse local Loki port-forward URL")
	checkURL.Path = "/ready"

	client := &http.Client{Timeout: 5 * time.Second}
	readinessDeadline := time.Now().Add(3 * time.Minute)
	lastStatus := "no response"
	t.Logf("Waiting for Loki /ready endpoint at %s...", checkURL.Redacted())
	for time.Now().Before(readinessDeadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL.String(), nil)
		require.NoError(t, err, "create Loki readiness request")

		response, err := client.Do(req)
		if err != nil {
			lastStatus = err.Error()
		} else {
			lastStatus = response.Status
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
				t.Logf("Loki is ready: %s", response.Status)
				return
			}
		}
		time.Sleep(2 * time.Second)
	}

	require.Fail(t, "Loki did not become ready within 3 minutes; last /ready response: "+lastStatus)
}

// cleanupNetObservOperator removes the NetObserv operator, FlowCollector, and IDMS.
func cleanupNetObservOperator(t *testing.T, kubeconfig string) {
	t.Helper()

	// Best effort cleanup - delete in reverse order
	t.Logf("Cleaning up NetObserv operator")

	// Delete FlowCollector
	cmd := exec.Command("kubectl", "delete", "-f", filepath.Join(netobservManifestDir, "flowcollector.yaml"), "--kubeconfig", kubeconfig, "--ignore-not-found")
	_ = cmd.Run()

	// Delete Subscription
	cmd = exec.Command("kubectl", "delete", "-f", filepath.Join(netobservManifestDir, "operator-subscription.yaml"), "--kubeconfig", kubeconfig, "--ignore-not-found")
	_ = cmd.Run()

	// Delete OperatorGroup
	cmd = exec.Command("kubectl", "delete", "-f", filepath.Join(netobservManifestDir, "operator-group.yaml"), "--kubeconfig", kubeconfig, "--ignore-not-found")
	_ = cmd.Run()

	// Delete namespaces
	cmd = exec.Command("kubectl", "delete", "-f", filepath.Join(netobservManifestDir, "operator-namespace.yaml"), "--kubeconfig", kubeconfig, "--ignore-not-found")
	_ = cmd.Run()

	// Delete CatalogSource
	cmd = exec.Command("kubectl", "delete", "-f", filepath.Join(netobservManifestDir, "operator-catalogsource.yaml"), "--kubeconfig", kubeconfig, "--ignore-not-found")
	_ = cmd.Run()

	// Delete the ImageDigestMirrorSet.
	cmd = exec.Command("kubectl", "delete", "-f", filepath.Join(netobservManifestDir, "operator-idms.yaml"), "--kubeconfig", kubeconfig, "--ignore-not-found")
	_ = cmd.Run()
}

// TestNetObservMock tests NetObserv MCP tools against mock plugin (no operator required)
func TestNetObservMock(t *testing.T) {
	f := features.New("netobserv-mock").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			kubeconfig := cfg.KubeconfigFile()
			clientset, err := clientsetFromKubeconfig(kubeconfig)
			require.NoError(t, err, "create clientset")

			// Deploy mock NetObserv plugin
			deployMockNetObservPlugin(ctx, t, kubeconfig, clientset)
			t.Cleanup(func() {
				cleanupMockNetObservPlugin(t, kubeconfig)
			})

			// Deploy MCP server configured to use mock plugin
			dep := deployServer(ctx, t, cfg, "netobserv-mock",
				withConfig(`
toolsets = ["core", "netobserv"]

[toolset_configs.netobserv]
url = "http://netobserv-plugin.netobserv.svc.cluster.local:9001"
`),
				withValues(viewClusterRoleBindingValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return netobservTS.set(ctx, &netobservState{dep: dep, mcpClient: mcpClient})
		}).
		// Tier 1: Smoke Tests
		Assess("netobserv tools are registered", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			result, err := s.mcpClient.ListTools()
			require.NoError(t, err)
			names := toolNames(result.Tools)

			require.Contains(t, names, "netobserv_list_flows", "list_flows tool should be registered")
			require.Contains(t, names, "netobserv_get_flow_metrics", "get_flow_metrics tool should be registered")
			require.Contains(t, names, "netobserv_export_flows", "export_flows tool should be registered")

			return ctx
		}).
		Assess("list_flows returns JSON with expected fields", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservListFlows(t, s.mcpClient, map[string]any{
				"timeRange": makeTimeRange(5),
				"namespace": "default",
			})

			return ctx
		}).
		Assess("get_flow_metrics returns success status", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservGetMetrics(t, s.mcpClient, map[string]any{
				"timeRange":   makeTimeRange(5),
				"aggregateBy": "namespace",
				"type":        "Bytes",
			})

			return ctx
		}).
		Assess("export_flows returns CSV format", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservExportFlows(t, s.mcpClient, map[string]any{
				"timeRange": makeTimeRange(5),
				"namespace": "default",
			})

			return ctx
		}).
		// Tier 2: Contract Tests
		Assess("filters parameter narrows results", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservListFlows(t, s.mcpClient, map[string]any{
				"timeRange": makeTimeRange(5),
				"filters":   makeFilters("SrcK8S_Namespace=default"),
			})

			return ctx
		}).
		Assess("invalid filter returns error", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservToolCallError(t, s.mcpClient, "netobserv_list_flows", map[string]any{
				"timeRange": makeTimeRange(5),
				"filters":   makeFilters("InvalidFilter"),
			})

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

// TestNetObservReal tests NetObserv MCP tools against a FlowCollector with Loki enabled.
// Set NETOBSERV_OPERATOR=deploy to reuse a ready installation or deploy missing prerequisites.
func TestNetObservReal(t *testing.T) {
	operatorMode := os.Getenv("NETOBSERV_OPERATOR")
	if operatorMode == "" {
		t.Skip("Skipping real plugin tests - set NETOBSERV_OPERATOR=deploy to run")
	}

	f := features.New("netobserv-real").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			kubeconfig := cfg.KubeconfigFile()
			clientset, err := clientsetFromKubeconfig(kubeconfig)
			require.NoError(t, err, "create clientset")

			if operatorMode != "deploy" {
				t.Skipf("Invalid NETOBSERV_OPERATOR value: %s (use 'deploy')", operatorMode)
			}

			t.Logf("Ensuring NetObserv operator, FlowCollector, and Loki are ready")
			pluginNamespace := deployNetObservOperator(ctx, t, kubeconfig, clientset)

			// Deploy MCP server configured to use real plugin service
			configTOML := fmt.Sprintf(`
toolsets = ["core", "netobserv"]

[toolset_configs.netobserv]
namespace = "%s"
`, pluginNamespace)

			dep := deployServer(ctx, t, cfg, "netobserv-real",
				withNamespace("e2e-netobserv-real"),
				withConfig(configTOML),
				withValues(viewClusterRoleBindingValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return netobservTS.set(ctx, &netobservState{dep: dep, mcpClient: mcpClient})
		}).
		// Tier 1: Smoke Tests
		Assess("list_flows returns actual flow data", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservListFlows(t, s.mcpClient, map[string]any{
				"timeRange": makeTimeRange(15),
			})

			return ctx
		}).
		Assess("get_flow_metrics returns actual metrics", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservGetMetrics(t, s.mcpClient, map[string]any{
				"timeRange":   makeTimeRange(15),
				"aggregateBy": "namespace",
				"type":        "Bytes",
				"function":    "rate",
			})

			return ctx
		}).
		// Tier 3: Optional Feature Sample (DNS enrichment)
		Assess("DNS enrichment is available in flows", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			result, err := s.mcpClient.CallTool("netobserv_list_flows", map[string]any{
				"timeRange": makeTimeRange(30),
				"filters":   makeFilters("DnsFlagsResponseCode!="),
			})

			// This may return empty if no DNS flows exist, which is okay
			// We're just checking the filter works without error
			require.NoError(t, err, "DNS filter should not error")
			require.NotNil(t, result, "should return result")

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
