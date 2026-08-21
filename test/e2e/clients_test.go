//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type clientsState struct {
	dep *serverDeployment
}

var clientsTS testState[clientsState]

func TestClientConnectivity(t *testing.T) {
	f := features.New("client-connectivity").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "clients",
				withConfig(`read_only = true`),
				withValues(viewClusterRoleBindingValues()),
			)
			return clientsTS.set(ctx, &clientsState{dep: dep})
		}).
		Assess("sdk matrix", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := clientsTS.get(ctx)

			type clientInfoCase struct {
				name   string
				option test.McpClientOption // nil means use SDK default
			}
			clientInfos := []clientInfoCase{
				{"named", test.WithClientInfo("test-client", "1.0")},
				{"empty", test.WithEmptyClientInfo()},
				{"default", nil},
			}

			type capCase struct {
				name   string
				option test.McpClientOption
			}
			capabilities := []capCase{
				{"none", test.WithClientCapabilities(&mcp.ClientCapabilities{})},
				{"roots", test.WithClientCapabilities(&mcp.ClientCapabilities{
					RootsV2: &mcp.RootCapabilities{ListChanged: true},
				})},
				{"elicitation", test.WithClientCapabilities(&mcp.ClientCapabilities{
					Elicitation: &mcp.ElicitationCapabilities{
						Form: &mcp.FormElicitationCapabilities{},
					},
				})},
				{"both", test.WithClientCapabilities(&mcp.ClientCapabilities{
					RootsV2: &mcp.RootCapabilities{ListChanged: true},
					Elicitation: &mcp.ElicitationCapabilities{
						Form: &mcp.FormElicitationCapabilities{},
					},
				})},
			}

			for _, ci := range clientInfos {
				for _, cap := range capabilities {
					name := fmt.Sprintf("%s/%s", ci.name, cap.name)
					opts := []test.McpClientOption{test.WithEndpoint(s.dep.serverURL + "/mcp")}
					if ci.option != nil {
						opts = append(opts, ci.option)
					}
					if cap.option != nil {
						opts = append(opts, cap.option)
					}
					t.Run(name, func(t *testing.T) {
						mcpClient := test.NewMcpClient(t, nil, opts...)
						t.Cleanup(mcpClient.Close)

						tools, err := mcpClient.ListTools()
						require.NoError(t, err, "ListTools")
						require.Greater(t, len(tools.Tools), 0, "expected at least one tool")

						output := requireToolCallSuccess(t, mcpClient, "configuration_view", nil)
						require.NotEmpty(t, output, "configuration_view returned empty")
					})
				}
			}

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
