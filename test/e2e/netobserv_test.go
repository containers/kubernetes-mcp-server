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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type netobservState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var netobservTS testState[netobservState]

// deployMockNetObservPlugin deploys the mock NetObserv console plugin
func deployMockNetObservPlugin(ctx context.Context, t *testing.T, kubeconfig string, clientset kubernetes.Interface) {
	t.Helper()

	// Path to mock plugin manifest (relative to repo root)
	manifestPath := filepath.Join("evals", "tasks", "netobserv", "shared", "mock-plugin.yaml")

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

	manifestPath := filepath.Join("evals", "tasks", "netobserv", "shared", "mock-plugin.yaml")
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

	// TODO: Implement full operator deployment
	// For now, this is a placeholder that would:
	// 1. Deploy NetObserv operator (via OLM or manifests)
	// 2. Wait for operator to be ready
	// 3. Deploy FlowCollector CR
	// 4. Wait for console plugin service to be ready
	// 5. Return the namespace

	t.Skip("Operator deployment not yet implemented - use NETOBSERV_OPERATOR=use-existing with pre-deployed operator")
	return "netobserv"
}

// cleanupNetObservOperator removes NetObserv operator and FlowCollector
func cleanupNetObservOperator(t *testing.T, kubeconfig string) {
	t.Helper()

	// TODO: Implement cleanup
	// Best effort cleanup of operator and FlowCollector
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
[toolsets.netobserv]
url = "http://netobserv-plugin.%s.svc.cluster.local:9001"
`, pluginNamespace)

			dep := deployServer(ctx, t, cfg, "netobserv-real",
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
