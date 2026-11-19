//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type obsMetricsState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var obsMetricsTS testState[obsMetricsState]

func TestObsMetricsTools(t *testing.T) {
	f := features.New("obs-metrics-tools").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "obs-metrics",
				withConfig(obsMetricsConfig()),
				withValues(viewClusterRoleBindingValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return obsMetricsTS.set(ctx, &obsMetricsState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("list_metrics returns known metrics", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "list_metrics", map[string]any{
				"name_regex": "prometheus_build_info",
			})
			require.Contains(t, output, "prometheus_build_info")
			return ctx
		}).
		Assess("list_metrics with wildcard regex", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "list_metrics", map[string]any{
				"name_regex": "prometheus.*",
			})
			require.Contains(t, output, "prometheus_build_info")
			return ctx
		}).
		Assess("execute_instant_query returns results", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			skipIfThanos(t, s.mcpClient)
			output := requireObsToolCallSuccess(t, s.mcpClient, "execute_instant_query", map[string]any{
				"query": `up{job=~"prometheus.*"}`,
				"time":  "NOW",
			})
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("execute_instant_query rejects invalid PromQL", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "execute_instant_query", map[string]any{
				"query": `up{{{invalid`,
			})
			return ctx
		}).
		Assess("execute_instant_query rejects missing query", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "execute_instant_query", map[string]any{
				"time": "NOW",
			})
			return ctx
		}).
		Assess("execute_range_query returns results", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			skipIfThanos(t, s.mcpClient)
			output := requireObsToolCallSuccess(t, s.mcpClient, "execute_range_query", map[string]any{
				"query":    `up{job=~"prometheus.*"}`,
				"step":     "1m",
				"duration": "5m",
			})
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("execute_range_query with explicit start/end", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			skipIfThanos(t, s.mcpClient)
			output := requireObsToolCallSuccess(t, s.mcpClient, "execute_range_query", map[string]any{
				"query": `up{job=~"prometheus.*"}`,
				"step":  "1m",
				"start": "NOW-5m",
				"end":   "NOW",
			})
			require.NotEmpty(t, output)
			return ctx
		}).
		Assess("execute_range_query rejects invalid PromQL", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "execute_range_query", map[string]any{
				"query":    `up{{{invalid`,
				"step":     "1m",
				"duration": "5m",
			})
			return ctx
		}).
		Assess("execute_range_query rejects missing query", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "execute_range_query", map[string]any{
				"step":     "1m",
				"duration": "5m",
			})
			return ctx
		}).
		Assess("execute_range_query rejects non-existent metric", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			// Metric existence guardrails reject unknown metric names before query execution.
			requireObsToolCallError(t, s.mcpClient, "execute_range_query", map[string]any{
				"query":    `nonexistent_metric_xyz{job="test"}`,
				"step":     "1m",
				"duration": "5m",
			})
			return ctx
		}).
		Assess("guardrails block dangerous query", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "execute_range_query", map[string]any{
				"query":    `{__name__=~".+"}`,
				"step":     "1m",
				"duration": "5m",
			})
			return ctx
		}).
		Assess("get_label_names returns common labels", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "get_label_names", map[string]any{
				"metric": "up",
			})
			require.Contains(t, output, "job")
			require.Contains(t, output, "instance")
			return ctx
		}).
		Assess("get_label_names with time range", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "get_label_names", map[string]any{
				"metric": "up",
				"start":  "NOW-1h",
				"end":    "NOW",
			})
			require.Contains(t, output, "job")
			return ctx
		}).
		Assess("get_label_names for all metrics", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "get_label_names", map[string]any{})
			return ctx
		}).
		Assess("get_label_values returns prometheus job", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "get_label_values", map[string]any{
				"label":  "job",
				"metric": "up",
			})
			require.Contains(t, output, "prometheus")
			return ctx
		}).
		Assess("get_label_values with time range", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "get_label_values", map[string]any{
				"label":  "job",
				"metric": "up",
				"start":  "NOW-1h",
				"end":    "NOW",
			})
			require.Contains(t, output, "prometheus")
			return ctx
		}).
		Assess("get_label_values rejects missing label param", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "get_label_values", map[string]any{
				"metric": "up",
			})
			return ctx
		}).
		Assess("get_series returns cardinality info", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "get_series", map[string]any{
				"matches": `up{job="prometheus"}`,
			})
			require.True(t, strings.Contains(output, "cardinality") || strings.Contains(output, "series"),
				"expected cardinality or series info in output")
			return ctx
		}).
		Assess("get_series with time range", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "get_series", map[string]any{
				"matches": `up{job="prometheus"}`,
				"start":   "NOW-1h",
				"end":     "NOW",
			})
			require.True(t, strings.Contains(output, "cardinality") || strings.Contains(output, "series"))
			return ctx
		}).
		Assess("get_series rejects missing matches param", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "get_series", map[string]any{})
			return ctx
		}).
		Assess("get_alerts returns successfully", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "get_alerts", map[string]any{})
			return ctx
		}).
		Assess("get_alerts with active filter", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "get_alerts", map[string]any{
				"active": true,
			})
			return ctx
		}).
		Assess("get_alerts with Watchdog filter", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "get_alerts", map[string]any{
				"filter": "alertname=Watchdog",
			})
			return ctx
		}).
		Assess("get_alerts with non-existent filter returns empty", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "get_alerts", map[string]any{
				"filter": "alertname=NonExistentAlert12345",
			})
			return ctx
		}).
		Assess("get_silences returns successfully", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			output := requireObsToolCallSuccess(t, s.mcpClient, "get_silences", map[string]any{})
			require.Contains(t, output, "silences")
			return ctx
		}).
		Assess("get_silences with filter", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "get_silences", map[string]any{
				"filter": "alertname=Watchdog",
			})
			return ctx
		}).
		Assess("get_silences with non-matching filter returns empty", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "get_silences", map[string]any{
				"filter": "alertname=NonExistentSilence12345",
			})
			return ctx
		}).
		Assess("get_alerts with boolean filters", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			for _, filter := range []string{"silenced", "inhibited", "unprocessed"} {
				requireObsToolCallSuccess(t, s.mcpClient, "get_alerts", map[string]any{
					filter: true,
				})
			}
			return ctx
		}).
		Assess("get_alerts with receiver param", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsMetricsTS.get(ctx)
			requireObsToolCallSuccess(t, s.mcpClient, "get_alerts", map[string]any{
				"receiver": "nonexistent-receiver-xyz",
			})
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
