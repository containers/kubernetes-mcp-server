package tempo

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoaderSearch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"traces":[]}`))
	}))
	defer server.Close()

	loader := NewTempoLoader(server.Client(), server.URL)
	result, err := loader.Search(t.Context(), SearchOptions{
		Query: "{}",
	})
	require.NoError(t, err)
	require.Equal(t, `{"traces":[]}`, result)
}

func TestSearch_LimitExceedsMax(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.Search(t.Context(), SearchOptions{
		Limit: MaxTraceLimit + 1,
	})
	require.ErrorContains(t, err, "greater than max limit")
}

func TestSearch_InvalidTimeRange(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.Search(t.Context(), SearchOptions{Start: 2000, End: 1000})
	require.ErrorContains(t, err, "start is after end")
}

func TestSearch_NegativeLimit(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.Search(t.Context(), SearchOptions{Limit: -1})
	require.ErrorContains(t, err, "non-negative")
}

func TestSearch_SpssExceedsMax(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.Search(t.Context(), SearchOptions{Spss: MaxSpssLimit + 1})
	require.ErrorContains(t, err, "greater than max limit")
}

func TestSearch_NegativeSpss(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.Search(t.Context(), SearchOptions{Spss: -1})
	require.ErrorContains(t, err, "non-negative")
}

func TestQueryV2_InvalidTimeRange(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.QueryV2(t.Context(), "traceid", QueryV2Options{Start: 5000, End: 1000})
	require.ErrorContains(t, err, "start is after end")
}

func TestLoaderQueryV2_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"trace":{}}`))
	}))
	defer server.Close()

	loader := NewTempoLoader(server.Client(), server.URL)
	result, err := loader.QueryV2(t.Context(), "abc123", QueryV2Options{})
	require.NoError(t, err)
	require.Equal(t, `{"trace":{}}`, result)
}

func TestLoaderSearchTagsV2_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"scopes":[]}`))
	}))
	defer server.Close()

	loader := NewTempoLoader(server.Client(), server.URL)
	result, err := loader.SearchTagsV2(t.Context(), SearchTagsV2Options{Scope: "resource"})
	require.NoError(t, err)
	require.Equal(t, `{"scopes":[]}`, result)
}

func TestSearchTagsV2_LimitExceedsMax(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.SearchTagsV2(t.Context(), SearchTagsV2Options{Limit: MaxTagNamesLimit + 1})
	require.ErrorContains(t, err, "greater than max limit")
}

func TestSearchTagsV2_InvalidTimeRange(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.SearchTagsV2(t.Context(), SearchTagsV2Options{Start: 2000, End: 1000})
	require.ErrorContains(t, err, "start is after end")
}

func TestSearchTagsV2_NegativeMaxStaleValues(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.SearchTagsV2(t.Context(), SearchTagsV2Options{MaxStaleValues: -1})
	require.ErrorContains(t, err, "non-negative")
}

func TestLoaderSearchTagValuesV2_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tagValues":{"string":["frontend"]}}`))
	}))
	defer server.Close()

	loader := NewTempoLoader(server.Client(), server.URL)
	result, err := loader.SearchTagValuesV2(t.Context(), "resource.service.name", SearchTagValuesV2Options{})
	require.NoError(t, err)
	require.Equal(t, `{"tagValues":{"string":["frontend"]}}`, result)
}

func TestSearchTagValuesV2_LimitExceedsMax(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.SearchTagValuesV2(t.Context(), "resource.service.name", SearchTagValuesV2Options{
		Limit: MaxTagValuesLimit + 1,
	})
	require.ErrorContains(t, err, "greater than max limit")
}

func TestSearchTagValuesV2_InvalidTimeRange(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.SearchTagValuesV2(t.Context(), "resource.service.name", SearchTagValuesV2Options{
		Start: 3000, End: 1000,
	})
	require.ErrorContains(t, err, "start is after end")
}

func TestSearchTagValuesV2_NegativeMaxStaleValues(t *testing.T) {
	loader := NewTempoLoader(nil, "http://unused")
	_, err := loader.SearchTagValuesV2(t.Context(), "resource.service.name", SearchTagValuesV2Options{
		MaxStaleValues: -1,
	})
	require.ErrorContains(t, err, "non-negative")
}
