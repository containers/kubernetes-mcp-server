//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	clusterHealthCheckPrompt = "cluster-health-check"
	mcpVersionPrompt         = "mcp-version"
	checkPodLogsPrompt       = "check-pod-logs"

	mcpVersionPromptText = "Tell me what version of Kubernetes this cluster is running on"
)

// configuredPromptsTOML mirrors the prompts used during manual validation.
//
// The chart renders `config` through Helm's `tpl`, so `{{name}}` placeholders
// must be escaped as {{`{{name}}`}} or Helm tries to evaluate them.
const configuredPromptsTOML = `
[[prompts]]
name = "mcp-version"
title = "Give me the version"
description = "Give me the version"

[[prompts.messages]]
role = "user"
content = "Tell me what version of Kubernetes this cluster is running on"

[[prompts]]
name = "check-pod-logs"
title = "Check Pod Logs"
description = "Show the logs for a pod"

[[prompts.arguments]]
name = "name"
description = "Pod name"
required = true

[[prompts.arguments]]
name = "namespace"
description = "Pod namespace"
required = false

[[prompts.messages]]
role = "user"
content = "Show me the logs for pod {{` + "`{{name}}`" + `}} in {{` + "`{{namespace}}`" + `}}"
`

// clusterReaderRBACValues binds the server's ServiceAccount to the built-in
// `view` ClusterRole and adds cluster-scoped read access to namespaces and
// nodes, which `view` lacks. cluster-health-check needs nodes to fill its
// Nodes section.
func clusterReaderRBACValues() map[string]any {
	return map[string]any{
		"rbac": map[string]any{
			"create": true,
			"extraClusterRoles": []map[string]any{
				{
					"name": "cluster-reader",
					"rules": []map[string]any{
						{
							"apiGroups": []string{""},
							"resources": []string{"namespaces", "nodes"},
							"verbs":     []string{"get", "list", "watch"},
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
					"name": "cluster-reader",
					"roleRef": map[string]any{
						"name": "cluster-reader",
					},
				},
			},
		},
	}
}

func promptNames(prompts []*mcp.Prompt) []string {
	names := make([]string, len(prompts))
	for i, p := range prompts {
		names[i] = p.Name
	}
	return names
}

func findPrompt(prompts []*mcp.Prompt, name string) *mcp.Prompt {
	for _, p := range prompts {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// requirePrompt lists prompts and fails unless `name` is among them.
func requirePrompt(t *testing.T, mcpClient *test.McpClient, name string) *mcp.Prompt {
	t.Helper()
	result, err := mcpClient.ListPrompts()
	require.NoError(t, err, "list prompts")
	prompt := findPrompt(result.Prompts, name)
	require.NotNil(t, prompt, "prompt %q not listed; got: %v", name, promptNames(result.Prompts))
	return prompt
}

// requirePromptText calls prompts/get and returns the text of the first message.
func requirePromptText(t *testing.T, mcpClient *test.McpClient, name string, args map[string]string) string {
	t.Helper()
	result, err := mcpClient.GetPrompt(name, args)
	require.NoError(t, err, "get prompt %s with %v", name, args)
	require.NotEmpty(t, result.Messages, "prompt %s returned no messages", name)
	text, ok := result.Messages[0].Content.(*mcp.TextContent)
	require.True(t, ok, "prompt %s first message is %T, want TextContent", name, result.Messages[0].Content)
	return text.Text
}

// assertConfiguredPrompts runs the configured-prompt matrix against a client.
// `prefix` is prepended to every prompt name (the MCP Gateway prefixes prompts
// the same way it prefixes tools; direct connections use an empty prefix).
// `expectTitle` controls whether the prompt title is asserted; the Kuadrant
// MCP Gateway pinned in build/kuadrant.mk drops it (its MCP library predates
// the field), so the gateway path passes false.
func assertConfiguredPrompts(t *testing.T, mcpClient *test.McpClient, prefix string, expectTitle bool) {
	t.Helper()

	t.Run("prompts/list exposes configured prompts with their metadata", func(t *testing.T) {
		version := requirePrompt(t, mcpClient, prefix+mcpVersionPrompt)
		if expectTitle {
			require.Equal(t, "Give me the version", version.Title)
		}
		require.Empty(t, version.Arguments, "mcp-version takes no arguments")

		podLogs := requirePrompt(t, mcpClient, prefix+checkPodLogsPrompt)
		require.Len(t, podLogs.Arguments, 2)
		require.Equal(t, "name", podLogs.Arguments[0].Name)
		require.True(t, podLogs.Arguments[0].Required, "name must be required")
		require.Equal(t, "namespace", podLogs.Arguments[1].Name)
		require.False(t, podLogs.Arguments[1].Required, "namespace must be optional")
	})

	t.Run("prompt without arguments", func(t *testing.T) {
		text := requirePromptText(t, mcpClient, prefix+mcpVersionPrompt, nil)
		require.Equal(t, mcpVersionPromptText, text)
	})

	t.Run("required argument only drops the optional placeholder", func(t *testing.T) {
		text := requirePromptText(t, mcpClient, prefix+checkPodLogsPrompt, map[string]string{"name": "my-pod"})
		require.Equal(t, "Show me the logs for pod my-pod in ", text)
	})

	t.Run("required and optional arguments are both substituted", func(t *testing.T) {
		text := requirePromptText(t, mcpClient, prefix+checkPodLogsPrompt, map[string]string{
			"name": "my-pod", "namespace": "my-ns",
		})
		require.Equal(t, "Show me the logs for pod my-pod in my-ns", text)
	})

	t.Run("missing required argument is rejected", func(t *testing.T) {
		_, err := mcpClient.GetPrompt(prefix+checkPodLogsPrompt, map[string]string{"namespace": "my-ns"})
		require.Error(t, err, "expected error when required argument is missing")
		require.Contains(t, err.Error(), "required argument 'name' is missing")
	})
}

// assertClusterHealthCheck runs the built-in cluster-health-check matrix.
// `namespace` must exist in the cluster (the server's own namespace is used).
// See assertConfiguredPrompts for `prefix` and `expectTitle`.
func assertClusterHealthCheck(t *testing.T, mcpClient *test.McpClient, prefix string, expectTitle bool, namespace string) {
	t.Helper()
	name := prefix + clusterHealthCheckPrompt

	t.Run("prompts/list exposes the built-in prompt with optional arguments", func(t *testing.T) {
		prompt := requirePrompt(t, mcpClient, name)
		if expectTitle {
			require.Equal(t, "Cluster Health Check", prompt.Title)
		}
		require.Len(t, prompt.Arguments, 2, "arguments: %+v", prompt.Arguments)
		require.Equal(t, "namespace", prompt.Arguments[0].Name)
		require.False(t, prompt.Arguments[0].Required)
		require.Equal(t, "check_events", prompt.Arguments[1].Name)
		require.False(t, prompt.Arguments[1].Required)
	})

	t.Run("namespace scoped without events", func(t *testing.T) {
		text := requirePromptText(t, mcpClient, name, map[string]string{
			"namespace": namespace, "check_events": "false",
		})
		require.Contains(t, text, "# Cluster Health Check Diagnostic Data")
		require.Contains(t, text, fmt.Sprintf("**Scope:** Namespace `%s`", namespace))
		require.NotContains(t, text, "**WARNING:**")
		require.Contains(t, text, "## 1. Nodes", "nodes section requires cluster-scoped node list RBAC")
		require.Contains(t, text, "## 3. Pods")
		require.NotContains(t, text, "## 6. Recent Events")
	})

	t.Run("namespace scoped with events", func(t *testing.T) {
		text := requirePromptText(t, mcpClient, name, map[string]string{
			"namespace": namespace, "check_events": "true",
		})
		require.Contains(t, text, fmt.Sprintf("**Scope:** Namespace `%s`", namespace))
		require.Contains(t, text, "## 6. Recent Events (Last Hour)")
	})

	t.Run("no arguments is cluster-wide with events", func(t *testing.T) {
		text := requirePromptText(t, mcpClient, name, nil)
		require.Contains(t, text, "**Scope:** All namespaces")
		require.Contains(t, text, "## 6. Recent Events (Last Hour)")
	})

	t.Run("unknown namespace warns and falls back to cluster-wide", func(t *testing.T) {
		text := requirePromptText(t, mcpClient, name, map[string]string{
			"namespace": "does-not-exist-e2e", "check_events": "false",
		})
		require.Contains(t, text, "**WARNING:** Namespace 'does-not-exist-e2e' not found or not accessible.")
		require.Contains(t, text, "**Scope:** All namespaces")
	})
}

type promptsState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var promptsTS testState[promptsState]

// TestPrompts validates built-in and configured prompts through the MCP
// protocol against a directly reachable (no gateway) server deployment.
func TestPrompts(t *testing.T) {
	f := features.New("prompts").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "prompts",
				withConfig(configuredPromptsTOML),
				withValues(clusterReaderRBACValues()),
			)
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return promptsTS.set(ctx, &promptsState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("prompts/list includes built-in and configured prompts", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := promptsTS.get(ctx)

			result, err := s.mcpClient.ListPrompts()
			require.NoError(t, err, "list prompts")
			names := promptNames(result.Prompts)
			require.Contains(t, names, clusterHealthCheckPrompt)
			require.Contains(t, names, mcpVersionPrompt)
			require.Contains(t, names, checkPodLogsPrompt)

			return ctx
		}).
		Assess("configured prompts render their arguments", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := promptsTS.get(ctx)
			assertConfiguredPrompts(t, s.mcpClient, "", true)
			return ctx
		}).
		Assess("cluster-health-check gathers live cluster data", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := promptsTS.get(ctx)
			assertClusterHealthCheck(t, s.mcpClient, "", true, s.dep.namespace)
			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}
