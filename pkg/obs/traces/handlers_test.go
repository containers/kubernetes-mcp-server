package traces

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// mockLoader is a test double for tempoclient.Loader that returns configurable responses.
type mockLoader struct {
	searchResult       string
	searchErr          error
	queryV2Result      string
	queryV2Err         error
	searchTagsResult   string
	searchTagsErr      error
	searchValuesResult string
	searchValuesErr    error
}

func (m *mockLoader) Search(_ context.Context, _ tempoclient.SearchOptions) (string, error) {
	return m.searchResult, m.searchErr
}

func (m *mockLoader) QueryV2(_ context.Context, _ string, _ tempoclient.QueryV2Options) (string, error) {
	return m.queryV2Result, m.queryV2Err
}

func (m *mockLoader) SearchTagsV2(_ context.Context, _ tempoclient.SearchTagsV2Options) (string, error) {
	return m.searchTagsResult, m.searchTagsErr
}

func (m *mockLoader) SearchTagValuesV2(_ context.Context, _ string, _ tempoclient.SearchTagValuesV2Options) (string, error) {
	return m.searchValuesResult, m.searchValuesErr
}

// toolParams builds a ToolParams that wires up the mock loader and a fake k8s client
// pre-populated with a single non-multitenant TempoStack in namespace "ns" named "tempo".
func toolParams(t *testing.T, loader tempoclient.Loader) ToolParams {
	t.Helper()
	fakeClient := newMockK8sClient(newTempoStack("ns", "tempo", []string{}))
	return ToolParams{
		context:       t.Context(),
		dynamicClient: fakeClient,
		config:        &Config{UseRoute: false},
		newTempoLoader: func(_ string) (tempoclient.Loader, error) {
			return loader, nil
		},
		arguments: map[string]any{
			"tempoNamespace": "ns",
			"tempoName":      "tempo",
		},
	}
}

// withArgs returns a copy of p with the given extra arguments merged in.
func withArgs(p ToolParams, extra map[string]any) ToolParams {
	merged := make(map[string]any, len(p.arguments)+len(extra))
	maps.Copy(merged, p.arguments)
	maps.Copy(merged, extra)
	p.arguments = merged
	return p
}

// --- SearchTracesHandler ---

func TestSearchTracesHandler_Success(t *testing.T) {
	mock := &mockLoader{searchResult: `{"traces":[{"traceID":"abc"}],"metrics":{}}`}
	params := withArgs(toolParams(t, mock), map[string]any{"query": "{}"})

	toolset := &Toolset{}
	output, err := toolset.SearchTracesHandler(params)
	require.NoError(t, err)
	require.Len(t, output.Traces, 1)
}

func TestSearchTracesHandler_EmptyQuery(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTracesHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{"query": ""}))
	require.ErrorContains(t, err, "query parameter must not be empty")
}

func TestSearchTracesHandler_MissingQuery(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTracesHandler(toolParams(t, &mockLoader{}))
	require.ErrorContains(t, err, "query parameter must not be empty")
}

func TestSearchTracesHandler_BackendError(t *testing.T) {
	mock := &mockLoader{searchErr: errors.New("tempo unavailable")}
	toolset := &Toolset{}
	_, err := toolset.SearchTracesHandler(withArgs(toolParams(t, mock), map[string]any{"query": "{}"}))
	require.ErrorContains(t, err, "tempo unavailable")
}

func TestSearchTracesHandler_InvalidStartTime(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTracesHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{
		"query": "{}",
		"start": "not-a-timestamp",
	}))
	require.ErrorContains(t, err, "invalid start time")
}

func TestSearchTracesHandler_InvalidEndTime(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTracesHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{
		"query": "{}",
		"end":   "not-a-timestamp",
	}))
	require.ErrorContains(t, err, "invalid end time")
}

// --- GetTraceByIDHandler ---

func TestGetTraceByIDHandler_Success(t *testing.T) {
	mock := &mockLoader{queryV2Result: `{"trace":{"traceID":"abc123","services":[]}}`}
	params := withArgs(toolParams(t, mock), map[string]any{"traceid": "abc123"})

	toolset := &Toolset{}
	output, err := toolset.GetTraceByIDHandler(params)
	require.NoError(t, err)
	require.NotNil(t, output.Trace)
}

func TestGetTraceByIDHandler_EmptyTraceID(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.GetTraceByIDHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{"traceid": ""}))
	require.ErrorContains(t, err, "traceid parameter must not be empty")
}

func TestGetTraceByIDHandler_MissingTraceID(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.GetTraceByIDHandler(toolParams(t, &mockLoader{}))
	require.ErrorContains(t, err, "traceid parameter must not be empty")
}

func TestGetTraceByIDHandler_BackendError(t *testing.T) {
	mock := &mockLoader{queryV2Err: errors.New("trace not found")}
	toolset := &Toolset{}
	_, err := toolset.GetTraceByIDHandler(withArgs(toolParams(t, mock), map[string]any{"traceid": "deadbeef"}))
	require.ErrorContains(t, err, "trace not found")
}

func TestGetTraceByIDHandler_InvalidStartTime(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.GetTraceByIDHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{
		"traceid": "abc",
		"start":   "bad-time",
	}))
	require.ErrorContains(t, err, "invalid start time")
}

