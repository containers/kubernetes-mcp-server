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

type obsLogsState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var obsLogsTS testState[obsLogsState]

func TestObsLogsTools(t *testing.T) {
	f := features.New("obs-logs-tools").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "obs-logs",
				withConfig(obsLogsConfig()),
				withValues(obsLogsRBACValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return obsLogsTS.set(ctx, &obsLogsState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("loki_list_instances returns instances", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "loki_list_instances", map[string]any{})
			require.Contains(t, output, "obs-loki")
			return ctx
		}).
		Assess("loki_label_names returns labels", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "loki_label_names", lokiBaseArgs())
			// Log generator pushes with app, level, SrcK8S_Namespace, DstK8S_Namespace.
			require.Contains(t, output, "app")
			require.Contains(t, output, "level")
			return ctx
		}).
		Assess("loki_label_names with time range", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "loki_label_names", mergeLokiArgs(map[string]any{
				"start": "NOW-1h",
				"end":   "NOW",
			}))
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("loki_label_names rejects start without end", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "loki_label_names", mergeLokiArgs(map[string]any{
				"start": "NOW-1h",
			}))
			return ctx
		}).
		Assess("loki_label_values returns app values", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "loki_label_values", mergeLokiArgs(map[string]any{
				"label": "app",
			}))
			require.Contains(t, output, "obs-log-generator")
			return ctx
		}).
		Assess("loki_label_values with time range", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "loki_label_values", mergeLokiArgs(map[string]any{
				"label": "level",
				"start": "NOW-1h",
				"end":   "NOW",
			}))
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("loki_label_values rejects missing label", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "loki_label_values", lokiBaseArgs())
			return ctx
		}).
		Assess("loki_label_values rejects start without end", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "loki_label_values", mergeLokiArgs(map[string]any{
				"label": "app",
				"start": "NOW-1h",
			}))
			return ctx
		}).
		Assess("loki_query_range returns log entries", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "loki_query_range", mergeLokiArgs(map[string]any{
				"query":    `{app="obs-log-generator"}`,
				"duration": "5m",
			}))
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("loki_query_range with explicit start/end", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "loki_query_range", mergeLokiArgs(map[string]any{
				"query": `{app="obs-log-generator"}`,
				"start": "NOW-10m",
				"end":   "NOW",
			}))
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("loki_query_range with limit", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "loki_query_range", mergeLokiArgs(map[string]any{
				"query":    `{app="obs-log-generator"}`,
				"duration": "5m",
				"limit":    5,
			}))
			return ctx
		}).
		Assess("loki_query_range with forward direction", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "loki_query_range", mergeLokiArgs(map[string]any{
				"query":     `{app="obs-log-generator"}`,
				"start":     "NOW-30m",
				"end":       "NOW",
				"limit":     5,
				"direction": "forward",
			}))
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("loki_query_range with non-existent app returns empty", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "loki_query_range", mergeLokiArgs(map[string]any{
				"query":    `{app="nonexistent-app-xyz-12345"}`,
				"duration": "5m",
			}))
			return ctx
		}).
		Assess("loki_query_range rejects invalid LogQL", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "loki_query_range", mergeLokiArgs(map[string]any{
				"query":    `{{{invalid`,
				"duration": "5m",
			}))
			return ctx
		}).
		Assess("loki_query_range rejects missing query", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "loki_query_range", lokiBaseArgs())
			return ctx
		}).
		Assess("loki_query_range rejects invalid direction", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsLogsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "loki_query_range", mergeLokiArgs(map[string]any{
				"query":     `{app="obs-log-generator"}`,
				"direction": "invalid",
			}))
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
