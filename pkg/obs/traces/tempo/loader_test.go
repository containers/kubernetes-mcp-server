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
