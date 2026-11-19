//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type observabilityState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var observabilityTS testState[observabilityState]

// TestObservabilityEndpoints exercises the /metrics and /stats HTTP endpoints.
// No extra infra beyond a basic cluster is required.
func TestObservabilityEndpoints(t *testing.T) {
	f := features.New("observability-endpoints").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "observability",
				withValues(viewClusterRoleBindingValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)

			// Seed metrics counters with a tool call.
			requireToolCallSuccess(t, mcpClient, "namespaces_list", map[string]any{})

			return observabilityTS.set(ctx, &observabilityState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("/metrics returns Prometheus data", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := observabilityTS.get(ctx)

			status, body := httpGetBody(t, s.dep.serverURL+"/metrics")
			require.Equal(t, http.StatusOK, status, "/metrics status")

			require.Contains(t, body, "k8s_mcp_tool_calls_total{",
				"/metrics should contain tool call counter")
			require.Contains(t, body, `tool_name="namespaces_list"`,
				"/metrics should have namespaces_list tool_name label")
			require.Contains(t, body, "k8s_mcp_http_requests_total{",
				"/metrics should contain HTTP request counter")
			require.Contains(t, body, "k8s_mcp_server_info{",
				"/metrics should contain server info gauge")

			return ctx
		}).
		Assess("/stats returns tool call statistics", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := observabilityTS.get(ctx)

			status, body := httpGetBody(t, s.dep.serverURL+"/stats")
			require.Equal(t, http.StatusOK, status, "/stats status")

			var stats statsResponse
			require.NoError(t, json.Unmarshal([]byte(body), &stats), "decode /stats JSON")

			require.GreaterOrEqual(t, stats.TotalToolCalls, int64(1),
				"total_tool_calls should be >= 1 after a tool call")
			require.GreaterOrEqual(t, stats.ToolCallsByName["namespaces_list"], int64(1),
				"tool_calls_by_name[namespaces_list] should be >= 1")
			require.GreaterOrEqual(t, stats.TotalHTTPRequests, int64(1),
				"total_http_requests should be >= 1")
			require.GreaterOrEqual(t, stats.UptimeSeconds, int64(0),
				"uptime_seconds should be >= 0")
			require.Greater(t, stats.StartTime, int64(0),
				"start_time_unix should be > 0")

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

type tracingState struct {
	dep      *serverDeployment
	tempoURL string
}

var tracingTS testState[tracingState]

// TestObservabilityTracing exercises the tracing pipeline by deploying the MCP
// server with OpenTelemetry export enabled, making a tool call, and verifying
// that traces arrive in Tempo.
func TestObservabilityTracing(t *testing.T) {
	f := features.New("observability-tracing").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			tempoURL := requireTempo(ctx, t, cfg)

			dep := deployServer(ctx, t, cfg, "otel-tracing",
				withConfig(`
					[telemetry]
					enabled = true
					endpoint = "http://tempo.tempo.svc:4317"
					protocol = "grpc"
					traces_sampler = "always_on"
				`),
				withValues(viewClusterRoleBindingValues()),
			)

			return tracingTS.set(ctx, &tracingState{dep: dep, tempoURL: tempoURL})
		}).
		Assess("traces are exported to Tempo", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			s := tracingTS.get(ctx)

			// Create an MCP client and make a tool call to generate spans.
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(s.dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			requireToolCallSuccess(t, mcpClient, "namespaces_list", map[string]any{})

			// Poll Tempo's search API until traces appear. The OTEL
			// BatchSpanProcessor flushes every 5s; allow up to 30s.
			searchURL := fmt.Sprintf("%s/api/search?tags=%s", s.tempoURL, "service.name=kubernetes-mcp-server")
			var searchResult tempoSearchResponse
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				status, body := httpGetBody(t, searchURL)
				require.Equal(t, http.StatusOK, status, "Tempo search API status")
				require.NoError(t, json.Unmarshal([]byte(body), &searchResult), "decode Tempo search response")
				if len(searchResult.Traces) > 0 {
					break
				}
				time.Sleep(2 * time.Second)
			}
			require.NotEmpty(t, searchResult.Traces,
				"expected at least one trace in Tempo for service kubernetes-mcp-server")

			// Fetch the first trace and verify it has spans.
			traceID := searchResult.Traces[0].TraceID
			traceURL := fmt.Sprintf("%s/api/traces/%s", s.tempoURL, traceID)
			status, traceBody := httpGetBody(t, traceURL)
			require.Equal(t, http.StatusOK, status, "Tempo trace API status")

			var trace tempoTraceResponse
			require.NoError(t, json.Unmarshal([]byte(traceBody), &trace), "decode Tempo trace response")
			require.NotEmpty(t, trace.Batches,
				"expected non-empty batches (resource spans) in trace %s", traceID)

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

// requireTempo checks that Tempo is installed, skips if not, and returns a
// port-forwarded URL to the Tempo HTTP API (port 3200).
func requireTempo(ctx context.Context, t *testing.T, cfg *envconf.Config) string {
	t.Helper()
	clientset, err := clientsetFromKubeconfig(cfg.KubeconfigFile())
	require.NoError(t, err)

	_, err = clientset.CoreV1().Services("tempo").Get(ctx, "tempo", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		t.Skip("Tempo not installed — run 'make tempo-install' first")
	}
	require.NoError(t, err, "check tempo service")

	restCfg, err := clientcmd.BuildConfigFromFlags("", cfg.KubeconfigFile())
	require.NoError(t, err, "build rest config")

	tempoURL, stopPF := portForwardService(ctx, t, restCfg, clientset, "tempo", "tempo", 3200)
	t.Cleanup(stopPF)
	return tempoURL
}

// httpGetBody performs a GET request and returns the status code and body as a string.
func httpGetBody(t *testing.T, url string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	require.NoError(t, err, "GET %s", url)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read body from %s", url)
	return resp.StatusCode, string(body)
}

// statsResponse mirrors pkg/metrics.Statistics for JSON decoding.
type statsResponse struct {
	TotalToolCalls       int64            `json:"total_tool_calls"`
	ToolCallErrors       int64            `json:"tool_call_errors"`
	ToolCallsByName      map[string]int64 `json:"tool_calls_by_name"`
	ToolErrorsByName     map[string]int64 `json:"tool_errors_by_name"`
	TotalHTTPRequests    int64            `json:"total_http_requests"`
	HTTPRequestsByPath   map[string]int64 `json:"http_requests_by_path"`
	HTTPRequestsByStatus map[string]int64 `json:"http_requests_by_status"`
	HTTPRequestsByMethod map[string]int64 `json:"http_requests_by_method"`
	UptimeSeconds        int64            `json:"uptime_seconds"`
	StartTime            int64            `json:"start_time_unix"`
}

// tempoSearchResponse is the minimal Tempo search API response structure.
type tempoSearchResponse struct {
	Traces []tempoTraceMetadata `json:"traces"`
}

type tempoTraceMetadata struct {
	TraceID         string `json:"traceID"`
	RootServiceName string `json:"rootServiceName"`
	RootTraceName   string `json:"rootTraceName"`
	DurationMs      int    `json:"durationMs"`
}

// tempoTraceResponse is the minimal Tempo trace-by-ID response (OTLP JSON).
type tempoTraceResponse struct {
	Batches []json.RawMessage `json:"batches"`
}
