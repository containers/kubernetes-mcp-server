package http

import (
	"fmt"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type McpTransportSuite struct {
	BaseHttpSuite
}

func (s *McpTransportSuite) SetupTest() {
	s.BaseHttpSuite.SetupTest()
	s.StartServer()
}

func (s *McpTransportSuite) TearDownTest() {
	s.BaseHttpSuite.TearDownTest()
}

func (s *McpTransportSuite) TestSseTransport() {
	sseClient, sseClientErr := client.NewSSEMCPClient(fmt.Sprintf("http://127.0.0.1:%s/sse", s.StaticConfig.Port))
	s.Require().NoError(sseClientErr, "Expected no error creating SSE MCP client")
	startErr := sseClient.Start(s.T().Context())
	s.Require().NoError(startErr, "Expected no error starting SSE MCP client")
	s.Run("Can Initialize Session", func() {
		_, err := sseClient.Initialize(s.T().Context(), test.McpInitRequest())
		s.Require().NoError(err, "Expected no error initializing SSE MCP client")
	})
	s.Run("Can List Tools", func() {
		tools, err := sseClient.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Require().NoError(err, "Expected no error listing tools from SSE MCP client")
		s.Greater(len(tools.Tools), 0, "Expected at least one tool from SSE MCP client")
	})
	s.Run("Can close SSE client", func() {
		s.Require().NoError(sseClient.Close(), "Expected no error closing SSE MCP client")
	})
}

func (s *McpTransportSuite) TestStreamableHttpTransport() {
	httpClient, httpClientErr := client.NewStreamableHttpClient(fmt.Sprintf("http://127.0.0.1:%s/mcp", s.StaticConfig.Port), transport.WithContinuousListening())
	s.Require().NoError(httpClientErr, "Expected no error creating Streamable HTTP MCP client")
	startErr := httpClient.Start(s.T().Context())
	s.Require().NoError(startErr, "Expected no error starting Streamable HTTP MCP client")
	s.Run("Can Initialize Session", func() {
		_, err := httpClient.Initialize(s.T().Context(), test.McpInitRequest())
		s.Require().NoError(err, "Expected no error initializing Streamable HTTP MCP client")
	})
	s.Run("Can List Tools", func() {
		tools, err := httpClient.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Require().NoError(err, "Expected no error listing tools from Streamable HTTP MCP client")
		s.Greater(len(tools.Tools), 0, "Expected at least one tool from Streamable HTTP MCP client")
	})
	s.Run("Can close Streamable HTTP client", func() {
		s.Require().NoError(httpClient.Close(), "Expected no error closing Streamable HTTP MCP client")
	})
}

func (s *McpTransportSuite) TestStatelessConfiguration() {
	s.Run("stateful mode by default", func() {
		// Default configuration should be stateful (false)
		s.False(s.StaticConfig.Stateless, "Expected default configuration to be stateful")

		// Test that the HTTP handler is created (we can't directly test the Stateless field
		// of StreamableHTTPOptions as it's not exposed, but we can verify the server works)
		httpClient, err := client.NewStreamableHttpClient(fmt.Sprintf("http://127.0.0.1:%s/mcp", s.StaticConfig.Port), transport.WithContinuousListening())
		s.Require().NoError(err, "Expected no error creating Streamable HTTP MCP client")
		defer func() { _ = httpClient.Close() }()

		startErr := httpClient.Start(s.T().Context())
		s.Require().NoError(startErr, "Expected no error starting Streamable HTTP MCP client")

		_, initErr := httpClient.Initialize(s.T().Context(), test.McpInitRequest())
		s.Require().NoError(initErr, "Expected no error initializing MCP client in stateful mode")
	})
}

type StatelessMcpTransportSuite struct {
	BaseHttpSuite
}

func (s *StatelessMcpTransportSuite) SetupTest() {
	s.BaseHttpSuite.SetupTest()
	// Configure for stateless mode
	s.StaticConfig.Stateless = true
	s.StartServer()
}

func (s *StatelessMcpTransportSuite) TearDownTest() {
	s.BaseHttpSuite.TearDownTest()
}

func (s *StatelessMcpTransportSuite) TestStatelessMode() {
	s.Run("stateless mode configuration", func() {
		// Verify configuration is set to stateless
		s.True(s.StaticConfig.Stateless, "Expected configuration to be stateless")

		// Test that the HTTP handler works in stateless mode
		httpClient, err := client.NewStreamableHttpClient(fmt.Sprintf("http://127.0.0.1:%s/mcp", s.StaticConfig.Port), transport.WithContinuousListening())
		s.Require().NoError(err, "Expected no error creating Streamable HTTP MCP client")
		defer func() { _ = httpClient.Close() }()

		startErr := httpClient.Start(s.T().Context())
		s.Require().NoError(startErr, "Expected no error starting Streamable HTTP MCP client")

		_, initErr := httpClient.Initialize(s.T().Context(), test.McpInitRequest())
		s.Require().NoError(initErr, "Expected no error initializing MCP client in stateless mode")

		// Basic functionality should still work
		tools, err := httpClient.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Require().NoError(err, "Expected no error listing tools in stateless mode")
		s.Greater(len(tools.Tools), 0, "Expected at least one tool in stateless mode")
	})
}

func TestMcpTransport(t *testing.T) {
	suite.Run(t, new(McpTransportSuite))
}

func TestStatelessMcpTransport(t *testing.T) {
	suite.Run(t, new(StatelessMcpTransportSuite))
}
