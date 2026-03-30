package mcp

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// KialiRequireTLSSuite tests Layer 2 (runtime) TLS enforcement for the Kiali toolset.
// It uses a plain HTTP mock server and verifies that TLSEnforcingTransport blocks
// requests when require_tls is enabled.
type KialiRequireTLSSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
}

func (s *KialiRequireTLSSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.mockServer.Config().BearerToken = "token-xyz"
}

func (s *KialiRequireTLSSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *KialiRequireTLSSuite) setupConfig(requireTLS bool) {
	kubeConfig := s.Cfg.KubeConfig
	// Parse config without require_tls to bypass Layer 1 (config-time) URL validation,
	// then enable require_tls to test Layer 2 (runtime) TLSEnforcingTransport enforcement.
	s.Cfg = test.Must(config.ReadToml([]byte(fmt.Sprintf(`
		toolsets = ["kiali"]
		[toolset_configs.kiali]
		url = "%s"
	`, s.mockServer.Config().Host))))
	s.Cfg.KubeConfig = kubeConfig
	s.Cfg.RequireTLS = requireTLS
}

func (s *KialiRequireTLSSuite) TestRequireTLS_BlocksHTTPRequests() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"test-trace-123","spans":[]}`))
	}))
	s.setupConfig(true)
	s.InitMcpClient()

	s.Run("kiali tool call to HTTP server is blocked", func() {
		toolResult, err := s.CallTool("kiali_get_traces", map[string]interface{}{
			"traceId": "test-trace-123",
		})
		s.Require().Nilf(err, "MCP protocol error: %v", err)
		s.True(toolResult.IsError, "tool call should return an error result")
		s.Contains(
			toolResult.Content[0].(*mcp.TextContent).Text,
			"secure scheme required",
			"error should indicate TLS enforcement",
		)
	})
}

func (s *KialiRequireTLSSuite) TestRequireTLS_AllowsHTTPWhenDisabled() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"test-trace-456","spans":[]}`))
	}))
	s.setupConfig(false)
	s.InitMcpClient()

	s.Run("kiali tool call to HTTP server succeeds", func() {
		toolResult, err := s.CallTool("kiali_get_traces", map[string]interface{}{
			"traceId": "test-trace-456",
		})
		s.Nilf(err, "call tool failed: %v", err)
		s.Falsef(toolResult.IsError, "call tool failed")
		s.Contains(
			toolResult.Content[0].(*mcp.TextContent).Text,
			"test-trace-456",
			"response should contain trace ID",
		)
	})
}

func TestKialiRequireTLS(t *testing.T) {
	suite.Run(t, new(KialiRequireTLSSuite))
}

// CoreRequireTLSSuite tests that enabling require_tls does not break core
// Kubernetes tools. The envtest API server uses HTTPS, so core tools should
// continue to work normally when TLS enforcement is enabled.
type CoreRequireTLSSuite struct {
	BaseMcpSuite
}

func (s *CoreRequireTLSSuite) TestRequireTLS_CoreToolsStillWork() {
	kubeConfig := s.Cfg.KubeConfig
	s.Cfg = test.Must(config.ReadToml([]byte(`
		require_tls = true
		list_output = "yaml"
	`)))
	s.Cfg.KubeConfig = kubeConfig
	s.InitMcpClient()

	s.Run("namespaces_list succeeds with require_tls enabled", func() {
		toolResult, err := s.CallTool("namespaces_list", map[string]interface{}{})
		s.Nilf(err, "call tool failed: %v", err)
		s.Falsef(toolResult.IsError, "call tool failed")
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(*mcp.TextContent).Text), &decoded)
		s.Require().NoError(err, "expected valid YAML response")
		s.True(slices.ContainsFunc(decoded, func(ns unstructured.Unstructured) bool {
			return ns.GetName() == "default"
		}), "expected default namespace in the list")
	})
}

func TestCoreRequireTLS(t *testing.T) {
	suite.Run(t, new(CoreRequireTLSSuite))
}
