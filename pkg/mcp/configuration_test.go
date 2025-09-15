package mcp

import (
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	v1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"
)

func TestConfigurationView(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		toolResult, err := c.callTool("configuration_view", map[string]interface{}{})
		t.Run("configuration_view returns configuration", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
		})
		var decoded *v1.Config
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		t.Run("configuration_view has yaml content", func(t *testing.T) {
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
			}
		})
		t.Run("configuration_view returns current-context", func(t *testing.T) {
			if decoded.CurrentContext != "fake-context" {
				t.Errorf("fake-context not found: %v", decoded.CurrentContext)
			}
		})
		t.Run("configuration_view returns context info", func(t *testing.T) {
			if len(decoded.Contexts) != 1 {
				t.Errorf("invalid context count, expected 1, got %v", len(decoded.Contexts))
			}
			if decoded.Contexts[0].Name != "fake-context" {
				t.Errorf("fake-context not found: %v", decoded.Contexts)
			}
			if decoded.Contexts[0].Context.Cluster != "fake" {
				t.Errorf("fake-cluster not found: %v", decoded.Contexts)
			}
			if decoded.Contexts[0].Context.AuthInfo != "fake" {
				t.Errorf("fake-auth not found: %v", decoded.Contexts)
			}
		})
		t.Run("configuration_view returns cluster info", func(t *testing.T) {
			if len(decoded.Clusters) != 1 {
				t.Errorf("invalid cluster count, expected 1, got %v", len(decoded.Clusters))
			}
			if decoded.Clusters[0].Name != "fake" {
				t.Errorf("fake-cluster not found: %v", decoded.Clusters)
			}
			if decoded.Clusters[0].Cluster.Server != "https://127.0.0.1:6443" {
				t.Errorf("fake-server not found: %v", decoded.Clusters)
			}
		})
		t.Run("configuration_view returns auth info", func(t *testing.T) {
			if len(decoded.AuthInfos) != 1 {
				t.Errorf("invalid auth info count, expected 1, got %v", len(decoded.AuthInfos))
			}
			if decoded.AuthInfos[0].Name != "fake" {
				t.Errorf("fake-auth not found: %v", decoded.AuthInfos)
			}
		})
		toolResult, err = c.callTool("configuration_view", map[string]interface{}{
			"minified": false,
		})
		t.Run("configuration_view with minified=false returns configuration", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
		})
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		t.Run("configuration_view with minified=false has yaml content", func(t *testing.T) {
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
			}
		})
		t.Run("configuration_view with minified=false returns additional context info", func(t *testing.T) {
			if len(decoded.Contexts) != 2 {
				t.Fatalf("invalid context count, expected2, got %v", len(decoded.Contexts))
			}
			if decoded.Contexts[0].Name != "additional-context" {
				t.Errorf("additional-context not found: %v", decoded.Contexts)
			}
			if decoded.Contexts[0].Context.Cluster != "additional-cluster" {
				t.Errorf("additional-cluster not found: %v", decoded.Contexts)
			}
			if decoded.Contexts[0].Context.AuthInfo != "additional-auth" {
				t.Errorf("additional-auth not found: %v", decoded.Contexts)
			}
			if decoded.Contexts[1].Name != "fake-context" {
				t.Errorf("fake-context not found: %v", decoded.Contexts)
			}
		})
		t.Run("configuration_view with minified=false returns cluster info", func(t *testing.T) {
			if len(decoded.Clusters) != 2 {
				t.Errorf("invalid cluster count, expected 2, got %v", len(decoded.Clusters))
			}
			if decoded.Clusters[0].Name != "additional-cluster" {
				t.Errorf("additional-cluster not found: %v", decoded.Clusters)
			}
		})
		t.Run("configuration_view with minified=false returns auth info", func(t *testing.T) {
			if len(decoded.AuthInfos) != 2 {
				t.Errorf("invalid auth info count, expected 2, got %v", len(decoded.AuthInfos))
			}
			if decoded.AuthInfos[0].Name != "additional-auth" {
				t.Errorf("additional-auth not found: %v", decoded.AuthInfos)
			}
		})
	})
}

