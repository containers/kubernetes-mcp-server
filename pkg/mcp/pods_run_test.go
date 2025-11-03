package mcp

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

type PodsRunSuite struct {
	BaseMcpSuite
}

func (s *PodsRunSuite) TestPodsRun() {
	s.InitMcpClient()
	s.Run("pods_run with nil image returns error", func() {
		toolResult, _ := s.CallTool("pods_run", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to run pod, missing argument image", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("pods_run(image=nginx, namespace=nil), uses configured namespace", func() {
		podsRunNilNamespace, err := s.CallTool("pods_run", map[string]interface{}{"image": "nginx"})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsRunNilNamespace.IsError, "call tool failed")
		})
		var decodedNilNamespace []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(podsRunNilNamespace.Content[0].(mcp.TextContent).Text), &decodedNilNamespace)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns 1 item (Pod)", func() {
			s.Lenf(decodedNilNamespace, 1, "invalid pods count, expected 1, got %v", len(decodedNilNamespace))
			s.Equalf("Pod", decodedNilNamespace[0].GetKind(), "invalid pod kind, expected Pod, got %v", decodedNilNamespace[0].GetKind())
		})
		s.Run("returns pod in default", func() {
			s.Equalf("default", decodedNilNamespace[0].GetNamespace(), "invalid pod namespace, expected default, got %v", decodedNilNamespace[0].GetNamespace())
		})
		s.Run("returns pod with random name", func() {
			s.Truef(strings.HasPrefix(decodedNilNamespace[0].GetName(), "kubernetes-mcp-server-run-"),
				"invalid pod name, expected random, got %v", decodedNilNamespace[0].GetName())
		})
		s.Run("returns pod with labels", func() {
			labels := decodedNilNamespace[0].Object["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
			s.NotEqualf("", labels["app.kubernetes.io/name"], "invalid labels, expected app.kubernetes.io/name, got %v", labels)
			s.NotEqualf("", labels["app.kubernetes.io/component"], "invalid labels, expected app.kubernetes.io/component, got %v", labels)
			s.Equalf("kubernetes-mcp-server", labels["app.kubernetes.io/managed-by"], "invalid labels, expected app.kubernetes.io/managed-by, got %v", labels)
			s.Equalf("kubernetes-mcp-server-run-sandbox", labels["app.kubernetes.io/part-of"], "invalid labels, expected app.kubernetes.io/part-of, got %v", labels)
		})
		s.Run("returns pod with nginx container", func() {
			containers := decodedNilNamespace[0].Object["spec"].(map[string]interface{})["containers"].([]interface{})
			s.Equalf("nginx", containers[0].(map[string]interface{})["image"], "invalid container name, expected nginx, got %v", containers[0].(map[string]interface{})["image"])
		})
	})
	s.Run("pods_run(image=nginx, namespace=nil, port=80)", func() {
		podsRunNamespaceAndPort, err := s.CallTool("pods_run", map[string]interface{}{"image": "nginx", "port": 80})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsRunNamespaceAndPort.IsError, "call tool failed")
		})
		var decodedNamespaceAndPort []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(podsRunNamespaceAndPort.Content[0].(mcp.TextContent).Text), &decodedNamespaceAndPort)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns 2 items (Pod + Service)", func() {
			s.Lenf(decodedNamespaceAndPort, 2, "invalid pods count, expected 2, got %v", len(decodedNamespaceAndPort))
			s.Equalf("Pod", decodedNamespaceAndPort[0].GetKind(), "invalid pod kind, expected Pod, got %v", decodedNamespaceAndPort[0].GetKind())
			s.Equalf("Service", decodedNamespaceAndPort[1].GetKind(), "invalid service kind, expected Service, got %v", decodedNamespaceAndPort[1].GetKind())
		})
		s.Run("returns pod with port", func() {
			containers := decodedNamespaceAndPort[0].Object["spec"].(map[string]interface{})["containers"].([]interface{})
			ports := containers[0].(map[string]interface{})["ports"].([]interface{})
			s.Equalf(int64(80), ports[0].(map[string]interface{})["containerPort"], "invalid container port, expected 80, got %v", ports[0].(map[string]interface{})["containerPort"])
		})
		s.Run("returns service with port and selector", func() {
			ports := decodedNamespaceAndPort[1].Object["spec"].(map[string]interface{})["ports"].([]interface{})
			s.Equalf(int64(80), ports[0].(map[string]interface{})["port"], "invalid service port, expected 80, got %v", ports[0].(map[string]interface{})["port"])
			s.Equalf(int64(80), ports[0].(map[string]interface{})["targetPort"], "invalid service target port, expected 80, got %v", ports[0].(map[string]interface{})["targetPort"])
			selector := decodedNamespaceAndPort[1].Object["spec"].(map[string]interface{})["selector"].(map[string]interface{})
			s.NotEqualf("", selector["app.kubernetes.io/name"], "invalid service selector, expected app.kubernetes.io/name, got %v", selector)
			s.Equalf("kubernetes-mcp-server", selector["app.kubernetes.io/managed-by"], "invalid service selector, expected app.kubernetes.io/managed-by, got %v", selector)
			s.Equalf("kubernetes-mcp-server-run-sandbox", selector["app.kubernetes.io/part-of"], "invalid service selector, expected app.kubernetes.io/part-of, got %v", selector)
		})
	})
}

func (s *PodsRunSuite) TestPodsRunDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Pod" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("pods_run (denied)", func() {
		podsRun, err := s.CallTool("pods_run", map[string]interface{}{"image": "nginx"})
		s.Run("has error", func() {
			s.Truef(podsRun.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			expectedMessage := "failed to run pod  in namespace : resource not allowed: /v1, Kind=Pod"
			s.Equalf(expectedMessage, podsRun.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, podsRun.Content[0].(mcp.TextContent).Text)
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

func TestPodsRun(t *testing.T) {
	suite.Run(t, new(PodsRunSuite))
}
