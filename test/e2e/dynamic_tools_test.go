//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

// The dynamic toolset in a deployed server, with no third-party backend:
// the CRDs, the chart's rbac.dynamicTools ClusterRole, Tool and ToolSet
// resources created before and after deployment, the status the server
// writes, the instructions a session receives, and a call that runs
// against the cluster. The hand-written examples under
// charts/dynamic-tools/tools and a sample family defined here are the
// resources; nothing in this test belongs to an ecosystem chart.

const dynamicToolsSampleFamily = `
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: ToolSet
metadata:
  name: e2e
spec:
  description: End-to-end sample family
  prefix: e2e
  instructions: |
    Start with e2e_namespaces.
---
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: e2e-namespaces
spec:
  toolset: e2e
  description: List the namespaces of the cluster by name, the end-to-end sample tool.
  readOnly: true
  examples:
    - summary: Every namespace
      arguments: {}
  inputSchema: {type: object}
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  limit: {default: 50, max: 500}
  output: response.items.map(n, n.metadata.name)
`

const dynamicToolsLaterTool = `
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: e2e-pods
spec:
  toolset: e2e
  description: List the pods of a namespace, or of the whole cluster when namespace is omitted, by name.
  readOnly: true
  examples:
    - summary: Pods of a namespace
      arguments: {namespace: default}
  inputSchema:
    type: object
    properties:
      namespace: {type: string}
  kubernetes:
    verb: list
    apiVersion: v1
    kind: Pod
    namespace: {argument: namespace, all: true}
  limit: {default: 50, max: 500}
  output: response.items.map(p, p.metadata.namespace + '/' + p.metadata.name)
`

type dynamicToolsState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var dynamicToolsTS testState[dynamicToolsState]

func TestDynamicToolsDeployment(t *testing.T) {
	f := features.New("dynamic-tools").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			examples, err := filepath.Abs("../../charts/dynamic-tools")
			require.NoError(t, err)
			dep := deployServer(ctx, t, cfg, "dynamic-tools",
				withConfig(`
toolsets = ["core", "dynamic"]
`),
				withValues(mergeValues(viewClusterRoleBindingValues(), dynamicToolsRBACValues())),
				withPreInstall(func(ctx context.Context, t *testing.T, _ kubernetesInterface, _ string) {
					applyDynamicToolsCRDs(ctx, t)
					kubectlApply(ctx, t, "-f", filepath.Join(examples, "tools"))
					kubectlApplyManifest(ctx, t, dynamicToolsSampleFamily)
				}),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return dynamicToolsTS.set(ctx, &dynamicToolsState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("tools defined as resources are listed next to the built-in ones", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := dynamicToolsTS.get(ctx)
			result, err := s.mcpClient.ListTools()
			require.NoError(t, err)
			names := toolNames(result.Tools)
			require.Contains(t, names, "namespaces_list", "a built-in tool")
			require.Contains(t, names, "deployments_summary", "a hand-written example without a family")
			require.Contains(t, names, "e2e_namespaces", "a tool of the sample family")
			return ctx
		}).
		Assess("the family's instructions reach the session", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := dynamicToolsTS.get(ctx)
			require.NotNil(t, s.mcpClient.InitializeResult)
			require.Contains(t, s.mcpClient.InitializeResult.Instructions, "## e2e\n\nStart with e2e_namespaces.")
			return ctx
		}).
		Assess("the server reports each resource's status", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			requireToolStatus(ctx, t, "e2e-namespaces", "e2e_namespaces")
			requireToolStatus(ctx, t, "deployments-summary", "deployments_summary")
			return ctx
		}).
		Assess("a resource-defined tool runs against the cluster", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := dynamicToolsTS.get(ctx)
			output := requireToolCallSuccess(t, s.mcpClient, "e2e_namespaces", map[string]any{})
			require.Contains(t, output, s.dep.namespace, "the server's own namespace is listed")
			return ctx
		}).
		Assess("a Tool created after deployment appears without a restart", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := dynamicToolsTS.get(ctx)
			kubectlApplyManifest(ctx, t, dynamicToolsLaterTool)
			require.Eventually(t, func() bool {
				result, err := s.mcpClient.ListTools()
				if err != nil {
					return false
				}
				for _, name := range toolNames(result.Tools) {
					if name == "e2e_pods" {
						return true
					}
				}
				return false
			}, 90*time.Second, 3*time.Second, "e2e_pods should be listed within the poll interval")
			output := requireToolCallSuccess(t, s.mcpClient, "e2e_pods", map[string]any{"namespace": s.dep.namespace})
			require.Contains(t, output, s.dep.namespace+"/", "the server's own pod is listed")
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
