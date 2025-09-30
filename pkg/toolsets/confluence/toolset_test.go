package confluence

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfluenceToolset_CreatePage(t *testing.T) {
	// Mock Confluence server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/content/", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var content goconfluence.Content
		err := json.NewDecoder(r.Body).Decode(&content)
		require.NoError(t, err)

		assert.Equal(t, "TEST", content.Space.Key)
		assert.Equal(t, "My Test Page", content.Title)
		assert.Equal(t, "<p>Hello, World!</p>", content.Body.Storage.Value)

		// Send back a mock response
		w.WriteHeader(http.StatusOK)
		response := goconfluence.Content{
			ID:    "12345",
			Title: "My Test Page",
			Links: &goconfluence.Links{
				WebUI: "/display/TEST/My+Test+Page",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Create a new toolset with the mock server's URL
	confluenceAPI, err := goconfluence.NewAPI(mockServer.URL, "user", "token")
	require.NoError(t, err)

	toolset := &confluenceToolset{api: confluenceAPI}

	// Get the createPage tool
	tools := toolset.GetTools(nil)
	require.Len(t, tools, 1)
	createPageTool := tools[0]

	// Create handler parameters
	params := api.ToolHandlerParams{
		Context: context.Background(),
		ToolCallRequest: &mockToolCallRequest{
			args: map[string]any{
				"space_key": "TEST",
				"title":     "My Test Page",
				"content":   "<p>Hello, World!</p>",
			},
		},
	}

	// Call the handler
	result, err := createPageTool.Handler(params)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result.Content, "Page created successfully")
	assert.Contains(t, result.Content, "/display/TEST/My+Test+Page")
}

// mockToolCallRequest is a simple implementation of api.ToolCallRequest for testing.
type mockToolCallRequest struct {
	args map[string]any
}

func (r *mockToolCallRequest) GetArguments() map[string]any {
	return r.args
}