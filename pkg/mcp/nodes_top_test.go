package mcp

import (
	"net/http"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodesTopSuite struct {
	BaseMcpSuite
	mockServer       *test.MockServer
	discoveryHandler *test.DiscoveryClientHandler
}

func (s *NodesTopSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())

	s.discoveryHandler = test.NewDiscoveryClientHandler()
	s.mockServer.Handle(s.discoveryHandler)
}

func (s *NodesTopSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *NodesTopSuite) WithMetricsServer() {
	s.discoveryHandler.AddAPIResourceList(metav1.APIResourceList{
		GroupVersion: "metrics.k8s.io/v1beta1",
		APIResources: []metav1.APIResource{
			{Name: "nodes", Kind: "NodeMetrics", Namespaced: false, Verbs: metav1.Verbs{"get", "list"}},
		},
	})
}

func (s *NodesTopSuite) TestNodesTop() {
	s.WithMetricsServer()
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// List Nodes
		if req.URL.Path == "/api/v1/nodes" {
			_, _ = w.Write([]byte(`{
				"apiVersion": "v1",
				"kind": "NodeList",
				"items": [
					{
						"metadata": {
							"name": "node-1",
							"labels": {
								"node-role.kubernetes.io/worker": ""
							}
						},
						"status": {
							"allocatable": {
								"cpu": "4",
								"memory": "16Gi"
							},
							"nodeInfo": {
								"swap": {
									"capacity": 0
								}
							}
						}
					},
					{
						"metadata": {
							"name": "node-2",
							"labels": {
								"node-role.kubernetes.io/worker": ""
							}
						},
						"status": {
							"allocatable": {
								"cpu": "4",
								"memory": "16Gi"
							},
							"nodeInfo": {
								"swap": {
									"capacity": 0
								}
							}
						}
					}
				]
			}`))
			return
		}
		// Get NodeMetrics
		if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/nodes" {
			_, _ = w.Write([]byte(`{
				"apiVersion": "metrics.k8s.io/v1beta1",
				"kind": "NodeMetricsList",
				"items": [
					{
						"metadata": {
							"name": "node-1"
						},
						"timestamp": "2025-10-29T09:00:00Z",
						"window": "30s",
						"usage": {
							"cpu": "500m",
							"memory": "2Gi"
						}
					},
					{
						"metadata": {
							"name": "node-2"
						},
						"timestamp": "2025-10-29T09:00:00Z",
						"window": "30s",
						"usage": {
							"cpu": "1000m",
							"memory": "4Gi"
						}
					}
				]
			}`))
			return
		}
		// Get specific NodeMetrics
		if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/nodes/node-1" {
			_, _ = w.Write([]byte(`{
				"apiVersion": "metrics.k8s.io/v1beta1",
				"kind": "NodeMetrics",
				"metadata": {
					"name": "node-1"
				},
				"timestamp": "2025-10-29T09:00:00Z",
				"window": "30s",
				"usage": {
					"cpu": "500m",
					"memory": "2Gi"
				}
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	s.InitMcpClient()

	s.Run("nodes_top() - all nodes", func() {
		toolResult, err := s.CallTool("nodes_top", map[string]interface{}{})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("no error", func() {
			s.Falsef(toolResult.IsError, "call tool should succeed")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("returns metrics for all nodes", func() {
			content := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(content, "node-1", "expected metrics to contain node-1")
			s.Contains(content, "node-2", "expected metrics to contain node-2")
			s.Contains(content, "CPU(cores)", "expected header with CPU column")
			s.Contains(content, "MEMORY(bytes)", "expected header with MEMORY column")
		})
	})

	s.Run("nodes_top(name=node-1) - specific node", func() {
		toolResult, err := s.CallTool("nodes_top", map[string]interface{}{
			"name": "node-1",
		})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("no error", func() {
			s.Falsef(toolResult.IsError, "call tool should succeed")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("returns metrics for specific node", func() {
			content := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(content, "node-1", "expected metrics to contain node-1")
			s.Contains(content, "500m", "expected CPU usage of 500m")
			s.Contains(content, "2048Mi", "expected memory usage of 2048Mi")
		})
	})

	s.Run("nodes_top(label_selector=node-role.kubernetes.io/worker=)", func() {
		toolResult, err := s.CallTool("nodes_top", map[string]interface{}{
			"label_selector": "node-role.kubernetes.io/worker=",
		})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("no error", func() {
			s.Falsef(toolResult.IsError, "call tool should succeed")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("returns metrics for filtered nodes", func() {
			content := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(content, "node-1", "expected metrics to contain node-1")
			s.Contains(content, "node-2", "expected metrics to contain node-2")
		})
	})
}

func (s *NodesTopSuite) TestNodesTopMetricsUnavailable() {
	s.InitMcpClient()

	s.Run("nodes_top() - metrics unavailable", func() {
		toolResult, err := s.CallTool("nodes_top", map[string]interface{}{})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail when metrics unavailable")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes metrics unavailable", func() {
			content := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(content, "failed to get nodes top", "expected error message about failing to get nodes top")
		})
	})
}

func (s *NodesTopSuite) TestNodesTopDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { group = "metrics.k8s.io", version = "v1beta1" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.WithMetricsServer()
	s.InitMcpClient()
	s.Run("nodes_top (denied)", func() {
		toolResult, err := s.CallTool("nodes_top", map[string]interface{}{})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get nodes top:(.+:)? resource not allowed: metrics.k8s.io/v1beta1, Kind=NodeMetrics"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func TestNodesTop(t *testing.T) {
	suite.Run(t, new(NodesTopSuite))
}
