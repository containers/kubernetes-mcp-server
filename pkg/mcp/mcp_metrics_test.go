package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/suite"
)

type McpMetricsSuite struct {
	BaseMcpSuite
}

func TestMcpMetrics(t *testing.T) {
	suite.Run(t, new(McpMetricsSuite))
}

func (s *McpMetricsSuite) TestToolCallMetricsRecorded() {
	s.InitMcpClient()
	toolResult, err := s.CallTool("namespaces_list", map[string]interface{}{})
	s.Require().Nilf(err, "call tool failed %v", err)
	s.Require().Falsef(toolResult.IsError, "call tool failed")

	stats := s.mcpServer.GetMetrics().GetStats()

	s.Run("increments total tool call count", func() {
		s.GreaterOrEqual(stats.TotalToolCalls, int64(1))
	})

	s.Run("records tool call by name", func() {
		s.GreaterOrEqual(stats.ToolCallsByName["namespaces_list"], int64(1))
	})

	s.Run("does not record errors for successful calls", func() {
		s.Equal(int64(0), stats.ToolErrorsByName["namespaces_list"])
	})
}

func (s *McpMetricsSuite) TestToolCallErrorMetricsRecorded() {
	s.InitMcpClient()

	// Call a tool with missing required params to trigger an error result
	toolResult, err := s.CallTool("pods_get", map[string]interface{}{})
	s.Require().Nilf(err, "call tool should not return transport error %v", err)
	s.Require().True(toolResult.IsError, "call tool should return error result for missing params")

	stats := s.mcpServer.GetMetrics().GetStats()

	s.Run("increments total tool call count", func() {
		s.GreaterOrEqual(stats.TotalToolCalls, int64(1))
	})

	s.Run("records tool call by name", func() {
		s.GreaterOrEqual(stats.ToolCallsByName["pods_get"], int64(1))
	})
}

func (s *McpMetricsSuite) TestMultipleToolCallsAggregate() {
	s.InitMcpClient()

	_, err := s.CallTool("namespaces_list", map[string]interface{}{})
	s.Require().Nilf(err, "first call failed %v", err)

	_, err = s.CallTool("namespaces_list", map[string]interface{}{})
	s.Require().Nilf(err, "second call failed %v", err)

	_, err = s.CallTool("pods_list", map[string]interface{}{})
	s.Require().Nilf(err, "third call failed %v", err)

	stats := s.mcpServer.GetMetrics().GetStats()

	s.Run("total count matches sum of calls", func() {
		s.GreaterOrEqual(stats.TotalToolCalls, int64(3))
	})

	s.Run("per-tool counts are correct", func() {
		s.GreaterOrEqual(stats.ToolCallsByName["namespaces_list"], int64(2))
		s.GreaterOrEqual(stats.ToolCallsByName["pods_list"], int64(1))
	})
}

func (s *McpMetricsSuite) TestPrometheusHandlerReflectsToolCalls() {
	s.InitMcpClient()

	_, err := s.CallTool("namespaces_list", map[string]interface{}{})
	s.Require().Nilf(err, "call tool failed %v", err)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.mcpServer.GetMetrics().PrometheusHandler().ServeHTTP(rec, req)

	s.Run("returns 200 OK", func() {
		s.Equal(http.StatusOK, rec.Code)
	})

	body := rec.Body.String()

	s.Run("contains tool call metrics", func() {
		s.Contains(body, "k8s_mcp_tool_calls")
	})

	s.Run("contains tool duration metrics", func() {
		s.Contains(body, "k8s_mcp_tool_duration")
	})

	s.Run("contains server info metric", func() {
		s.Contains(body, "k8s_mcp_server_info")
	})
}

func (s *McpMetricsSuite) TestStatsInitializedBeforeCalls() {
	s.InitMcpClient()
	stats := s.mcpServer.GetMetrics().GetStats()

	s.Run("maps are initialized", func() {
		s.NotNil(stats.ToolCallsByName)
		s.NotNil(stats.ToolErrorsByName)
		s.NotNil(stats.HTTPRequestsByPath)
		s.NotNil(stats.HTTPRequestsByStatus)
		s.NotNil(stats.HTTPRequestsByMethod)
	})

	s.Run("start time is set", func() {
		s.Greater(stats.StartTime, int64(0))
	})
}

func (s *McpMetricsSuite) TestMetricsShutdown() {
	s.InitMcpClient()

	_, err := s.CallTool("namespaces_list", map[string]interface{}{})
	s.Require().Nilf(err, "call tool failed %v", err)

	s.Run("completes without error", func() {
		err := s.mcpServer.GetMetrics().Shutdown(context.Background())
		s.NoError(err)
	})
}

func (s *McpMetricsSuite) TestMetricsExportedToConfigEndpoint() {
	var requestReceived atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s.T().Setenv("OTEL_METRICS_EXPORTER", "")
	s.T().Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	s.T().Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")

	s.Require().NoError(toml.Unmarshal([]byte(`
		[telemetry]
		endpoint = "`+server.URL+`"
		protocol = "http/protobuf"
	`), s.Cfg))
	s.InitMcpClient()

	_, err := s.CallTool("namespaces_list", map[string]interface{}{})
	s.Require().Nilf(err, "call tool failed %v", err)

	// Shutdown flushes the periodic reader, triggering export to the configured endpoint
	s.Require().NoError(s.mcpServer.GetMetrics().Shutdown(context.Background()))

	s.Run("exports to TOML-configured endpoint", func() {
		s.True(requestReceived.Load(), "metrics should be exported to the TOML-configured endpoint")
	})
}
