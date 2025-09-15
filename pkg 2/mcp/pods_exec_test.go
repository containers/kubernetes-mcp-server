package mcp

import (
	"bytes"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/mark3labs/mcp-go/mcp"
	"io"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"testing"
)

func TestPodsExec(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		mockServer := NewMockServer()
		defer mockServer.Close()
		c.withKubeConfig(mockServer.config)
		mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path != "/api/v1/namespaces/default/pods/pod-to-exec/exec" {
				return
			}
			var stdin, stdout bytes.Buffer
			ctx, err := createHTTPStreams(w, req, &StreamOptions{
				Stdin:  &stdin,
				Stdout: &stdout,
			})
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
			defer func(conn io.Closer) { _ = conn.Close() }(ctx.conn)
			_, _ = io.WriteString(ctx.stdoutStream, "command:"+strings.Join(req.URL.Query()["command"], " ")+"\n")
			_, _ = io.WriteString(ctx.stdoutStream, "container:"+strings.Join(req.URL.Query()["container"], " ")+"\n")
		}))
		mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path != "/api/v1/namespaces/default/pods/pod-to-exec" {
				return
			}
			writeObject(w, &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "pod-to-exec",
				},
				Spec: v1.PodSpec{Containers: []v1.Container{{Name: "container-to-exec"}}},
			})
		}))
		podsExecNilNamespace, err := c.callTool("pods_exec", map[string]interface{}{
			"name":    "pod-to-exec",
			"command": []interface{}{"ls", "-l"},
		})
		t.Run("pods_exec with name and nil namespace returns command output", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if podsExecNilNamespace.IsError {
				t.Fatalf("call tool failed")
			}
			if !strings.Contains(podsExecNilNamespace.Content[0].(mcp.TextContent).Text, "command:ls -l\n") {
				t.Errorf("unexpected result %v", podsExecNilNamespace.Content[0].(mcp.TextContent).Text)
			}
		})
		podsExecInNamespace, err := c.callTool("pods_exec", map[string]interface{}{
			"namespace": "default",
			"name":      "pod-to-exec",
			"command":   []interface{}{"ls", "-l"},
		})
		t.Run("pods_exec with name and namespace returns command output", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if podsExecInNamespace.IsError {
				t.Fatalf("call tool failed")
			}
			if !strings.Contains(podsExecNilNamespace.Content[0].(mcp.TextContent).Text, "command:ls -l\n") {
				t.Errorf("unexpected result %v", podsExecInNamespace.Content[0].(mcp.TextContent).Text)
			}
		})
		podsExecInNamespaceAndContainer, err := c.callTool("pods_exec", map[string]interface{}{
			"namespace": "default",
			"name":      "pod-to-exec",
			"command":   []interface{}{"ls", "-l"},
			"container": "a-specific-container",
		})
		t.Run("pods_exec with name, namespace, and container returns command output", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if podsExecInNamespaceAndContainer.IsError {
				t.Fatalf("call tool failed")
			}
			if !strings.Contains(podsExecInNamespaceAndContainer.Content[0].(mcp.TextContent).Text, "command:ls -l\n") {
				t.Errorf("unexpected result %v", podsExecInNamespaceAndContainer.Content[0].(mcp.TextContent).Text)
			}
			if !strings.Contains(podsExecInNamespaceAndContainer.Content[0].(mcp.TextContent).Text, "container:a-specific-container\n") {
				t.Errorf("expected container name not found %v", podsExecInNamespaceAndContainer.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}

func TestPodsExecDenied(t *testing.T) {
	deniedResourcesServer := &config.StaticConfig{DeniedResources: []config.GroupVersionKind{{Version: "v1", Kind: "Pod"}}}
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		c.withEnvTest()
		podsRun, _ := c.callTool("pods_exec", map[string]interface{}{
			"namespace": "default",
			"name":      "pod-to-exec",
			"command":   []interface{}{"ls", "-l"},
			"container": "a-specific-container",
		})
		t.Run("pods_exec has error", func(t *testing.T) {
			if !podsRun.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("pods_exec describes denial", func(t *testing.T) {
			expectedMessage := "failed to exec in pod pod-to-exec in namespace default: resource not allowed: /v1, Kind=Pod"
			if podsRun.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, podsRun.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}