func TestConfigurationViewInCluster(t *testing.T) {
	kubernetes.InClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{
			Host:        "https://kubernetes.default.svc",
			BearerToken: "fake-token",
		}, nil
	}
	defer func() {
		kubernetes.InClusterConfig = rest.InClusterConfig
	}()
	testCase(t, func(c *mcpContext) {
		toolResult, err := c.callTool("configuration_view", map[string]interface{}{})
		t.Run("configuration_view returns configuration", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
		})
		var decoded *v1.Config
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		t.Run("configuration_view has yaml content", func(t *testing.T) {
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
			}
		})
		t.Run("configuration_view returns current-context", func(t *testing.T) {
			if decoded.CurrentContext != "context" {
				t.Fatalf("context not found: %v", decoded.CurrentContext)
			}
		})
		t.Run("configuration_view returns context info", func(t *testing.T) {
			if len(decoded.Contexts) != 1 {
				t.Fatalf("invalid context count, expected 1, got %v", len(decoded.Contexts))
			}
			if decoded.Contexts[0].Name != "context" {
				t.Fatalf("context not found: %v", decoded.Contexts)
			}
			if decoded.Contexts[0].Context.Cluster != "cluster" {
				t.Fatalf("cluster not found: %v", decoded.Contexts)
			}
			if decoded.Contexts[0].Context.AuthInfo != "user" {
				t.Fatalf("user not found: %v", decoded.Contexts)
			}
		})
		t.Run("configuration_view returns cluster info", func(t *testing.T) {
			if len(decoded.Clusters) != 1 {
				t.Fatalf("invalid cluster count, expected 1, got %v", len(decoded.Clusters))
			}
			if decoded.Clusters[0].Name != "cluster" {
				t.Fatalf("cluster not found: %v", decoded.Clusters)
			}
			if decoded.Clusters[0].Cluster.Server != "https://kubernetes.default.svc" {
				t.Fatalf("server not found: %v", decoded.Clusters)
			}
		})
		t.Run("configuration_view returns auth info", func(t *testing.T) {
			if len(decoded.AuthInfos) != 1 {
				t.Fatalf("invalid auth info count, expected 1, got %v", len(decoded.AuthInfos))
			}
			if decoded.AuthInfos[0].Name != "user" {
				t.Fatalf("user not found: %v", decoded.AuthInfos)
			}
		})
	})
}

func TestContextsList(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		toolResult, err := c.callTool("contexts_list", map[string]interface{}{})
		require.NoError(t, err, "contexts_list tool call should not fail")

		content := toolResult.Content[0].(mcp.TextContent).Text

		// Expected exact output format based on test setup with fake contexts
		expectedOutput := `Available Kubernetes contexts (2 total, current: fake-context):

Format: [*] CONTEXT_NAME -> CLUSTER_SERVER_URL
        (* indicates the current active context)

Contexts:
─────────
* fake-context -> https://127.0.0.1:6443
  additional-context -> 
─────────

Usage:
To use a specific context with any tool, add the 'context' parameter:
Example: {"name": "pods_list", "arguments": {"context": "fake-context"}}`

		// Split both expected and actual content into lines for comparison
		expectedLines := strings.Split(expectedOutput, "\n")
		actualLines := strings.Split(content, "\n")

		// First, verify line count matches
		assert.Equal(t, len(expectedLines), len(actualLines), "line count should match")

		// Then verify each expected line is present in the actual content
		// Note: Context lines might appear in different order due to Go map iteration randomness
		for i, expectedLine := range expectedLines {
			// For context lines (lines with -> in them), check if any actual line matches
			if strings.Contains(expectedLine, " -> ") {
				found := false
				for _, actualLine := range actualLines {
					if actualLine == expectedLine {
						found = true
						break
					}
				}
				assert.True(t, found, "expected context line should be present: %s", expectedLine)
			} else {
				// For non-context lines, they should appear in the same position
				if i < len(actualLines) {
					assert.Equal(t, expectedLine, actualLines[i], "line %d should match", i+1)
				} else {
					t.Errorf("expected line %d not found in actual output: %s", i+1, expectedLine)
				}
			}
		}
	})
}
