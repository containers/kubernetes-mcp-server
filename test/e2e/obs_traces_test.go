//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type obsTracesState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var obsTracesTS testState[obsTracesState]

func TestObsTracesTools(t *testing.T) {
	f := features.New("obs-traces-tools").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "obs-traces",
				withConfig(obsTracesConfig()),
				withValues(obsTracesRBACValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return obsTracesTS.set(ctx, &obsTracesState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("tempo_list_instances returns instances", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsTracesTS.get(ctx)
			output := requireObsToolCallWithRetry(t, s.mcpClient, "tempo_list_instances", map[string]any{})
			require.Contains(t, output, "tempo1")
			require.Contains(t, output, "tempo2")
			return ctx
		}).
		Assess("tempo_search_tags returns scopes", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsTracesTS.get(ctx)
			output := requireObsToolCallWithRetry(t, s.mcpClient, "tempo_search_tags", map[string]any{
				"tempoNamespace": "tracing",
				"tempoName":      "tempo1",
			})
			require.Contains(t, output, "resource")
			return ctx
		}).
		Assess("tempo_search_traces returns traces", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsTracesTS.get(ctx)
			output := requireObsToolCallWithRetry(t, s.mcpClient, "tempo_search_traces", map[string]any{
				"tempoNamespace": "tracing",
				"tempoName":      "tempo1",
				"query":          "{}",
			})
			require.Contains(t, output, "traceID")
			return ctx
		}).
		Assess("tempo_search_traces rejects empty query", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsTracesTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "tempo_search_traces", map[string]any{
				"tempoNamespace": "tracing",
				"tempoName":      "tempo1",
				"query":          "",
			})
			return ctx
		}).
		Assess("tempo_get_trace_by_id fetches a real trace", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsTracesTS.get(ctx)
			// First search for a trace to get a real ID.
			searchOutput := requireObsToolCallWithRetry(t, s.mcpClient, "tempo_search_traces", map[string]any{
				"tempoNamespace": "tracing",
				"tempoName":      "tempo1",
				"query":          "{}",
				"limit":          1,
			})
			// textContent is the structured payload JSON (e.g. {"traces":[...]}).
			var searchResult map[string]any
			require.NoError(t, json.Unmarshal([]byte(searchOutput), &searchResult), "parse tempo_search_traces output")
			traces, _ := searchResult["traces"].([]any)
			require.NotEmpty(t, traces, "expected at least one trace from tempo_search_traces")
			firstTrace, _ := traces[0].(map[string]any)
			traceID, _ := firstTrace["traceID"].(string)
			require.NotEmpty(t, traceID, "expected traceID in search result")

			output := requireObsToolCallWithRetry(t, s.mcpClient, "tempo_get_trace_by_id", map[string]any{
				"tempoNamespace": "tracing",
				"tempoName":      "tempo1",
				"traceid":        traceID,
			})
			require.Contains(t, output, "trace")
			return ctx
		}).
		Assess("tempo_get_trace_by_id with non-existent ID returns error", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsTracesTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "tempo_get_trace_by_id", map[string]any{
				"tempoNamespace": "tracing",
				"tempoName":      "tempo1",
				"traceid":        "00000000000000000000000000000000",
			})
			return ctx
		}).
		Assess("tempo_search_tag_values returns values", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsTracesTS.get(ctx)
			output := requireObsToolCallWithRetry(t, s.mcpClient, "tempo_search_tag_values", map[string]any{
				"tempoNamespace": "tracing",
				"tempoName":      "tempo1",
				"tag":            "resource.service.name",
			})
			require.Contains(t, output, "tagValues")
			return ctx
		}).
		Assess("tempo_search_tag_values rejects missing tag", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := obsTracesTS.get(ctx)
			requireObsToolCallError(t, s.mcpClient, "tempo_search_tag_values", map[string]any{
				"tempoNamespace": "tracing",
				"tempoName":      "tempo1",
			})
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
