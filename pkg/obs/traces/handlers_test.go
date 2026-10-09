package traces

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/require"
)

// tempoServer starts an httptest server that responds to Tempo API paths with the given responses.
// Supported keys: "search", "traces/{traceID}", "api/v2/search/tags", "api/v2/search/tag/{tag}/values".
func tempoServer(t *testing.T, responses map[string]mockResponse) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		for pattern, resp := range responses {
			if pathMatches(path, pattern) {
				if resp.err != "" {
					http.Error(w, resp.err, http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, resp.body)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type mockResponse struct {
	body string
	err  string
}

func pathMatches(actual, pattern string) bool {
	switch pattern {
	case "search":
		return actual == "/api/search"
	case "traces":
		return strings.HasPrefix(actual, "/api/v2/traces/")
	case "tags":
		return actual == "/api/v2/search/tags"
	case "tag_values":
		return strings.HasPrefix(actual, "/api/v2/search/tag/")
	}
	return false
}

func handlerParams(t *testing.T, tempoURL string, args map[string]any) api.ToolHandlerParams {
	t.Helper()
	return newTestParams(t, &Config{TempoURL: tempoURL}, nil, args)
}

func discoveryParams(t *testing.T, args map[string]any) api.ToolHandlerParams {
	t.Helper()
	fakeClient := newMockK8sClient(newTempoStack("ns", "tempo", []string{}))
	return newTestParams(t, &Config{UseRoute: false}, fakeClient, args)
}

// --- SearchTracesHandler ---

func TestSearchTracesHandler_Success(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"search": {body: `{"traces":[{"traceID":"abc"}],"metrics":{}}`},
	})
	params := handlerParams(t, srv.URL, map[string]any{"query": "{}"})

	result, err := searchTracesHandler(params)
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(searchTracesOutput)
	require.Len(t, output.Traces, 1)
}

func TestSearchTracesHandler_EmptyQuery(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTracesHandler(handlerParams(t, srv.URL, map[string]any{"query": ""}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "query parameter must not be empty")
}

func TestSearchTracesHandler_MissingQuery(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTracesHandler(handlerParams(t, srv.URL, map[string]any{}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "query parameter")
}

func TestSearchTracesHandler_BackendError(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"search": {err: "tempo unavailable"},
	})
	result, err := searchTracesHandler(handlerParams(t, srv.URL, map[string]any{"query": "{}"}))
	require.NoError(t, err)
	require.Error(t, result.Error)
}

func TestSearchTracesHandler_InvalidStartTime(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTracesHandler(handlerParams(t, srv.URL, map[string]any{
		"query": "{}",
		"start": "not-a-timestamp",
	}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "invalid start time")
}

func TestSearchTracesHandler_InvalidEndTime(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTracesHandler(handlerParams(t, srv.URL, map[string]any{
		"query": "{}",
		"end":   "not-a-timestamp",
	}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "invalid end time")
}

// --- GetTraceByIDHandler ---

func TestGetTraceByIDHandler_Success(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"traces": {body: `{"trace":{"traceID":"abc123","services":[]}}`},
	})
	params := handlerParams(t, srv.URL, map[string]any{"traceid": "abc123"})

	result, err := getTraceByIDHandler(params)
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(getTraceByIDOutput)
	require.NotNil(t, output.Trace)
}

func TestGetTraceByIDHandler_EmptyTraceID(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := getTraceByIDHandler(handlerParams(t, srv.URL, map[string]any{"traceid": ""}))
	require.NoError(t, err)
	require.Error(t, result.Error)
}

func TestGetTraceByIDHandler_MissingTraceID(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := getTraceByIDHandler(handlerParams(t, srv.URL, map[string]any{}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "traceid parameter")
}

func TestGetTraceByIDHandler_BackendError(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"traces": {err: "trace not found"},
	})
	result, err := getTraceByIDHandler(handlerParams(t, srv.URL, map[string]any{"traceid": "deadbeef"}))
	require.NoError(t, err)
	require.Error(t, result.Error)
}

func TestGetTraceByIDHandler_InvalidStartTime(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := getTraceByIDHandler(handlerParams(t, srv.URL, map[string]any{
		"traceid": "abc",
		"start":   "bad-time",
	}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "invalid start time")
}

func TestGetTraceByIDHandler_InvalidEndTime(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := getTraceByIDHandler(handlerParams(t, srv.URL, map[string]any{
		"traceid": "abc",
		"end":     "bad-time",
	}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "invalid end time")
}

func TestGetTraceByIDHandler_NullTrace(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"traces": {body: `{"trace":null}`},
	})
	params := handlerParams(t, srv.URL, map[string]any{"traceid": "00000000000000000000000000000000"})
	result, err := getTraceByIDHandler(params)
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "not found")
}

