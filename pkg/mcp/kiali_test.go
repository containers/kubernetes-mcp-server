package mcp

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type KialiSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
}

func (s *KialiSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Config().BearerToken = "token-xyz"
	kubeConfig := s.Cfg.KubeConfig
	s.Cfg = test.Must(config.ReadToml([]byte(fmt.Sprintf(`
		toolsets = ["kiali"]
		[toolset_configs.kiali]
		url = "%s"
	`, s.mockServer.Config().Host))))
	s.Cfg.KubeConfig = kubeConfig
}

func (s *KialiSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *KialiSuite) TestGetTraces() {
	var capturedURL *url.URL
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte(`{"traceId":"test-trace-123","spans":[]}`))
	}))
	s.InitMcpClient()

	s.Run("get_traces(traceId = 'test-trace-123')", func() {
		traceId := "test-trace-123"
		toolResult, err := s.CallTool("kiali_get_traces", map[string]interface{}{
			"traceId": traceId,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("path is correct", func() {
			s.Equal("/api/traces/test-trace-123", capturedURL.Path, "Unexpected path")
		})
		s.Run("response contains trace ID", func() {
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, traceId, "Response should contain trace ID")
		})
	})
}

func (s *KialiSuite) TestMeshGraph() {
	var capturedUrls []url.URL
	mu := sync.Mutex{}
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		capturedUrls = append(capturedUrls, *r.URL)
		_, _ = w.Write([]byte("{}"))
	}))
	s.InitMcpClient()

	s.Run("mesh_graph() with defaults", func() {
		toolResult, err := s.CallTool("kiali_mesh_graph", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("performs 4 simultaneous requests", func() {
			s.Equal(4, len(capturedUrls), "expected 4 requests to Kiali")
		})
		s.Run("retrieves graph data", func() {
			i := slices.IndexFunc(capturedUrls, func(capturedUrl url.URL) bool {
				return capturedUrl.Path == "/api/namespaces/graph"
			})
			s.Require().NotEqual(-1, i, "expected request to /api/namespaces/graph")
			s.Run("requested with correct query parameters", func() {
				s.Equal("", capturedUrls[i].Query().Get("namespaces"), "Unexpected namespaces query parameter")
				s.Equal("false", capturedUrls[i].Query().Get("includeIdleEdges"), "Unexpected includeIdleEdges query parameter")
				s.Equal("true", capturedUrls[i].Query().Get("injectServiceNodes"), "Unexpected injectServiceNodes query parameter")
				s.Equal("cluster,namespace,app", capturedUrls[i].Query().Get("boxBy"), "Unexpected boxBy query parameter")
				s.Equal("none", capturedUrls[i].Query().Get("ambientTraffic"), "Unexpected ambientTraffic query parameter")
				s.Equal("deadNode,istio,serviceEntry,meshCheck,workloadEntry,health", capturedUrls[i].Query().Get("appenders"), "Unexpected appenders query parameter")
				s.Equal("requests", capturedUrls[i].Query().Get("rateGrpc"), "Unexpected rateGrpc query parameter")
				s.Equal("requests", capturedUrls[i].Query().Get("rateHttp"), "Unexpected rateHttp query parameter")
				s.Equal("sent", capturedUrls[i].Query().Get("rateTcp"), "Unexpected rateTcp query parameter")
			})
		})
		s.Run("retrieves mesh status", func() {
			i := slices.IndexFunc(capturedUrls, func(capturedUrl url.URL) bool {
				return capturedUrl.Path == "/api/mesh/graph"
			})
			s.Require().NotEqual(-1, i, "expected request to /api/mesh/graph")
			s.Run("requested with correct query parameters", func() {
				s.Equal("false", capturedUrls[i].Query().Get("includeGateways"), "Unexpected includeGateways query parameter")
				s.Equal("false", capturedUrls[i].Query().Get("includeWaypoints"), "Unexpected includeWaypoints query parameter")
			})
		})
		s.Run("retrieves namespaces", func() {
			i := slices.IndexFunc(capturedUrls, func(capturedUrl url.URL) bool {
				return capturedUrl.Path == "/api/namespaces"
			})
			s.Require().NotEqual(-1, i, "expected request to /api/namespaces")
		})
		s.Run("retrieves health data", func() {
			i := slices.IndexFunc(capturedUrls, func(capturedUrl url.URL) bool {
				return capturedUrl.Path == "/api/clusters/health"
			})
			s.Require().NotEqual(-1, i, "expected request to /api/clusters/health")
		})
	})

}

func TestKiali(t *testing.T) {
	suite.Run(t, new(KialiSuite))
}
