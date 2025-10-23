package mcp

import (
	"net/http"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type NodesSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
}

func (s *NodesSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
}

func (s *NodesSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *NodesSuite) TestNodesLog() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Get Node response
		if req.URL.Path == "/api/v1/nodes/existing-node" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"apiVersion": "v1",
				"kind": "Node",
				"metadata": {
					"name": "existing-node"
				}
			}`))
			return
		}
		// Get Empty Log response
		if req.URL.Path == "/api/v1/nodes/existing-node/proxy/logs/empty.log" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(``))
			return
		}
		// Get Kubelet Log response
		if req.URL.Path == "/api/v1/nodes/existing-node/proxy/logs/kubelet.log" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			logContent := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
			if req.URL.Query().Get("tailLines") != "" {
				logContent = "Line 4\nLine 5\n"
			}
			_, _ = w.Write([]byte(logContent))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	s.InitMcpClient()
	s.Run("nodes_log(name=nil)", func() {
		toolResult, err := s.CallTool("nodes_log", map[string]interface{}{})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes missing name", func() {
			expectedMessage := "failed to get node log, missing argument name"
			s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("nodes_log(name=inexistent-node)", func() {
		toolResult, err := s.CallTool("nodes_log", map[string]interface{}{
			"name": "inexistent-node",
		})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes missing node", func() {
			expectedMessage := "failed to get node log for inexistent-node: failed to get node inexistent-node: the server could not find the requested resource (get nodes inexistent-node)"
			s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("nodes_log(name=existing-node, log_path=missing.log)", func() {
		toolResult, err := s.CallTool("nodes_log", map[string]interface{}{
			"name":     "existing-node",
			"log_path": "missing.log",
		})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes missing log file", func() {
			expectedMessage := "failed to get node log for existing-node: failed to get node logs: the server could not find the requested resource"
			s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("nodes_log(name=existing-node, log_path=empty.log)", func() {
		toolResult, err := s.CallTool("nodes_log", map[string]interface{}{
			"name":     "existing-node",
			"log_path": "empty.log",
		})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("no error", func() {
			s.Falsef(toolResult.IsError, "call tool should succeed")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes empty log", func() {
			expectedMessage := "The node existing-node has not logged any message yet or the log file is empty"
			s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
				"expected descriptive message '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("nodes_log(name=existing-node, log_path=kubelet.log)", func() {
		toolResult, err := s.CallTool("nodes_log", map[string]interface{}{
			"name":     "existing-node",
			"log_path": "kubelet.log",
		})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("no error", func() {
			s.Falsef(toolResult.IsError, "call tool should succeed")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("returns full log", func() {
			expectedMessage := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
			s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
				"expected log content '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	for _, tailCase := range []interface{}{2, int64(2), float64(2)} {
		s.Run("nodes_log(name=existing-node, log_path=kubelet.log, tail=2)", func() {
			toolResult, err := s.CallTool("nodes_log", map[string]interface{}{
				"name":     "existing-node",
				"log_path": "kubelet.log",
				"tail":     tailCase,
			})
			s.Require().NotNil(toolResult, "toolResult should not be nil")
			s.Run("no error", func() {
				s.Falsef(toolResult.IsError, "call tool should succeed")
				s.Nilf(err, "call tool should not return error object")
			})
			s.Run("returns tail log", func() {
				expectedMessage := "Line 4\nLine 5\n"
				s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
					"expected log content '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
			})
		})
		s.Run("nodes_log(name=existing-node, log_path=kubelet.log, tail=-1)", func() {
			toolResult, err := s.CallTool("nodes_log", map[string]interface{}{
				"name":     "existing-node",
				"log_path": "kubelet.log",
				"tail":     -1,
			})
			s.Require().NotNil(toolResult, "toolResult should not be nil")
			s.Run("no error", func() {
				s.Falsef(toolResult.IsError, "call tool should succeed")
				s.Nilf(err, "call tool should not return error object")
			})
			s.Run("returns full log", func() {
				expectedMessage := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
				s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
					"expected log content '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
			})
		})
	}
}

func (s *NodesSuite) TestNodesLogDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Node" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("nodes_log (denied)", func() {
		toolResult, err := s.CallTool("nodes_log", map[string]interface{}{
			"name": "does-not-matter",
		})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			expectedMessage := "failed to get node log for does-not-matter: resource not allowed: /v1, Kind=Node"
			s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
}

func TestNodes(t *testing.T) {
	suite.Run(t, new(NodesSuite))
}
