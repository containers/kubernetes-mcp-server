package mcp

import (
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func TestPodsRun(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()
		t.Run("pods_run with nil image returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("pods_run", map[string]interface{}{})
			if toolResult.IsError != true {
				t.Errorf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to run pod, missing argument image" {
				t.Errorf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		podsRunNilNamespace, err := c.callTool("pods_run", map[string]interface{}{"image": "nginx"})
		t.Run("pods_run with image and nil namespace runs pod", func(t *testing.T) {
			if err != nil {
				t.Errorf("call tool failed %v", err)
				return
			}
			if podsRunNilNamespace.IsError {
				t.Errorf("call tool failed")
				return
			}
		})
		var decodedNilNamespace []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(podsRunNilNamespace.Content[0].(mcp.TextContent).Text), &decodedNilNamespace)
		t.Run("pods_run with image and nil namespace has yaml content", func(t *testing.T) {
			if err != nil {
				t.Errorf("invalid tool result content %v", err)
				return
			}
		})
		t.Run("pods_run with image and nil namespace returns 1 item (Pod)", func(t *testing.T) {
			if len(decodedNilNamespace) != 1 {
				t.Errorf("invalid pods count, expected 1, got %v", len(decodedNilNamespace))
				return
			}
			if decodedNilNamespace[0].GetKind() != "Pod" {
				t.Errorf("invalid pod kind, expected Pod, got %v", decodedNilNamespace[0].GetKind())
				return
			}
		})
		t.Run("pods_run with image and nil namespace returns pod in default", func(t *testing.T) {
			if decodedNilNamespace[0].GetNamespace() != "default" {
				t.Errorf("invalid pod namespace, expected default, got %v", decodedNilNamespace[0].GetNamespace())
				return
			}
		})
		t.Run("pods_run with image and nil namespace returns pod with random name", func(t *testing.T) {
			if !strings.HasPrefix(decodedNilNamespace[0].GetName(), "kubernetes-mcp-server-run-") {
				t.Errorf("invalid pod name, expected random, got %v", decodedNilNamespace[0].GetName())
				return
			}
		})
		t.Run("pods_run with image and nil namespace returns pod with labels", func(t *testing.T) {
			labels := decodedNilNamespace[0].Object["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
			if labels["app.kubernetes.io/name"] == "" {
				t.Errorf("invalid labels, expected app.kubernetes.io/name, got %v", labels)
				return
			}
			if labels["app.kubernetes.io/component"] == "" {
				t.Errorf("invalid labels, expected app.kubernetes.io/component, got %v", labels)
				return
			}
			if labels["app.kubernetes.io/managed-by"] != "kubernetes-mcp-server" {
				t.Errorf("invalid labels, expected app.kubernetes.io/managed-by, got %v", labels)
				return
			}
			if labels["app.kubernetes.io/part-of"] != "kubernetes-mcp-server-run-sandbox" {
				t.Errorf("invalid labels, expected app.kubernetes.io/part-of, got %v", labels)
				return
			}
		})
		t.Run("pods_run with image and nil namespace returns pod with nginx container", func(t *testing.T) {
			containers := decodedNilNamespace[0].Object["spec"].(map[string]interface{})["containers"].([]interface{})
			if containers[0].(map[string]interface{})["image"] != "nginx" {
				t.Errorf("invalid container name, expected nginx, got %v", containers[0].(map[string]interface{})["image"])
				return
			}
		})

		podsRunNamespaceAndPort, err := c.callTool("pods_run", map[string]interface{}{"image": "nginx", "port": 80})
		t.Run("pods_run with image, namespace, and port runs pod", func(t *testing.T) {
			if err != nil {
				t.Errorf("call tool failed %v", err)
				return
			}
			if podsRunNamespaceAndPort.IsError {
				t.Errorf("call tool failed")
				return
			}
		})
		var decodedNamespaceAndPort []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(podsRunNamespaceAndPort.Content[0].(mcp.TextContent).Text), &decodedNamespaceAndPort)
		t.Run("pods_run with image, namespace, and port has yaml content", func(t *testing.T) {
			if err != nil {
				t.Errorf("invalid tool result content %v", err)
				return
			}
		})
		t.Run("pods_run with image, namespace, and port returns 2 items (Pod + Service)", func(t *testing.T) {
			if len(decodedNamespaceAndPort) != 2 {
				t.Errorf("invalid pods count, expected 2, got %v", len(decodedNamespaceAndPort))
				return
			}
			if decodedNamespaceAndPort[0].GetKind() != "Pod" {
				t.Errorf("invalid pod kind, expected Pod, got %v", decodedNamespaceAndPort[0].GetKind())
				return
			}
			if decodedNamespaceAndPort[1].GetKind() != "Service" {
				t.Errorf("invalid service kind, expected Service, got %v", decodedNamespaceAndPort[1].GetKind())
				return
			}
		})
		t.Run("pods_run with image, namespace, and port returns pod with port", func(t *testing.T) {
			containers := decodedNamespaceAndPort[0].Object["spec"].(map[string]interface{})["containers"].([]interface{})
			ports := containers[0].(map[string]interface{})["ports"].([]interface{})
			if ports[0].(map[string]interface{})["containerPort"] != int64(80) {
				t.Errorf("invalid container port, expected 80, got %v", ports[0].(map[string]interface{})["containerPort"])
				return
			}
		})
		t.Run("pods_run with image, namespace, and port returns service with port and selector", func(t *testing.T) {
			ports := decodedNamespaceAndPort[1].Object["spec"].(map[string]interface{})["ports"].([]interface{})
			if ports[0].(map[string]interface{})["port"] != int64(80) {
				t.Errorf("invalid service port, expected 80, got %v", ports[0].(map[string]interface{})["port"])
				return
			}
			if ports[0].(map[string]interface{})["targetPort"] != int64(80) {
				t.Errorf("invalid service target port, expected 80, got %v", ports[0].(map[string]interface{})["targetPort"])
				return
			}
			selector := decodedNamespaceAndPort[1].Object["spec"].(map[string]interface{})["selector"].(map[string]interface{})
			if selector["app.kubernetes.io/name"] == "" {
				t.Errorf("invalid service selector, expected app.kubernetes.io/name, got %v", selector)
				return
			}
			if selector["app.kubernetes.io/managed-by"] != "kubernetes-mcp-server" {
				t.Errorf("invalid service selector, expected app.kubernetes.io/managed-by, got %v", selector)
				return
			}
			if selector["app.kubernetes.io/part-of"] != "kubernetes-mcp-server-run-sandbox" {
				t.Errorf("invalid service selector, expected app.kubernetes.io/part-of, got %v", selector)
				return
			}
		})
	})
}

