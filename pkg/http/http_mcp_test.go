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
	s.StaticConfig.Stateless = false
}

func (s *McpTransportSuite) TearDownTest() {
	s.BaseHttpSuite.TearDownTest()
}

func (s *McpTransportSuite) TestSseTransport() {
	s.StartServer()
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
	testCases := []bool{true, false}
	for _, stateless := range testCases {
		s.Run(fmt.Sprintf("Streamable HTTP transport with server stateless=%v", stateless), func() {
			s.StaticConfig.Stateless = stateless
			s.StartServer()
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
			s.StopServer()
			s.Require().NoError(s.WaitForShutdown())
		})
	}
}

func TestMcpTransport(t *testing.T) {
	suite.Run(t, new(McpTransportSuite))
}