func TestGetTraceByIDHandler_InvalidEndTime(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.GetTraceByIDHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{
		"traceid": "abc",
		"end":     "bad-time",
	}))
	require.ErrorContains(t, err, "invalid end time")
}

func TestGetTraceByIDHandler_NullTrace(t *testing.T) {
	mock := &mockLoader{queryV2Result: `{"trace":null}`}
	params := withArgs(toolParams(t, mock), map[string]any{"traceid": "00000000000000000000000000000000"})
	toolset := &Toolset{}
	_, err := toolset.GetTraceByIDHandler(params)
	require.ErrorContains(t, err, "not found")
}

// --- SearchTagsHandler ---

func TestSearchTagsHandler_Success(t *testing.T) {
	mock := &mockLoader{searchTagsResult: `{"scopes":[{"name":"resource","tags":["service.name"]}]}`}
	toolset := &Toolset{}
	output, err := toolset.SearchTagsHandler(toolParams(t, mock))
	require.NoError(t, err)
	require.Len(t, output.Scopes, 1)
}

func TestSearchTagsHandler_WithScope(t *testing.T) {
	mock := &mockLoader{searchTagsResult: `{"scopes":[{"name":"span","tags":["http.method"]}]}`}
	params := withArgs(toolParams(t, mock), map[string]any{"scope": "span"})
	toolset := &Toolset{}
	output, err := toolset.SearchTagsHandler(params)
	require.NoError(t, err)
	require.Len(t, output.Scopes, 1)
}

func TestSearchTagsHandler_BackendError(t *testing.T) {
	mock := &mockLoader{searchTagsErr: errors.New("backend error")}
	toolset := &Toolset{}
	_, err := toolset.SearchTagsHandler(toolParams(t, mock))
	require.ErrorContains(t, err, "backend error")
}

func TestSearchTagsHandler_InvalidStartTime(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTagsHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{
		"start": "not-valid",
	}))
	require.ErrorContains(t, err, "invalid start time")
}

func TestSearchTagsHandler_InvalidEndTime(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTagsHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{
		"end": "not-valid",
	}))
	require.ErrorContains(t, err, "invalid end time")
}

// --- SearchTagValuesHandler ---

func TestSearchTagValuesHandler_Success(t *testing.T) {
	mock := &mockLoader{searchValuesResult: `{"tagValues":{"string":["frontend","backend"]}}`}
	params := withArgs(toolParams(t, mock), map[string]any{"tag": "resource.service.name"})
	toolset := &Toolset{}
	output, err := toolset.SearchTagValuesHandler(params)
	require.NoError(t, err)
	require.NotNil(t, output.TagValues)
}

func TestSearchTagValuesHandler_EmptyTag(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTagValuesHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{"tag": ""}))
	require.ErrorContains(t, err, "tag parameter must not be empty")
}

func TestSearchTagValuesHandler_MissingTag(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTagValuesHandler(toolParams(t, &mockLoader{}))
	require.ErrorContains(t, err, "tag parameter must not be empty")
}

func TestSearchTagValuesHandler_BackendError(t *testing.T) {
	mock := &mockLoader{searchValuesErr: errors.New("values unavailable")}
	params := withArgs(toolParams(t, mock), map[string]any{"tag": "resource.service.name"})
	toolset := &Toolset{}
	_, err := toolset.SearchTagValuesHandler(params)
	require.ErrorContains(t, err, "values unavailable")
}

func TestSearchTagValuesHandler_InvalidEndTime(t *testing.T) {
	toolset := &Toolset{}
	_, err := toolset.SearchTagValuesHandler(withArgs(toolParams(t, &mockLoader{}), map[string]any{
		"tag": "resource.service.name",
		"end": "bad-time",
	}))
	require.ErrorContains(t, err, "invalid end time")
}

// --- Instance resolution errors ---

func TestHandler_UnknownInstance(t *testing.T) {
	fakeClient := newMockK8sClient(newTempoStack("ns", "tempo", []string{}))
	params := ToolParams{
		context:       t.Context(),
		dynamicClient: fakeClient,
		config:        &Config{UseRoute: false},
		newTempoLoader: func(_ string) (tempoclient.Loader, error) {
			return &mockLoader{}, nil
		},
		arguments: map[string]any{
			"tempoNamespace": "ns",
			"tempoName":      "does-not-exist",
			"query":          "{}",
		},
	}

	toolset := &Toolset{}
	_, err := toolset.SearchTracesHandler(params)
	require.ErrorContains(t, err, "not found")
}

func TestHandler_MultitenantMissingTenant(t *testing.T) {
	fakeClient := newMockK8sClient(newTempoStack("ns", "mt-tempo", []string{"dev", "prod"}))
	params := ToolParams{
		context:       t.Context(),
		dynamicClient: fakeClient,
		config:        &Config{UseRoute: false},
		newTempoLoader: func(_ string) (tempoclient.Loader, error) {
			return &mockLoader{}, nil
		},
		arguments: map[string]any{
			"tempoNamespace": "ns",
			"tempoName":      "mt-tempo",
			"query":          "{}",
		},
	}

	toolset := &Toolset{}
	_, err := toolset.SearchTracesHandler(params)
	require.ErrorContains(t, err, "tenant parameter must not be empty")
}