// --- SearchTagsHandler ---

func TestSearchTagsHandler_Success(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"tags": {body: `{"scopes":[{"name":"resource","tags":["service.name"]}]}`},
	})
	result, err := searchTagsHandler(handlerParams(t, srv.URL, map[string]any{}))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(searchTagsOutput)
	require.Len(t, output.Scopes, 1)
}

func TestSearchTagsHandler_WithScope(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"tags": {body: `{"scopes":[{"name":"span","tags":["http.method"]}]}`},
	})
	result, err := searchTagsHandler(handlerParams(t, srv.URL, map[string]any{"scope": "span"}))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(searchTagsOutput)
	require.Len(t, output.Scopes, 1)
}

func TestSearchTagsHandler_BackendError(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"tags": {err: "backend error"},
	})
	result, err := searchTagsHandler(handlerParams(t, srv.URL, map[string]any{}))
	require.NoError(t, err)
	require.Error(t, result.Error)
}

func TestSearchTagsHandler_InvalidStartTime(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTagsHandler(handlerParams(t, srv.URL, map[string]any{
		"start": "not-valid",
	}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "invalid start time")
}

func TestSearchTagsHandler_InvalidEndTime(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTagsHandler(handlerParams(t, srv.URL, map[string]any{
		"end": "not-valid",
	}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "invalid end time")
}

// --- SearchTagValuesHandler ---

func TestSearchTagValuesHandler_Success(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"tag_values": {body: `{"tagValues":{"string":["frontend","backend"]}}`},
	})
	result, err := searchTagValuesHandler(handlerParams(t, srv.URL, map[string]any{"tag": "resource.service.name"}))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(searchTagValuesOutput)
	require.NotNil(t, output.TagValues)
}

func TestSearchTagValuesHandler_EmptyTag(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTagValuesHandler(handlerParams(t, srv.URL, map[string]any{"tag": ""}))
	require.NoError(t, err)
	require.Error(t, result.Error)
}

func TestSearchTagValuesHandler_MissingTag(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTagValuesHandler(handlerParams(t, srv.URL, map[string]any{}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "tag parameter")
}

func TestSearchTagValuesHandler_BackendError(t *testing.T) {
	srv := tempoServer(t, map[string]mockResponse{
		"tag_values": {err: "values unavailable"},
	})
	result, err := searchTagValuesHandler(handlerParams(t, srv.URL, map[string]any{"tag": "resource.service.name"}))
	require.NoError(t, err)
	require.Error(t, result.Error)
}

func TestSearchTagValuesHandler_InvalidEndTime(t *testing.T) {
	srv := tempoServer(t, nil)
	result, err := searchTagValuesHandler(handlerParams(t, srv.URL, map[string]any{
		"tag": "resource.service.name",
		"end": "bad-time",
	}))
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "invalid end time")
}

// --- Static TempoURL ---

func TestHandler_StaticTempoURL(t *testing.T) {
	var capturedHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHost = r.Host
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"traces":[{"traceID":"abc"}],"metrics":{}}`)
	}))
	t.Cleanup(srv.Close)

	params := handlerParams(t, srv.URL, map[string]any{"query": "{}"})

	result, err := searchTracesHandler(params)
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(searchTracesOutput)
	require.Len(t, output.Traces, 1)
	require.NotEmpty(t, capturedHost)
}

func TestHandler_NoURLAndNoDiscoveryParams(t *testing.T) {
	params := newTestParams(t, &Config{}, nil, map[string]any{"query": "{}"})

	result, err := searchTracesHandler(params)
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "tempo URL not configured")
}

// --- Instance resolution errors ---

func TestHandler_UnknownInstance(t *testing.T) {
	params := discoveryParams(t, map[string]any{
		"tempoNamespace": "ns",
		"tempoName":      "does-not-exist",
		"query":          "{}",
	})

	result, err := searchTracesHandler(params)
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "not found")
}

func TestHandler_MultitenantMissingTenant(t *testing.T) {
	fakeClient := newMockK8sClient(newTempoStack("ns", "mt-tempo", []string{"dev", "prod"}))
	params := newTestParams(t, &Config{UseRoute: false}, fakeClient, map[string]any{
		"tempoNamespace": "ns",
		"tempoName":      "mt-tempo",
		"query":          "{}",
	})

	result, err := searchTracesHandler(params)
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "tenant parameter must not be empty")
}
