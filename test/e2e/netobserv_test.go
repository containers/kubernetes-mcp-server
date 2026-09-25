//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type netobservState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
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

// checkNetObservOperatorDeployed checks if NetObserv operator is deployed and returns the plugin namespace
func checkNetObservOperatorDeployed(ctx context.Context, t *testing.T, clientset kubernetes.Interface) (string, bool) {
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

// deployNetObservOperator deploys NetObserv operator and FlowCollector
func deployNetObservOperator(ctx context.Context, t *testing.T, kubeconfig string, clientset kubernetes.Interface) string {
	t.Helper()

	operatorNamespace := "openshift-netobserv-operator"
	pluginNamespace := "netobserv"

	// Check if operator is already deployed
	if checkNetObservOperatorStatus(ctx, t, kubeconfig, clientset, operatorNamespace) {
		t.Logf("NetObserv operator already deployed and ready, skipping operator deployment")

		// Still need to ensure FlowCollector exists
		t.Logf("Checking FlowCollector...")
		cmd := exec.CommandContext(ctx, "kubectl", "get", "flowcollector", "cluster", "--kubeconfig", kubeconfig)
		if cmd.Run() != nil {
			// FlowCollector doesn't exist, create it
			t.Logf("Creating FlowCollector")
			cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", filepath.Join(netobservManifestDir, "flowcollector.yaml"), "--kubeconfig", kubeconfig)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "Failed to create FlowCollector: %s", string(output))
		} else {
			t.Logf("FlowCollector already exists")
		}

		// Wait for console plugin to be ready and return
		return waitForConsolePlugin(ctx, t, clientset, pluginNamespace)
	}

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

	// Wait for console plugin service to be ready
	return waitForConsolePlugin(ctx, t, clientset, pluginNamespace)
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

	// Check if all operator pods are running
	for _, pod := range pods.Items {
		if pod.Status.Phase != "Running" {
			t.Logf("Operator pod %s not running (phase: %s), operator will be deployed", pod.Name, pod.Status.Phase)
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

// waitForConsolePlugin waits for the NetObserv console plugin to be ready
func waitForConsolePlugin(ctx context.Context, t *testing.T, clientset kubernetes.Interface, pluginNamespace string) string {
	t.Helper()

	t.Logf("Waiting for console plugin service to be ready...")
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		svc, err := clientset.CoreV1().Services(pluginNamespace).Get(ctx, "netobserv-plugin", metav1.GetOptions{})
		if err == nil && svc != nil {
			// Also check if plugin pods are running
			pods, err := clientset.CoreV1().Pods(pluginNamespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=netobserv-plugin",
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
					t.Logf("Console plugin is ready in namespace: %s", pluginNamespace)
					return pluginNamespace
				}
			}
		}
		time.Sleep(10 * time.Second)
	}

	require.Fail(t, "Console plugin did not become ready in time")
	return pluginNamespace
}

// cleanupNetObservOperator removes NetObserv operator and FlowCollector
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
[toolsets.netobserv]
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

// TestNetObservReal tests NetObserv MCP tools against real plugin (requires operator)
// Set NETOBSERV_OPERATOR=deploy to deploy operator, or NETOBSERV_OPERATOR=use-existing to use pre-deployed operator
func TestNetObservReal(t *testing.T) {
	operatorMode := os.Getenv("NETOBSERV_OPERATOR")
	if operatorMode == "" {
		t.Skip("Skipping real plugin tests - set NETOBSERV_OPERATOR=deploy or NETOBSERV_OPERATOR=use-existing to run")
	}

	f := features.New("netobserv-real").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			kubeconfig := cfg.KubeconfigFile()
			clientset, err := clientsetFromKubeconfig(kubeconfig)
			require.NoError(t, err, "create clientset")

			var pluginNamespace string

			switch operatorMode {
			case "deploy":
				t.Logf("NETOBSERV_OPERATOR=deploy - deploying NetObserv operator and FlowCollector")
				pluginNamespace = deployNetObservOperator(ctx, t, kubeconfig, clientset)
				t.Cleanup(func() {
					cleanupNetObservOperator(t, kubeconfig)
				})

			case "use-existing":
				t.Logf("NETOBSERV_OPERATOR=use-existing - checking for existing operator")
				var found bool
				pluginNamespace, found = checkNetObservOperatorDeployed(ctx, t, clientset)
				if !found {
					t.Skip("NetObserv operator not found - deploy it first or use NETOBSERV_OPERATOR=deploy")
				}
				t.Logf("Using existing NetObserv plugin in namespace: %s", pluginNamespace)

			default:
				t.Skipf("Invalid NETOBSERV_OPERATOR value: %s (use 'deploy' or 'use-existing')", operatorMode)
			}

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
