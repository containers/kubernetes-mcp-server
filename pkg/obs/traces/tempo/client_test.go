package tempo

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/search", r.URL.Path)
		require.Equal(t, "application/vnd.grafana.llm", r.Header.Get("Accept"))

		q := r.URL.Query()
		require.Equal(t, "{duration > 1s}", q.Get("q"))
		require.Equal(t, "50", q.Get("limit"))
		require.Equal(t, "1000", q.Get("start"))
		require.Equal(t, "2000", q.Get("end"))
		require.Equal(t, "5", q.Get("spss"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"traces":[]}`))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	result, err := client.Search(t.Context(), SearchOptions{
		Query: "{duration > 1s}",
		Limit: 50,
		Start: 1000,
		End:   2000,
		Spss:  5,
	})
	require.NoError(t, err)
	require.Equal(t, `{"traces":[]}`, result)
}

func TestSearch_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	_, err := client.Search(t.Context(), SearchOptions{})
	require.ErrorContains(t, err, "status 500")
}

func TestQueryV2_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/traces/abc123", r.URL.Path)
		require.Equal(t, "application/vnd.grafana.llm", r.Header.Get("Accept"))

		q := r.URL.Query()
		require.Equal(t, "1000", q.Get("start"))
		require.Equal(t, "2000", q.Get("end"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"trace":{}}`))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	result, err := client.QueryV2(t.Context(), "abc123", QueryV2Options{Start: 1000, End: 2000})
	require.NoError(t, err)
	require.Equal(t, `{"trace":{}}`, result)
}

func TestQueryV2_NoTimeRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/traces/xyz", r.URL.Path)
		q := r.URL.Query()
		require.Empty(t, q.Get("start"))
		require.Empty(t, q.Get("end"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"trace":{}}`))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	_, err := client.QueryV2(t.Context(), "xyz", QueryV2Options{})
	require.NoError(t, err)
}

func TestQueryV2_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("trace not found"))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	_, err := client.QueryV2(t.Context(), "missing", QueryV2Options{})
	require.ErrorContains(t, err, "status 404")
}

func TestSearchTagsV2_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/search/tags", r.URL.Path)
		require.Equal(t, "application/vnd.grafana.llm", r.Header.Get("Accept"))

		q := r.URL.Query()
		require.Equal(t, "resource", q.Get("scope"))
		require.Equal(t, `{ resource.service.name="svc" }`, q.Get("q"))
		require.Equal(t, "1000", q.Get("start"))
		require.Equal(t, "2000", q.Get("end"))
		require.Equal(t, "100", q.Get("limit"))
		require.Equal(t, "5", q.Get("maxStaleValues"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"scopes":[]}`))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	result, err := client.SearchTagsV2(t.Context(), SearchTagsV2Options{
		Scope:          "resource",
		Query:          `{ resource.service.name="svc" }`,
		Start:          1000,
		End:            2000,
		Limit:          100,
		MaxStaleValues: 5,
	})
	require.NoError(t, err)
	require.Equal(t, `{"scopes":[]}`, result)
}

func TestSearchTagsV2_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	_, err := client.SearchTagsV2(t.Context(), SearchTagsV2Options{})
	require.ErrorContains(t, err, "status 500")
}

func TestSearchTagValuesV2_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/search/tag/resource.service.name/values", r.URL.Path)
		require.Equal(t, "application/vnd.grafana.llm", r.Header.Get("Accept"))

		q := r.URL.Query()
		require.Equal(t, `{ status=error }`, q.Get("q"))
		require.Equal(t, "500", q.Get("start"))
		require.Equal(t, "1500", q.Get("end"))
		require.Equal(t, "50", q.Get("limit"))
		require.Equal(t, "10", q.Get("maxStaleValues"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tagValues":{"string":["frontend","backend"]}}`))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	result, err := client.SearchTagValuesV2(t.Context(), "resource.service.name", SearchTagValuesV2Options{
		Query:          `{ status=error }`,
		Start:          500,
		End:            1500,
		Limit:          50,
		MaxStaleValues: 10,
	})
	require.NoError(t, err)
	require.Equal(t, `{"tagValues":{"string":["frontend","backend"]}}`, result)
}

func TestSearchTagValuesV2_TagURLEncoded(t *testing.T) {
	// Use a tag containing '/' which url.PathEscape must encode as '%2F'.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/search/tag/span%2Fhttp.response%2Fstatus/values", r.URL.EscapedPath())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tagValues":{}}`))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	_, err := client.SearchTagValuesV2(t.Context(), "span/http.response/status", SearchTagValuesV2Options{})
	require.NoError(t, err)
}

func TestSearchTagValuesV2_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	_, err := client.SearchTagValuesV2(t.Context(), "resource.service.name", SearchTagValuesV2Options{})
	require.ErrorContains(t, err, "status 500")
}

func TestDoRequest_HTMLResponseIsAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>Login required</body></html>`))
	}))
	defer server.Close()

	client := NewTempoClient(server.Client(), server.URL)
	_, err := client.Search(t.Context(), SearchOptions{})
	require.ErrorContains(t, err, "invalid authentication token")
}
