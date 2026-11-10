//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
)

// obsTracesRBACValues returns Helm values granting the server SA permission
// to list Tempo CRDs (TempoStacks, TempoMonolithics) at cluster scope.
func obsTracesRBACValues() map[string]any {
	return map[string]any{
		"rbac": map[string]any{
			"create": true,
			"extraClusterRoles": []map[string]any{
				{
					"name": "obs-tracing",
					"rules": []map[string]any{
						{
							"apiGroups": []string{"tempo.grafana.com"},
							"resources": []string{"tempostacks", "tempomonolithics"},
							"verbs":     []string{"list"},
						},
					},
				},
			},
			"extraClusterRoleBindings": []map[string]any{
				{
					"name": "view",
					"roleRef": map[string]any{
						"name":     "view",
						"external": true,
					},
				},
				{
					"name": "obs-tracing",
					"roleRef": map[string]any{
						"name": "obs-tracing",
					},
				},
			},
		},
	}
}

// obsLogsRBACValues returns Helm values granting the server SA permission
// to list Loki CRDs (LokiStacks) and Routes at cluster scope.
func obsLogsRBACValues() map[string]any {
	return map[string]any{
		"rbac": map[string]any{
			"create": true,
			"extraClusterRoles": []map[string]any{
				{
					"name": "obs-loki",
					"rules": []map[string]any{
						{
							"apiGroups": []string{""},
							"resources": []string{"namespaces", "pods"},
							"verbs":     []string{"get", "list", "watch"},
						},
						{
							"apiGroups": []string{"loki.grafana.com"},
							"resources": []string{"lokistacks"},
							"verbs":     []string{"get", "list", "watch"},
						},
						{
							"apiGroups": []string{"route.openshift.io"},
							"resources": []string{"routes"},
							"verbs":     []string{"get", "list", "watch"},
						},
						{
							"apiGroups":     []string{"loki.grafana.com"},
							"resources":     []string{"application", "infrastructure", "audit", "network"},
							"resourceNames": []string{"logs"},
							"verbs":         []string{"get"},
						},
					},
				},
			},
			"extraClusterRoleBindings": []map[string]any{
				{
					"name": "view",
					"roleRef": map[string]any{
						"name":     "view",
						"external": true,
					},
				},
				{
					"name": "obs-loki",
					"roleRef": map[string]any{
						"name": "obs-loki",
					},
				},
			},
		},
	}
}

// obsMetricsConfig returns the TOML config for deploying the MCP server
// with observability/metrics toolset pointing at the in-cluster Prometheus.
func obsMetricsConfig() string {
	return `
toolsets = ["observability/metrics"]

[toolset_configs."observability/metrics"]
prometheus_url = "http://prometheus-k8s.monitoring.svc:9090"
alertmanager_url = "http://alertmanager-main.monitoring.svc:9093"
`
}

// obsTracesConfig returns the TOML config for deploying the MCP server
// with observability/traces toolset pointing at the in-cluster Tempo.
func obsTracesConfig() string {
	return `
toolsets = ["observability/traces"]
`
}

// obsLogsConfig returns the TOML config for deploying the MCP server
// with observability/logs toolset pointing at the in-cluster Loki.
func obsLogsConfig() string {
	return `
toolsets = ["observability/logs"]
`
}

// obsOtelColConfig returns the TOML config for deploying the MCP server
// with observability/otelcol toolset.
func obsOtelColConfig() string {
	return `
toolsets = ["observability/otelcol"]
`
}

// obsAllConfig returns the TOML config for deploying the MCP server
// with all observability toolsets enabled.
func obsAllConfig() string {
	return `
toolsets = ["observability/metrics", "observability/traces", "observability/logs", "observability/otelcol"]

[toolset_configs."observability/metrics"]
prometheus_url = "http://prometheus-k8s.monitoring.svc:9090"
alertmanager_url = "http://alertmanager-main.monitoring.svc:9093"
`
}

// requireObsToolCallSuccess calls a tool and asserts it returns non-error structured content.
func requireObsToolCallSuccess(t *testing.T, mcpClient *test.McpClient, tool string, args map[string]any) string {
	t.Helper()
	return requireToolCallSuccess(t, mcpClient, tool, args)
}

// requireObsToolCallError calls a tool and asserts it returns an error.
func requireObsToolCallError(t *testing.T, mcpClient *test.McpClient, tool string, args map[string]any) string {
	t.Helper()
	return requireToolCallError(t, mcpClient, tool, args)
}

const (
	tempoRetryTimeout  = 60 * time.Second
	tempoRetryInterval = 5 * time.Second
)

// isTransientBackendError reports whether a tool error result is likely a
// temporary connectivity failure against the Tempo backend.
func isTransientBackendError(output string) bool {
	return strings.Contains(output, "connection refused") ||
		strings.Contains(output, "i/o timeout") ||
		strings.Contains(output, "connection reset") ||
		strings.Contains(output, "no such host") ||
		strings.Contains(output, "temporary failure in name resolution") ||
		strings.Contains(output, "TLS handshake timeout")
}

// requireObsToolCallWithRetry calls a tool and retries on transient backend
// connectivity errors (common with Tempo after TempoStack reports Ready).
func requireObsToolCallWithRetry(t *testing.T, mcpClient *test.McpClient, tool string, args map[string]any) string {
	t.Helper()

	deadline := time.Now().Add(tempoRetryTimeout)
	attempt := 0

	for {
		attempt++
		result, err := mcpClient.CallTool(tool, args)
		require.NoError(t, err, "%s transport error", tool)

		output := textContent(result)
		if !result.IsError {
			return output
		}

		if isTransientBackendError(output) && time.Now().Before(deadline) {
			t.Logf("%s attempt %d: transient backend error, retrying in %s: %s",
				tool, attempt, tempoRetryInterval, output)
			time.Sleep(tempoRetryInterval)
			continue
		}

		require.False(t, result.IsError, "%s returned tool error: %s", tool, output)
		return output
	}
}

// detectThanos checks whether the metrics backend is Thanos by looking for
// the thanos_build_info metric.
func detectThanos(t *testing.T, mcpClient *test.McpClient) bool {
	t.Helper()
	output := requireObsToolCallSuccess(t, mcpClient, "get_series", map[string]any{
		"matches": `thanos_build_info{component="query"}`,
	})
	return strings.Contains(output, "thanos_build_info")
}

// skipIfThanos skips the test when the backend is Thanos, which may not
// support all Prometheus API endpoints used by guardrail checks.
func skipIfThanos(t *testing.T, mcpClient *test.McpClient) {
	t.Helper()
	if detectThanos(t, mcpClient) {
		t.Skip("Thanos backend detected — skipping test that requires vanilla Prometheus")
	}
}

// lokiBaseArgs returns the common instance/tenant args for Loki tool tests.
func lokiBaseArgs() map[string]any {
	return map[string]any{
		"lokiNamespace": "obs-loki",
		"lokiName":      "obs-loki",
		"tenant":        "network",
	}
}

// mergeLokiArgs merges base Loki args with extra args.
func mergeLokiArgs(extra map[string]any) map[string]any {
	base := lokiBaseArgs()
	args := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		args[k] = v
	}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

// parseStructuredContent extracts the structuredContent map from a tool output string.
func parseStructuredContent(t *testing.T, output string) map[string]any {
	t.Helper()
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &result), "unmarshal tool output")
	sc, ok := result["structuredContent"].(map[string]any)
	require.True(t, ok, "expected structuredContent in result")
	return sc
}
