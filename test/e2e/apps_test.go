//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type appsState struct {
	mcpClient *test.McpClient
}

var appsTS testState[appsState]

func TestMCPApps(t *testing.T) {
	f := features.New("mcp-apps").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "mcp-apps",
				withConfig("apps_enabled = true"),
				withValues(viewClusterRoleBindingValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return appsTS.set(ctx, &appsState{mcpClient: mcpClient})
		}).
		Assess("advertises the MCP Apps extension", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			result := appsTS.get(ctx).mcpClient.InitializeResult
			require.Contains(t, result.Capabilities.Extensions, "io.modelcontextprotocol/ui")
			return ctx
		}).
		Assess("associates namespaces_list with its UI resource", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			tools, err := appsTS.get(ctx).mcpClient.ListTools()
			require.NoError(t, err)
			for _, tool := range tools.Tools {
				if tool.Name != "namespaces_list" {
					continue
				}
				ui, ok := tool.Meta["ui"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "ui://kubernetes-mcp-server/namespaces-list", ui["resourceUri"])
				return ctx
			}
			require.Fail(t, "namespaces_list was not registered")
			return ctx
		}).
		Assess("serves the self-contained UI resource", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			resource, err := appsTS.get(ctx).mcpClient.Session.ReadResource(ctx, &gosdk.ReadResourceParams{
				URI: "ui://kubernetes-mcp-server/namespaces-list",
			})
			require.NoError(t, err)
			require.Len(t, resource.Contents, 1)
			require.Equal(t, "text/html;profile=mcp-app", resource.Contents[0].MIMEType)
			require.Contains(t, resource.Contents[0].Text, "ui/notifications/tool-result")
			return ctx
		}).
		Assess("returns structured namespace data", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			result, err := appsTS.get(ctx).mcpClient.CallTool("namespaces_list", map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, result.StructuredContent)
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
