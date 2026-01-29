package mcp

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CodeTopSuite tests the code evaluation tool's ability to retrieve metrics from the metrics server.
// This is a separate suite because it requires mocking the metrics API which is not available in envtest.
type CodeTopSuite struct {
	BaseMcpSuite
	mockServer       *test.MockServer
	discoveryHandler *test.DiscoveryClientHandler
}

func (s *CodeTopSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	// Enable the code toolset for these tests
	s.Cfg.Toolsets = []string{"code"}
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())

	s.discoveryHandler = test.NewDiscoveryClientHandler()
	s.mockServer.Handle(s.discoveryHandler)
}

func (s *CodeTopSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *CodeTopSuite) TestCodeEvaluateMetricsUnavailable() {
	s.InitMcpClient()

	s.Run("evaluate_script with metrics API not available", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const metrics = k8s.metricsV1beta1Client();
				const podMetrics = metrics.podMetricses("").list(ctx, {});
				JSON.stringify(podMetrics.items);
			`,
		})
		s.NoError(err, "call tool failed %v", err)
		s.Require().NotNil(toolResult)
		s.True(toolResult.IsError, "call tool should have returned an error")
		// Error can be either "no matches for metrics" or "server could not find"
		errorText := toolResult.Content[0].(mcp.TextContent).Text
		s.Contains(errorText, "no matches for metrics.k8s.io/v1beta1, Resource=pods", "error should indicate metrics API is not available")
	})
}

func (s *CodeTopSuite) TestCodeEvaluateMetricsAvailable() {
	// Register the metrics API
	s.discoveryHandler.AddAPIResourceList(metav1.APIResourceList{
		GroupVersion: "metrics.k8s.io/v1beta1",
		APIResources: []metav1.APIResource{
			{Name: "pods", Kind: "PodMetrics", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
			{Name: "nodes", Kind: "NodeMetrics", Namespaced: false, Verbs: metav1.Verbs{"get", "list"}},
		},
	})

	// Mock the metrics endpoints
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Pod Metrics from all namespaces
		if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/pods" {
			_, _ = w.Write([]byte(`{
				"kind": "PodMetricsList",
				"apiVersion": "metrics.k8s.io/v1beta1",
				"items": [
					{
						"metadata": {"name": "nginx-pod", "namespace": "default"},
						"timestamp": "2024-01-15T10:00:00Z",
						"window": "30s",
						"containers": [
							{"name": "nginx", "usage": {"cpu": "100m", "memory": "128Mi"}}
						]
					},
					{
						"metadata": {"name": "redis-pod", "namespace": "cache"},
						"timestamp": "2024-01-15T10:00:00Z",
						"window": "30s",
						"containers": [
							{"name": "redis", "usage": {"cpu": "50m", "memory": "64Mi"}}
						]
					}
				]
			}`))
			return
		}

		// Pod Metrics from specific namespace
		if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/namespaces/default/pods" {
			_, _ = w.Write([]byte(`{
				"kind": "PodMetricsList",
				"apiVersion": "metrics.k8s.io/v1beta1",
				"items": [
					{
						"metadata": {"name": "nginx-pod", "namespace": "default"},
						"timestamp": "2024-01-15T10:00:00Z",
						"window": "30s",
						"containers": [
							{"name": "nginx", "usage": {"cpu": "100m", "memory": "128Mi"}}
						]
					}
				]
			}`))
			return
		}

		// Node Metrics
		if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/nodes" {
			_, _ = w.Write([]byte(`{
				"kind": "NodeMetricsList",
				"apiVersion": "metrics.k8s.io/v1beta1",
				"items": [
					{
						"metadata": {"name": "node-1"},
						"timestamp": "2024-01-15T10:00:00Z",
						"window": "30s",
						"usage": {"cpu": "500m", "memory": "2Gi"}
					},
					{
						"metadata": {"name": "node-2"},
						"timestamp": "2024-01-15T10:00:00Z",
						"window": "30s",
						"usage": {"cpu": "750m", "memory": "3Gi"}
					}
				]
			}`))
			return
		}
	}))

	s.InitMcpClient()

	s.Run("lists pod metrics", func() {
		s.Run("evaluate_script from all namespaces", func() {
			toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
				"script": `
					var metrics = k8s.metricsV1beta1Client();
					var podMetrics = metrics.podMetricses("").list(ctx, {});
					var result = podMetrics.items.map(function(pm) {
						return {
							name: pm.metadata.name,
							namespace: pm.metadata.namespace,
							containerCount: pm.containers.length,
							firstContainer: pm.containers.length > 0 ? pm.containers[0].name : null
						};
					});
					JSON.stringify(result);
				`,
			})
			s.Run("no error", func() {
				s.NoError(err)
				s.Require().NotNil(toolResult)
				s.False(toolResult.IsError, "call tool failed: %v", toolResult.Content)
			})
			s.Run("returns structured output", func() {
				var result []map[string]interface{}
				text := toolResult.Content[0].(mcp.TextContent).Text
				s.Require().NoError(json.Unmarshal([]byte(text), &result))

				s.Run("with expected pod count", func() {
					s.Len(result, 2)
				})
				s.Run("with nginx-pod metadata", func() {
					var nginxPod map[string]interface{}
					for _, pod := range result {
						if pod["name"] == "nginx-pod" {
							nginxPod = pod
							break
						}
					}
					s.Require().NotNil(nginxPod, "should include nginx-pod")
					s.Equal("default", nginxPod["namespace"])
					s.Equal(float64(1), nginxPod["containerCount"])
					s.Equal("nginx", nginxPod["firstContainer"])
				})
			})
		})

		s.Run("evaluate_script from specific namespace", func() {
			toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
				"script": `
					const metrics = k8s.metricsV1beta1Client();
					const podMetrics = metrics.podMetricses("default").list(ctx, {});
					JSON.stringify(podMetrics.items.map(pm => pm.metadata.name));
				`,
			})
			s.Run("no error", func() {
				s.NoError(err)
				s.Require().NotNil(toolResult)
				s.False(toolResult.IsError, "call tool failed: %v", toolResult.Content)
			})
			s.Run("returns structured output", func() {
				var names []string
				text := toolResult.Content[0].(mcp.TextContent).Text
				s.Require().NoError(json.Unmarshal([]byte(text), &names))

				s.Run("with only default namespace pods", func() {
					s.Len(names, 1)
					s.Equal("nginx-pod", names[0])
				})
			})
		})
	})

	s.Run("evaluate_script lists node metrics", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				var metrics = k8s.metricsV1beta1Client();
				var nodeMetrics = metrics.nodeMetricses().list(ctx, {});
				var result = nodeMetrics.items.map(function(nm) {
					return {
						name: nm.metadata.name,
						hasUsage: nm.usage !== undefined && nm.usage !== null
					};
				});
				JSON.stringify(result);
			`,
		})
		s.Run("no error", func() {
			s.NoError(err)
			s.Require().NotNil(toolResult)
			s.False(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns structured output", func() {
			var result []map[string]interface{}
			text := toolResult.Content[0].(mcp.TextContent).Text
			s.Require().NoError(json.Unmarshal([]byte(text), &result))

			s.Run("with expected node count", func() {
				s.Len(result, 2)
			})
			s.Run("with node-1 usage data", func() {
				var node1 map[string]interface{}
				for _, node := range result {
					if node["name"] == "node-1" {
						node1 = node
						break
					}
				}
				s.Require().NotNil(node1, "should include node-1")
				s.True(node1["hasUsage"].(bool))
			})
		})
	})

	s.Run("evaluate_script aggregates metrics with JavaScript", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				var metrics = k8s.metricsV1beta1Client();
				var podMetrics = metrics.podMetricses("").list(ctx, {});

				var totalContainers = 0;
				podMetrics.items.forEach(function(pm) {
					totalContainers += pm.containers.length;
				});

				JSON.stringify({
					podCount: podMetrics.items.length,
					totalContainers: totalContainers
				});
			`,
		})
		s.Run("no error", func() {
			s.NoError(err)
			s.Require().NotNil(toolResult)
			s.False(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns structured output", func() {
			var result map[string]interface{}
			text := toolResult.Content[0].(mcp.TextContent).Text
			s.Require().NoError(json.Unmarshal([]byte(text), &result))

			s.Run("with correct pod count", func() {
				s.Equal(float64(2), result["podCount"])
			})
			s.Run("with correct container count", func() {
				s.Equal(float64(2), result["totalContainers"])
			})
		})
	})

	s.Run("evaluate_script accesses Quantity values transparently", func() {
		// Kubernetes Quantities serialize to strings; the k8s proxy converts API responses
		// to pure JavaScript objects, so Quantity values are accessible as strings directly.
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				var metrics = k8s.metricsV1beta1Client();
				var podMetrics = metrics.podMetricses("").list(ctx, {});
				var container = podMetrics.items[0].containers[0];

				JSON.stringify({
					cpu: container.usage.cpu,
					memory: container.usage.memory
				});
			`,
		})
		s.Run("no error", func() {
			s.NoError(err)
			s.Require().NotNil(toolResult)
			s.False(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns structured output", func() {
			var result map[string]interface{}
			text := toolResult.Content[0].(mcp.TextContent).Text
			s.Require().NoError(json.Unmarshal([]byte(text), &result))

			s.Run("with CPU value as string", func() {
				s.Equal("100m", result["cpu"])
			})
			s.Run("with memory value as string", func() {
				s.Equal("128Mi", result["memory"])
			})
		})
	})
}

func TestCodeTop(t *testing.T) {
	suite.Run(t, new(CodeTopSuite))
}
