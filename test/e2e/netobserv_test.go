//go:build e2e

package e2e

import (
	"context"
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

			assertNetobservListFlows(ctx, t, s.mcpClient.Client, map[string]any{
				"timeRange": makeTimeRange(5),
				"namespace": "default",
			})

			return ctx
		}).
		Assess("get_flow_metrics returns success status", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservGetMetrics(ctx, t, s.mcpClient.Client, map[string]any{
				"timeRange":   makeTimeRange(5),
				"aggregateBy": "namespace",
			})

			return ctx
		}).
		Assess("export_flows returns CSV format", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservExportFlows(ctx, t, s.mcpClient.Client, map[string]any{
				"timeRange": makeTimeRange(5),
				"namespace": "default",
			})

			return ctx
		}).
		// Tier 2: Contract Tests
		Assess("filters parameter narrows results", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservListFlows(ctx, t, s.mcpClient.Client, map[string]any{
				"timeRange": makeTimeRange(5),
				"filters":   makeFilters("SrcK8S_Namespace=default"),
			})

			return ctx
		}).
		Assess("invalid filter returns error", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			netobservToolCallExpectError(ctx, t, s.mcpClient.Client, "netobserv_list_flows", map[string]any{
				"timeRange": makeTimeRange(5),
				"filters":   makeFilters("InvalidFilter"),
			})

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

// TestNetObservReal tests NetObserv MCP tools against real plugin (requires operator)
func TestNetObservReal(t *testing.T) {
	t.Skip("Real NetObserv plugin tests require operator deployment")

	// TODO: Check if NetObserv operator is deployed
	// If not, skip with message: "NetObserv operator not found, skipping real plugin tests"

	f := features.New("netobserv-real").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Use real NetObserv plugin service
			dep := deployServer(ctx, t, cfg, "netobserv-real",
				withConfig(`
[toolsets.netobserv]
# Will auto-detect OpenShift and use in-cluster service
`),
				withValues(viewClusterRoleBindingValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return netobservTS.set(ctx, &netobservState{dep: dep, mcpClient: mcpClient})
		}).
		// Tier 1: Smoke Tests
		Assess("list_flows returns actual flow data", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservListFlows(ctx, t, s.mcpClient.Client, map[string]any{
				"timeRange": makeTimeRange(15),
			})

			return ctx
		}).
		Assess("get_flow_metrics returns actual metrics", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			assertNetobservGetMetrics(ctx, t, s.mcpClient.Client, map[string]any{
				"timeRange":   makeTimeRange(15),
				"aggregateBy": "namespace",
			})

			return ctx
		}).
		// Tier 3: Optional Feature Sample (DNS enrichment)
		Assess("DNS enrichment is available in flows", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := netobservTS.get(ctx)

			contents, err := netobservToolCall(ctx, t, s.mcpClient.Client, "netobserv_list_flows", map[string]any{
				"timeRange": makeTimeRange(30),
				"filters":   makeFilters("DnsFlagsResponseCode!="),
			})

			// This may return empty if no DNS flows exist, which is okay
			// We're just checking the filter works without error
			require.NoError(t, err, "DNS filter should not error")
			require.NotNil(t, contents, "should return content")

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
