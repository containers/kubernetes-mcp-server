package mcp

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

// podsLogLimitLine is 100 bytes including the newline, so tail N returns N*100 bytes.
var podsLogLimitLine = strings.Repeat("x", 99) + "\n"

type PodsLogLimitSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
}

func (s *PodsLogLimitSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v1/namespaces/ns-1/pods/chatty":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"apiVersion": "v1",
				"kind": "Pod",
				"metadata": {"name": "chatty", "namespace": "ns-1"},
				"spec": {"containers": [{"name": "app", "image": "app"}]}
			}`))
		case "/api/v1/namespaces/ns-1/pods/chatty/log":
			tail, err := strconv.Atoi(req.URL.Query().Get("tailLines"))
			if err != nil {
				tail = 100
			}
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(strings.Repeat(podsLogLimitLine, tail)))
		}
	}))
	s.Cfg.KubeConfig.SetForTest(s.mockServer.KubeconfigFile(s.T()))
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		max_result_bytes = 4096
	`)
}

func (s *PodsLogLimitSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *PodsLogLimitSuite) TestPodsLogOverResultLimit() {
	s.InitMcpClient()
	s.Run("pods_log(tail=100) over max_result_bytes", func() {
		toolResult, err := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "chatty",
			"tail":      100,
		})
		s.Require().NoError(err, "the oversized log must come back as a tool error, not a protocol error")
		s.Require().NotNil(toolResult)
		s.Run("returns a tool error", func() {
			s.True(toolResult.IsError)
		})
		s.Run("states the sizes", func() {
			s.Contains(toolResult.Content[0].(*mcp.TextContent).Text,
				"has 10000 bytes in 100 lines, more than max_result_bytes (4096)")
		})
		s.Run("names a tail that fits", func() {
			// 100 lines * 4096 / 10000 bytes = 40 lines, minus 10% headroom.
			s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "retry with tail 36 or less")
		})
	})
	s.Run("pods_log(tail=36) within max_result_bytes", func() {
		toolResult, err := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "chatty",
			"tail":      36,
		})
		s.Require().NoError(err)
		s.Run("returns the log", func() {
			s.Equal(strings.Repeat(podsLogLimitLine, 36), toolResult.Content[0].(*mcp.TextContent).Text)
		})
	})
}

func TestPodsLogLimit(t *testing.T) {
	suite.Run(t, new(PodsLogLimitSuite))
}
