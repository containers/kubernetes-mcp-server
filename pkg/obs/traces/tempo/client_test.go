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
