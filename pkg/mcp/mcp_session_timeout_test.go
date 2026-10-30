package mcp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type McpSessionTimeoutSuite struct {
	BaseMcpSuite
	server *httptest.Server
}

func (s *McpSessionTimeoutSuite) TearDownTest() {
	if s.server != nil {
		s.server.Close()
	}
	s.BaseMcpSuite.TearDownTest()
}

func (s *McpSessionTimeoutSuite) start(timeout time.Duration) {
	s.Cfg.HTTP.SessionTimeout.SetForTest(timeout)
	s.InitMcpClient()
	s.server = httptest.NewServer(s.mcpServer.ServeHTTP())
}

func (s *McpSessionTimeoutSuite) request(ctx context.Context, method, sessionID, body string) *http.Response {
	req, err := http.NewRequestWithContext(ctx, method, s.server.URL, bytes.NewBufferString(body))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	return resp
}

func (s *McpSessionTimeoutSuite) post(sessionID, body string) (int, string) {
	resp := s.request(s.T().Context(), http.MethodPost, sessionID, body)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header.Get("Mcp-Session-Id")
}

// openSession initializes a session the way a client that never sends DELETE does.
func (s *McpSessionTimeoutSuite) openSession() string {
	status, sessionID := s.post("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"no-delete","version":"1"}}}`)
	s.Require().Equal(http.StatusOK, status)
	s.Require().NotEmpty(sessionID)
	status, _ = s.post(sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	s.Require().Equal(http.StatusAccepted, status)
	return sessionID
}

func (s *McpSessionTimeoutSuite) ping(sessionID string) int {
	status, _ := s.post(sessionID, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	return status
}

func (s *McpSessionTimeoutSuite) TestDefaultKeepsIdleSessions() {
	s.start(0)
	sessionID := s.openSession()
	time.Sleep(500 * time.Millisecond)
	s.Equal(http.StatusOK, s.ping(sessionID), "idle sessions are kept when session_timeout is unset")
}

func (s *McpSessionTimeoutSuite) TestClosesIdleSessions() {
	s.start(200 * time.Millisecond)
	sessionID := s.openSession()
	s.Require().Equal(http.StatusOK, s.ping(sessionID))
	s.Eventually(func() bool {
		return s.ping(sessionID) == http.StatusNotFound
	}, 5*time.Second, 300*time.Millisecond, "idle session should be closed after session_timeout")
}

func (s *McpSessionTimeoutSuite) TestRequestsKeepSessionAlive() {
	s.start(500 * time.Millisecond)
	sessionID := s.openSession()
	for i := 0; i < 6; i++ {
		time.Sleep(200 * time.Millisecond)
		s.Require().Equal(http.StatusOK, s.ping(sessionID), "a session used within session_timeout stays open")
	}
}

func TestMcpSessionTimeout(t *testing.T) {
	suite.Run(t, new(McpSessionTimeoutSuite))
}
