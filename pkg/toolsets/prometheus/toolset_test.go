package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	p8s_api "github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func TestPrometheusToolset_RunQuery(t *testing.T) {
	// Mock Prometheus server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/query", r.URL.Path)
		err := r.ParseForm()
		require.NoError(t, err)
		query := r.Form.Get("query")
		assert.Equal(t, "up", query)

		// Send back a mock response
		w.WriteHeader(http.StatusOK)
		response := fmt.Sprintf(`{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[{
					"metric":{"__name__":"up","job":"prometheus"},
					"value":[%f,"1"]
				}]
			}
		}`, float64(time.Now().Unix()))
		_, _ = w.Write([]byte(response))
	}))
	defer mockServer.Close()

	// Create a new toolset with the mock server's URL
	client, err := p8s_api.NewClient(p8s_api.Config{
		Address: mockServer.URL,
	})
	require.NoError(t, err)

	toolset := &prometheusToolset{
		api: v1.NewAPI(client),
	}

	// Get the runQuery tool
	tools := toolset.GetTools(nil)
	require.Len(t, tools, 1)
	runQueryTool := tools[0]

	// Create handler parameters
	params := api.ToolHandlerParams{
		Context: context.Background(),
		ToolCallRequest: &mockToolCallRequest{
			args: map[string]any{
				"query": "up",
			},
		},
	}

	// Call the handler
	result, err := runQueryTool.Handler(params)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the result content
	assert.Contains(t, result.Content, "up{job=\"prometheus\"} => 1")
}

// mockToolCallRequest is a simple implementation of api.ToolCallRequest for testing.
type mockToolCallRequest struct {
	args map[string]any
}

func (r *mockToolCallRequest) GetArguments() map[string]any {
	return r.args
}