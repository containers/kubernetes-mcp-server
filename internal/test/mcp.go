package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

type McpClient struct {
	ctx        context.Context
	testServer *httptest.Server
	*client.Client
}

func NewMcpClient(t *testing.T, mcpHttpServer http.Handler, options ...transport.StreamableHTTPCOption) *McpClient {
	require.NotNil(t, mcpHttpServer, "McpHttpServer must be provided")
	var err error
	ret := &McpClient{ctx: t.Context()}
	ret.testServer = httptest.NewServer(mcpHttpServer)
	ret.Client, err = client.NewStreamableHttpClient(ret.testServer.URL+"/mcp", options...)
	require.NoError(t, err, "Expected no error creating MCP client")
	err = ret.Start(t.Context())
	require.NoError(t, err, "Expected no error starting MCP client")
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.33.7"}
	_, err = ret.Initialize(t.Context(), initRequest)
	require.NoError(t, err, "Expected no error initializing MCP client")
	return ret
}

func (m *McpClient) Close() {
	if m.Client != nil {
		_ = m.Client.Close()
	}
	if m.testServer != nil {
		m.testServer.Close()
	}
}

// CallTool helper function to call a tool by name with arguments
func (m *McpClient) CallTool(name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	callToolRequest := mcp.CallToolRequest{}
	callToolRequest.Params.Name = name
	callToolRequest.Params.Arguments = args
	return m.Client.CallTool(m.ctx, callToolRequest)
}
