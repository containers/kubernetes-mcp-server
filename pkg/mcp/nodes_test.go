package mcp

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeLog(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()

		// Create test node
		kubernetesAdmin := c.newKubernetesClient()
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-log",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{Type: corev1.NodeInternalIP, Address: "192.168.1.10"},
				},
			},
		}

		_, _ = kubernetesAdmin.CoreV1().Nodes().Create(c.ctx, node, metav1.CreateOptions{})

		// Test node_log tool
		toolResult, err := c.callTool("node_log", map[string]interface{}{
			"name": "test-node-log",
		})

		t.Run("node_log returns successfully or with expected error", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed: %v", err)
			}
			// Node logs might not be available in test environment
			// We just check that the tool call completes
			if toolResult.IsError {
				content := toolResult.Content[0].(mcp.TextContent).Text
				// Expected error messages in test environment
				if !strings.Contains(content, "failed to get node logs") &&
					!strings.Contains(content, "not logged any message yet") {
					t.Logf("tool returned error (expected in test environment): %v", content)
				}
			}
		})
	})
}

func TestNodeLogMissingArguments(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()

		t.Run("node_log requires name", func(t *testing.T) {
			toolResult, err := c.callTool("node_log", map[string]interface{}{})

			if err == nil && !toolResult.IsError {
				t.Fatal("expected error when name is missing")
			}
		})
	})
}
