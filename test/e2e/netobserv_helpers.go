//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// assertNetobservListFlows calls netobserv_list_flows and validates response structure
func assertNetobservListFlows(t *testing.T, mcpClient *test.McpClient, args map[string]any) {
	t.Helper()

	result, err := mcpClient.CallTool("netobserv_list_flows", args)
	require.NoError(t, err, "list_flows should succeed")
	require.False(t, result.IsError, "list_flows should not return error")
	require.NotEmpty(t, result.Content, "should return content")

	// Parse JSON response
	textContent := result.Content[0].(mcp.TextContent)
	var flows map[string]interface{}
	err = json.Unmarshal([]byte(textContent.Text), &flows)
	require.NoError(t, err, "response should be valid JSON")
	require.Contains(t, flows, "result", "response should have 'result' field")
}

// assertNetobservGetMetrics calls netobserv_get_flow_metrics and validates response structure
func assertNetobservGetMetrics(t *testing.T, mcpClient *test.McpClient, args map[string]any) {
	t.Helper()

	result, err := mcpClient.CallTool("netobserv_get_flow_metrics", args)
	require.NoError(t, err, "get_flow_metrics should succeed")
	require.False(t, result.IsError, "get_flow_metrics should not return error")
	require.NotEmpty(t, result.Content, "should return content")

	// Parse JSON response
	textContent := result.Content[0].(mcp.TextContent)
	var metrics map[string]interface{}
	err = json.Unmarshal([]byte(textContent.Text), &metrics)
	require.NoError(t, err, "response should be valid JSON")
	require.Contains(t, metrics, "status", "response should have 'status' field")
	require.Equal(t, "success", metrics["status"], "status should be 'success'")
}

// assertNetobservExportFlows calls netobserv_export_flows and validates CSV response
func assertNetobservExportFlows(t *testing.T, mcpClient *test.McpClient, args map[string]any) {
	t.Helper()

	result, err := mcpClient.CallTool("netobserv_export_flows", args)
	require.NoError(t, err, "export_flows should succeed")
	require.False(t, result.IsError, "export_flows should not return error")
	require.NotEmpty(t, result.Content, "should return content")

	textContent := result.Content[0].(mcp.TextContent)
	csvData := textContent.Text
	require.Contains(t, csvData, "TimeFlowStartMs", "CSV should have expected headers")
	require.True(t, strings.Contains(csvData, "\n"), "CSV should have multiple lines")
}

// assertNetobservToolCallError calls a tool and expects an error
func assertNetobservToolCallError(t *testing.T, mcpClient *test.McpClient, toolName string, args map[string]any) {
	t.Helper()

	result, err := mcpClient.CallTool(toolName, args)
	// Either transport error OR tool error is acceptable
	if err == nil {
		require.True(t, result.IsError, "tool call should return error")
	}
}

// makeTimeRange creates a time range value for the last N minutes (in seconds)
func makeTimeRange(minutes int) int {
	return minutes * 60
}

// makeFilters creates a filters array from key-value pairs
func makeFilters(filters ...string) []string {
	return filters
}
