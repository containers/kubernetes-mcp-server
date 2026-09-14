//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// netobservToolCall calls a NetObserv MCP tool and returns the parsed response
func netobservToolCall(ctx context.Context, t *testing.T, client *mcp.Client, toolName string, args map[string]any) ([]mcp.TextContent, error) {
	t.Helper()

	result, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		return nil, err
	}

	var textContents []mcp.TextContent
	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			textContents = append(textContents, tc)
		}
	}

	return textContents, nil
}

// assertNetobservListFlows calls netobserv_list_flows and validates response structure
func assertNetobservListFlows(ctx context.Context, t *testing.T, client *mcp.Client, args map[string]any) {
	t.Helper()

	contents, err := netobservToolCall(ctx, t, client, "netobserv_list_flows", args)
	require.NoError(t, err, "list_flows should succeed")
	require.NotEmpty(t, contents, "should return content")

	// Parse JSON response
	var flows map[string]interface{}
	err = json.Unmarshal([]byte(contents[0].Text), &flows)
	require.NoError(t, err, "response should be valid JSON")
	require.Contains(t, flows, "result", "response should have 'result' field")
}

// assertNetobservGetMetrics calls netobserv_get_flow_metrics and validates response structure
func assertNetobservGetMetrics(ctx context.Context, t *testing.T, client *mcp.Client, args map[string]any) {
	t.Helper()

	contents, err := netobservToolCall(ctx, t, client, "netobserv_get_flow_metrics", args)
	require.NoError(t, err, "get_flow_metrics should succeed")
	require.NotEmpty(t, contents, "should return content")

	// Parse JSON response
	var metrics map[string]interface{}
	err = json.Unmarshal([]byte(contents[0].Text), &metrics)
	require.NoError(t, err, "response should be valid JSON")
	require.Contains(t, metrics, "status", "response should have 'status' field")
	require.Equal(t, "success", metrics["status"], "status should be 'success'")
}

// assertNetobservExportFlows calls netobserv_export_flows and validates CSV response
func assertNetobservExportFlows(ctx context.Context, t *testing.T, client *mcp.Client, args map[string]any) {
	t.Helper()

	contents, err := netobservToolCall(ctx, t, client, "netobserv_export_flows", args)
	require.NoError(t, err, "export_flows should succeed")
	require.NotEmpty(t, contents, "should return content")

	csvData := contents[0].Text
	require.Contains(t, csvData, "TimeFlowStartMs", "CSV should have expected headers")
	require.True(t, strings.Contains(csvData, "\n"), "CSV should have multiple lines")
}

// netobservToolCallExpectError calls a tool and expects an error
func netobservToolCallExpectError(ctx context.Context, t *testing.T, client *mcp.Client, toolName string, args map[string]any) {
	t.Helper()

	_, err := client.CallTool(ctx, toolName, args)
	require.Error(t, err, "tool call should fail")
}

// makeTimeRange creates a time range map for the last N minutes
func makeTimeRange(minutes int) map[string]string {
	return map[string]string{
		"last": fmt.Sprintf("%dm", minutes),
	}
}

// makeFilters creates a filters array from key-value pairs
func makeFilters(filters ...string) []string {
	return filters
}
