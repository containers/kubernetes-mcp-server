package mcp

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	kialiToolset "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type KialiSuite struct {
	BaseMcpSuite
	mockServer  *test.MockServer
	toolsetName string
}

func (s *KialiSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Config().BearerToken = "token-xyz"
	s.toolsetName = (&kialiToolset.Toolset{}).GetName()
	// toolset_configs requires the two-phase parsing performed by config.ReadToml,
	// so we replace s.Cfg and restore the runtime fields the suite already set.
	kubeConfig := s.Cfg.KubeConfig
	listOutput := s.Cfg.ListOutput
	readOnly := s.Cfg.ReadOnly
	cfg, err := config.ReadToml([]byte(fmt.Sprintf(`
		toolsets = ["%s"]
		[toolset_configs.kiali]
		url = "%s"
	`, s.toolsetName, s.mockServer.Config().Host)))
	s.Require().NoError(err, "failed to parse kiali toolset config")
	s.Cfg = cfg
	s.Cfg.KubeConfig = kubeConfig
	s.Cfg.ListOutput = listOutput
	s.Cfg.ReadOnly = readOnly
}

func (s *KialiSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *KialiSuite) TestGetTraces() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"traceId":"test-trace-123","spans":[]}`))
	}))
	s.InitMcpClient()

	s.Run("get_trace_details(traceId = 'test-trace-123')", func() {
		traceId := "test-trace-123"
		toolResult, err := s.CallTool(fmt.Sprintf("%s_get_trace_details", s.toolsetName), map[string]interface{}{
			"traceId": traceId,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("path is correct", func() {
			s.Equal("/api/chat/mcp/get_trace_details", capturedURL.Path, "Unexpected path")
		})
		s.Run("request body contains traceId", func() {
			s.Contains(capturedBody, traceId, "Request body should contain trace ID")
		})
		s.Run("response contains trace ID", func() {
			s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, traceId, "Response should contain trace ID")
		})
	})
}

func (s *KialiSuite) TestGetMeshTrafficGraph() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"elements":{}}`))
	}))
	s.InitMcpClient()

	s.Run("get_mesh_traffic_graph with namespaces", func() {
		toolResult, err := s.CallTool(fmt.Sprintf("%s_get_mesh_traffic_graph", s.toolsetName), map[string]interface{}{
			"namespaces": "bookinfo",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("sends single POST to MCP endpoint", func() {
			s.Equal("/api/chat/mcp/get_mesh_traffic_graph", capturedURL.Path, "Unexpected path")
		})
		s.Run("request body contains namespaces", func() {
			s.Contains(capturedBody, "bookinfo", "Request body should contain namespaces")
		})
	})
}

func (s *KialiSuite) TestGetMeshStatus() {
	var capturedURL *url.URL
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	s.InitMcpClient()

	s.Run("get_mesh_status", func() {
		toolResult, err := s.CallTool(fmt.Sprintf("%s_get_mesh_status", s.toolsetName), map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("sends POST to MCP endpoint", func() {
			s.Equal("/api/chat/mcp/get_mesh_status", capturedURL.Path, "Unexpected path")
		})
		s.Run("response contains status", func() {
			s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "healthy", "Response should contain status")
		})
	})
}

func (s *KialiSuite) TestListClusters() {
	var capturedURL *url.URL
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte(`[{"name":"cluster-east","isHomeCluster":true},{"name":"cluster-west","isHomeCluster":false}]`))
	}))
	s.InitMcpClient()

	s.Run("list_mesh_clusters", func() {
		toolResult, err := s.CallTool(fmt.Sprintf("%s_list_mesh_clusters", s.toolsetName), map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("sends POST to MCP endpoint", func() {
			s.Equal("/api/chat/mcp/list_clusters", capturedURL.Path, "Unexpected path")
		})
		s.Run("response contains cluster names", func() {
			text := toolResult.Content[0].(*mcp.TextContent).Text
			s.Contains(text, "cluster-east", "Response should contain home cluster name")
			s.Contains(text, "cluster-west", "Response should contain remote cluster name")
		})
	})
}

