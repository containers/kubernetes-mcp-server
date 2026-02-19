package http

import (
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

	sseClient := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.33.7"}, nil)
	transport := &mcp.SSEClientTransport{
		Endpoint: fmt.Sprintf("http://127.0.0.1:%s/sse", s.StaticConfig.Port),
	}
	session, err := sseClient.Connect(s.T().Context(), transport, nil)
	s.Require().NoError(err, "Expected no error connecting SSE MCP client")
	defer func() { _ = session.Close() }()

	s.Run("Session is initialized", func() {
		s.Require().NotNil(session.InitializeResult(), "Expected initialize result")
	})
	s.Run("Can List Tools", func() {
		tools, err := session.ListTools(s.T().Context(), &mcp.ListToolsParams{})
		s.Require().NoError(err, "Expected no error listing tools from SSE MCP client")
		s.Greater(len(tools.Tools), 0, "Expected at least one tool from SSE MCP client")
	})
	s.Run("Can close SSE client", func() {
		s.Require().NoError(session.Close(), "Expected no error closing SSE MCP client")
	})
}

func (s *McpTransportSuite) TestStreamableHttpTransport() {
	testCases := []bool{true, false}
	for _, stateless := range testCases {
		s.Run(fmt.Sprintf("Streamable HTTP transport with server stateless=%v", stateless), func() {
			s.StaticConfig.Stateless = stateless
			s.StartServer()

			httpClient := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.33.7"}, nil)
			transport := &mcp.StreamableClientTransport{
				Endpoint: fmt.Sprintf("http://127.0.0.1:%s/mcp", s.StaticConfig.Port),
			}
			session, err := httpClient.Connect(s.T().Context(), transport, nil)
			s.Require().NoError(err, "Expected no error connecting Streamable HTTP MCP client")
			defer func() { _ = session.Close() }()

			s.Run("Session is initialized", func() {
				s.Require().NotNil(session.InitializeResult(), "Expected initialize result")
			})
			s.Run("Can List Tools", func() {
				tools, err := session.ListTools(s.T().Context(), &mcp.ListToolsParams{})
				s.Require().NoError(err, "Expected no error listing tools from Streamable HTTP MCP client")
				s.Greater(len(tools.Tools), 0, "Expected at least one tool from Streamable HTTP MCP client")
			})
			s.Run("Can close Streamable HTTP client", func() {
				s.Require().NoError(session.Close(), "Expected no error closing Streamable HTTP MCP client")
			})
			s.StopServer()
			s.Require().NoError(s.WaitForShutdown())
		})
	}
}

func TestMcpTransport(t *testing.T) {
	suite.Run(t, new(McpTransportSuite))
}
