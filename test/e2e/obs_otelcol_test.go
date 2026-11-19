//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type obsOtelColState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var obsOtelColTS testState[obsOtelColState]

func TestObsOtelColTools(t *testing.T) {
	f := features.New("obs-otelcol-tools").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "obs-otelcol",
				withConfig(obsOtelColConfig()),
				withValues(viewClusterRoleBindingValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return obsOtelColTS.set(ctx, &obsOtelColState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("otelcol_get_versions returns version info", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsOtelColTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "otelcol_get_versions", map[string]any{})
			require.Contains(t, output, "versions")
			return ctx
		}).
		Assess("otelcol_list_components returns components", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsOtelColTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "otelcol_list_components", map[string]any{})
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("otelcol_get_component_schema returns schema", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsOtelColTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "otelcol_get_component_schema", map[string]any{
				"component_type": "receiver",
				"component_name": "otlp",
			})
			require.Contains(t, output, "otlp")
			return ctx
		}).
		Assess("otelcol_get_component_schema rejects missing component_name", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsOtelColTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "otelcol_get_component_schema", map[string]any{
				"component_type": "receiver",
			})
			return ctx
		}).
		Assess("otelcol_get_component_schema with non-existent component returns error", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsOtelColTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "otelcol_get_component_schema", map[string]any{
				"component_type": "receiver",
				"component_name": "nonexistent-component-12345",
			})
			return ctx
		}).
		Assess("otelcol_validate_config validates configuration", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsOtelColTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "otelcol_validate_config", map[string]any{
				"component_type": "processor",
				"component_name": "batch",
				"config":         "send_batch_size: 8192\ntimeout: 200ms",
				"format":         "yaml",
			})
			require.NotEmpty(t, output)
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