func (s *KialiSuite) TestKialiPromptsRegistered() {
	s.InitMcpClient()
	prompts, err := s.ListPrompts()

	s.Run("ListPrompts succeeds", func() {
		s.NoError(err)
		s.NotNil(prompts)
	})

	expectedPrompts := []string{
		"mesh-list-applications",
		"mesh-list-namespaces",
		"mesh-list-services",
		"mesh-list-workloads",
		"list-istio-config",
		"mesh-topology",
		"mesh-health-check",
		"traffic-topology",
		"service-troubleshoot",
		"trace-analysis",
		"istio-config-review",
	}

	s.Run("all 11 kiali prompts are registered", func() {
		s.Require().NotNil(prompts)
		names := make(map[string]bool, len(prompts.Prompts))
		for _, p := range prompts.Prompts {
			names[p.Name] = true
		}
		for _, expected := range expectedPrompts {
			s.Truef(names[expected], "expected prompt %q to be registered", expected)
		}
	})
}

func (s *KialiSuite) TestListApplicationsPrompt() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("mesh-list-applications", map[string]string{
		"namespace": "bookinfo",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls list_or_get_resources endpoint", func() {
		s.Require().NotNil(capturedURL)
		s.Equal("/api/chat/mcp/list_or_get_resources", capturedURL.Path)
	})
	s.Run("request body contains resourceType app", func() {
		s.Contains(capturedBody, "app")
	})
	s.Run("request body contains namespace filter", func() {
		s.Contains(capturedBody, "bookinfo")
	})
	s.Run("prompt result contains at least one user message", func() {
		s.Require().NotEmpty(result.Messages)
		s.Equal("user", string(result.Messages[0].Role))
	})
}

func (s *KialiSuite) TestListNamespacesPrompt() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"cluster":"default","namespaces":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("mesh-list-namespaces", map[string]string{})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls list_or_get_resources endpoint", func() {
		s.Require().NotNil(capturedURL)
		s.Equal("/api/chat/mcp/list_or_get_resources", capturedURL.Path)
	})
	s.Run("request body contains resourceType namespace", func() {
		s.Contains(capturedBody, "namespace")
	})
	s.Run("prompt result contains at least one user message", func() {
		s.Require().NotEmpty(result.Messages)
		s.Equal("user", string(result.Messages[0].Role))
	})
}

func (s *KialiSuite) TestListServicesPrompt() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"services":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("mesh-list-services", map[string]string{
		"namespace": "bookinfo",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls list_or_get_resources endpoint", func() {
		s.Require().NotNil(capturedURL)
		s.Equal("/api/chat/mcp/list_or_get_resources", capturedURL.Path)
	})
	s.Run("request body contains resourceType service", func() {
		s.Contains(capturedBody, "service")
	})
	s.Run("request body contains namespace filter", func() {
		s.Contains(capturedBody, "bookinfo")
	})
	s.Run("prompt result contains at least one user message", func() {
		s.Require().NotEmpty(result.Messages)
		s.Equal("user", string(result.Messages[0].Role))
	})
}

func (s *KialiSuite) TestListWorkloadsPrompt() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"workloads":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("mesh-list-workloads", map[string]string{
		"namespace": "kube-system",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls list_or_get_resources endpoint", func() {
		s.Require().NotNil(capturedURL)
		s.Equal("/api/chat/mcp/list_or_get_resources", capturedURL.Path)
	})
	s.Run("request body contains resourceType workload", func() {
		s.Contains(capturedBody, "workload")
	})
	s.Run("request body contains namespace filter", func() {
		s.Contains(capturedBody, "kube-system")
	})
	s.Run("prompt result contains at least one user message", func() {
		s.Require().NotEmpty(result.Messages)
		s.Equal("user", string(result.Messages[0].Role))
	})
}

func (s *KialiSuite) TestListIstioConfigPrompt() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("list-istio-config", map[string]string{
		"namespace": "bookinfo",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls manage_istio_config_read endpoint (not list_or_get_resources)", func() {
		s.Require().NotNil(capturedURL)
		s.Equal("/api/chat/mcp/manage_istio_config_read", capturedURL.Path)
	})
	s.Run("request body contains action list", func() {
		s.Contains(capturedBody, "list")
	})
	s.Run("request body contains namespace filter", func() {
		s.Contains(capturedBody, "bookinfo")
	})
}