func TestPodsRunDenied(t *testing.T) {
	deniedResourcesServer := test.Must(config.ReadToml([]byte(`
		denied_resources = [ { version = "v1", kind = "Pod" } ]
	`)))
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		c.withEnvTest()
		podsRun, _ := c.callTool("pods_run", map[string]interface{}{"image": "nginx"})
		t.Run("pods_run has error", func(t *testing.T) {
			if !podsRun.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("pods_run describes denial", func(t *testing.T) {
			expectedMessage := "failed to run pod  in namespace : resource not allowed: /v1, Kind=Pod"
			if podsRun.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, podsRun.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}

func TestPodsRunInOpenShift(t *testing.T) {
	testCaseWithContext(t, &mcpContext{before: inOpenShift, after: inOpenShiftClear}, func(c *mcpContext) {
		t.Run("pods_run with image, namespace, and port returns route with port", func(t *testing.T) {
			podsRunInOpenShift, err := c.callTool("pods_run", map[string]interface{}{"image": "nginx", "port": 80})
			if err != nil {
				t.Errorf("call tool failed %v", err)
				return
			}
			if podsRunInOpenShift.IsError {
				t.Errorf("call tool failed")
				return
			}
			var decodedPodServiceRoute []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(podsRunInOpenShift.Content[0].(mcp.TextContent).Text), &decodedPodServiceRoute)
			if err != nil {
				t.Errorf("invalid tool result content %v", err)
				return
			}
			if len(decodedPodServiceRoute) != 3 {
				t.Errorf("invalid pods count, expected 3, got %v", len(decodedPodServiceRoute))
				return
			}
			if decodedPodServiceRoute[2].GetKind() != "Route" {
				t.Errorf("invalid route kind, expected Route, got %v", decodedPodServiceRoute[2].GetKind())
				return
			}
			targetPort := decodedPodServiceRoute[2].Object["spec"].(map[string]interface{})["port"].(map[string]interface{})["targetPort"].(int64)
			if targetPort != 80 {
				t.Errorf("invalid route target port, expected 80, got %v", targetPort)
				return
			}
		})
	})
}
