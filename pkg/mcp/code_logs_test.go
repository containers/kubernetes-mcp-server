package mcp

import (
	"net/http"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type CodeLogsSuite struct {
	BaseMcpSuite
	mockServer       *test.MockServer
	discoveryHandler *test.DiscoveryClientHandler
}

func (s *CodeLogsSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.Cfg.Toolsets = []string{"code"}
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())

	s.discoveryHandler = test.NewDiscoveryClientHandler()
	s.mockServer.Handle(s.discoveryHandler)

	// Mock the pod logs endpoint
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/v1/namespaces/default/pods/test-pod/log" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("2024-01-15T10:00:00Z Hello from test-pod\n2024-01-15T10:00:01Z Log line 2\n"))
			return
		}
	}))
}

func (s *CodeLogsSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *CodeLogsSuite) TestCodeEvaluatePodLogs() {
	s.InitMcpClient()

	s.Run("evaluate_script retrieves pod logs", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const pods = k8s.coreV1().pods("default");
				const logBytes = pods.getLogs("test-pod", {}).doRaw(ctx);
				// Convert byte array to string
				var logs = "";
				for (var i = 0; i < logBytes.length; i++) {
					logs += String.fromCharCode(logBytes[i]);
				}
				logs;
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns log content", func() {
			text := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(text, "Hello from test-pod")
			s.Contains(text, "Log line 2")
		})
	})
}

func TestCodeLogs(t *testing.T) {
	suite.Run(t, new(CodeLogsSuite))
}
