package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type McpTransportSuite struct {
	BaseHttpSuite
}

func (s *McpTransportSuite) SetupTest() {
	s.BaseHttpSuite.SetupTest()
	s.Config.Stateless.SetForTest(false)
}

func (s *McpTransportSuite) TearDownTest() {
	s.BaseHttpSuite.TearDownTest()
}

func (s *McpTransportSuite) TestSseEndpointsRemoved() {
	s.StartServer()

	for _, path := range []string{"/sse", "/message"} {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s%s", s.Config.Port.Get(), path))
		s.Require().NoError(err, "Expected GET %s to complete", path)
		_ = resp.Body.Close()
		s.Equal(http.StatusNotFound, resp.StatusCode, "SSE endpoint %s must not be served", path)
	}
}

func (s *McpTransportSuite) TestStreamableHttpTransport() {
	testCases := []bool{true, false}
	for _, stateless := range testCases {
		s.Run(fmt.Sprintf("Streamable HTTP transport with server stateless=%v", stateless), func() {
			s.Config.Stateless.SetForTest(stateless)
			s.StartServer()

			httpClient := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.33.7"}, nil)
			transport := &mcp.StreamableClientTransport{
				Endpoint: fmt.Sprintf("http://127.0.0.1:%s/mcp", s.Config.Port.Get()),
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
			s.stopRunningServer()
		})
	}
}

func TestMcpTransport(t *testing.T) {
	suite.Run(t, new(McpTransportSuite))
}

const mcpInitializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

func (s *McpTransportSuite) postInitializeWithHost(host string) *http.Response {
	s.T().Helper()
	url := fmt.Sprintf("http://127.0.0.1:%s/mcp", s.Config.Port.Get())
	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodPost, url, strings.NewReader(mcpInitializeBody))
	s.Require().NoError(err)
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	return resp
}

func (s *McpTransportSuite) TestLocalhostProtectionHostHeader() {
	s.Run("rejects non-localhost Host by default", func() {
		s.Config.DisableLocalhostProtection.SetForTest(false)
		s.StartServer()
		resp := s.postInitializeWithHost("kubernetes-mcp-server:8443")
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		s.Require().NoError(err)
		s.Equal(http.StatusForbidden, resp.StatusCode)
		s.Contains(string(body), "invalid Host header")
	})
	s.Run("allows non-localhost Host when disabled", func() {
		s.Config.DisableLocalhostProtection.SetForTest(true)
		s.StartServer()
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.33.7"}, nil)
		transport := &mcp.StreamableClientTransport{
			Endpoint: fmt.Sprintf("http://127.0.0.1:%s/mcp", s.Config.Port.Get()),
			HTTPClient: &http.Client{
				Timeout:   http.DefaultClient.Timeout,
				Transport: hostHeaderRoundTripper{host: "kubernetes-mcp-server:8443"},
			},
		}
		session, err := client.Connect(s.T().Context(), transport, nil)
		s.Require().NoError(err, "initialize handshake should succeed with a non-localhost Host")
		defer func() { _ = session.Close() }()
		s.Require().NotNil(session.InitializeResult())
	})
}

// hostHeaderRoundTripper sends requests to the URL host but sets Host to a
// different value (the kube-rbac-proxy / Service-name case).
type hostHeaderRoundTripper struct {
	host string
	base http.RoundTripper
}

func (t hostHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Host = t.host
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}
