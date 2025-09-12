package mcp

import (
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/client-go/rest"
	v1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"
	"testing"
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
				t.Errorf("invalid context count, expected2, got %v", len(decoded.Contexts))
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