func (s *KialiSuite) TestMeshHealthCheckPrompt() {
	var capturedURL *url.URL
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte(`{"status":"healthy","components":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("mesh-health-check", map[string]string{})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls get_mesh_status endpoint", func() {
		s.Require().NotNil(capturedURL)
		s.Equal("/api/chat/mcp/get_mesh_status", capturedURL.Path)
	})
	s.Run("result has user and assistant messages", func() {
		s.Require().Len(result.Messages, 2)
		s.Equal("user", string(result.Messages[0].Role))
		s.Equal("assistant", string(result.Messages[1].Role))
	})
}

func (s *KialiSuite) TestMeshTopologyPrompt() {
	var capturedPaths []string
	var capturedBodies []string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPaths = append(capturedPaths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		capturedBodies = append(capturedBodies, string(body))
		if r.URL.Path == "/api/chat/mcp/list_or_get_resources" {
			_, _ = w.Write([]byte(`{"cluster":"default","namespaces":[{"name":"bookinfo","health":"Healthy"},{"name":"istio-system","health":"Healthy"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("mesh-topology", map[string]string{})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls get_mesh_status endpoint", func() {
		s.Contains(capturedPaths, "/api/chat/mcp/get_mesh_status")
	})
	s.Run("resolves namespaces via list_or_get_resources", func() {
		s.Contains(capturedPaths, "/api/chat/mcp/list_or_get_resources")
	})
	s.Run("calls get_mesh_traffic_graph with resolved namespaces", func() {
		s.Contains(capturedPaths, "/api/chat/mcp/get_mesh_traffic_graph")
		for i, path := range capturedPaths {
			if path == "/api/chat/mcp/get_mesh_traffic_graph" {
				s.Contains(capturedBodies[i], "bookinfo")
				s.Contains(capturedBodies[i], "istio-system")
			}
		}
	})
	s.Run("prompt result contains user message", func() {
		s.Require().NotEmpty(result.Messages)
		s.Equal("user", string(result.Messages[0].Role))
	})
}

func (s *KialiSuite) TestTrafficTopologyPrompt() {
	var capturedURL *url.URL
	var capturedBody string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte(`{"elements":{}}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("traffic-topology", map[string]string{
		"namespaces": "bookinfo,default",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls get_mesh_traffic_graph endpoint", func() {
		s.Require().NotNil(capturedURL)
		s.Equal("/api/chat/mcp/get_mesh_traffic_graph", capturedURL.Path)
	})
	s.Run("request body contains the namespaces", func() {
		s.Contains(capturedBody, "bookinfo")
	})
}

func (s *KialiSuite) TestTrafficTopologyPromptAllNamespaces() {
	var mu sync.Mutex
	var capturedPaths []string
	var capturedBodies []string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedPaths = append(capturedPaths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		capturedBodies = append(capturedBodies, string(body))
		mu.Unlock()
		if r.URL.Path == "/api/chat/mcp/list_or_get_resources" {
			_, _ = w.Write([]byte(`{"namespaces":[{"name":"bookinfo"},{"name":"istio-system"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"elements":{}}`))
		}
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("traffic-topology", map[string]string{
		"namespaces": "all",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("resolves namespaces via list_or_get_resources then calls traffic graph", func() {
		mu.Lock()
		defer mu.Unlock()
		s.Require().Len(capturedPaths, 2)
		s.Equal("/api/chat/mcp/list_or_get_resources", capturedPaths[0])
		s.Equal("/api/chat/mcp/get_mesh_traffic_graph", capturedPaths[1])
	})
	s.Run("traffic graph request contains resolved namespaces", func() {
		mu.Lock()
		defer mu.Unlock()
		s.Contains(capturedBodies[1], "bookinfo")
		s.Contains(capturedBodies[1], "istio-system")
	})
}

func (s *KialiSuite) TestServiceTroubleshootPromptWithWorkload() {
	var mu sync.Mutex
	var capturedBodies []string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		body, _ := io.ReadAll(r.Body)
		capturedBodies = append(capturedBodies, string(body))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"logs":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("service-troubleshoot", map[string]string{
		"namespace": "bookinfo",
		"service":   "productpage",
		"workload":  "productpage-v1",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("logs request uses workload name instead of service name", func() {
		mu.Lock()
		defer mu.Unlock()
		s.Require().NotEmpty(capturedBodies)
		s.Contains(capturedBodies[0], "productpage-v1")
	})
}

func (s *KialiSuite) TestServiceTroubleshootPromptRequiredArgs() {
	s.InitMcpClient()

	s.Run("returns error when namespace is missing", func() {
		result, err := s.GetPrompt("service-troubleshoot", map[string]string{
			"service": "productpage",
		})
		s.Error(err)
		s.Nil(result)
	})

	s.Run("returns error when service is missing", func() {
		result, err := s.GetPrompt("service-troubleshoot", map[string]string{
			"namespace": "bookinfo",
		})
		s.Error(err)
		s.Nil(result)
	})
}

func (s *KialiSuite) TestServiceTroubleshootPrompt() {
	var mu sync.Mutex
	var capturedPaths []string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedPaths = append(capturedPaths, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"logs":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("service-troubleshoot", map[string]string{
		"namespace": "bookinfo",
		"service":   "productpage",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls get_logs and manage_istio_config_read endpoints", func() {
		mu.Lock()
		defer mu.Unlock()
		pathSet := make(map[string]bool)
		for _, p := range capturedPaths {
			pathSet[p] = true
		}
		s.True(pathSet["/api/chat/mcp/get_logs"], "expected get_logs to be called")
		s.True(pathSet["/api/chat/mcp/manage_istio_config_read"], "expected manage_istio_config_read to be called")
	})
	s.Run("result contains user and assistant messages", func() {
		s.Require().Len(result.Messages, 2)
		s.Equal("user", string(result.Messages[0].Role))
		s.Equal("assistant", string(result.Messages[1].Role))
	})
}

func (s *KialiSuite) TestTraceAnalysisPromptRequiredArgs() {
	s.InitMcpClient()

	s.Run("returns error when namespace is missing", func() {
		result, err := s.GetPrompt("trace-analysis", map[string]string{
			"service": "productpage",
		})
		s.Error(err)
		s.Nil(result)
	})

	s.Run("returns error when service is missing", func() {
		result, err := s.GetPrompt("trace-analysis", map[string]string{
			"namespace": "bookinfo",
		})
		s.Error(err)
		s.Nil(result)
	})
}

func (s *KialiSuite) TestTraceAnalysisPrompt() {
	var mu sync.Mutex
	var capturedPaths []string
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedPaths = append(capturedPaths, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"traces":[]}`))
	}))
	s.InitMcpClient()

	result, err := s.GetPrompt("trace-analysis", map[string]string{
		"namespace": "bookinfo",
		"service":   "productpage",
	})

	s.Run("prompt executes without error", func() {
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("calls list_traces endpoint twice (recent + error traces)", func() {
		mu.Lock()
		defer mu.Unlock()
		count := 0
		for _, p := range capturedPaths {
			if p == "/api/chat/mcp/list_traces" {
				count++
			}
		}
		s.Equal(2, count, "expected list_traces to be called twice")
	})
	s.Run("result contains user and assistant messages", func() {
		s.Require().Len(result.Messages, 2)
		s.Equal("user", string(result.Messages[0].Role))
	})
}

func (s *KialiSuite) TestKialiToolsNotClusterAware() {
	kubeconfig := s.mockServer.Kubeconfig()
	for i := range 10 {
		kubeconfig.Contexts[strconv.Itoa(i)] = clientcmdapi.NewContext()
	}
	s.Cfg.KubeConfig = test.KubeconfigFile(s.T(), kubeconfig)
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NoError(err)
	s.Require().NotEmpty(tools.Tools)

	for _, tool := range tools.Tools {
		s.Run(tool.Name, func() {
			schema, ok := tool.InputSchema.(map[string]any)
			s.Require().True(ok, "expected InputSchema map for tool %s", tool.Name)
			properties, ok := schema["properties"].(map[string]any)
			s.Require().True(ok, "expected properties map for tool %s", tool.Name)
			s.NotContains(properties, "context", "kiali tool %s must not expose context parameter", tool.Name)
		})
	}
}

func TestKiali(t *testing.T) {
	suite.Run(t, new(KialiSuite))
}
