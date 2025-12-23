package mcp

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
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
			labels := test.FieldValue(&decodedNilNamespace[0], "metadata.labels").(map[string]interface{})
			s.Contains(labels["app.kubernetes.io/name"], "kubernetes-mcp-server-run-", "invalid labels, expected app.kubernetes.io/name")
			s.Contains(labels["app.kubernetes.io/component"], "kubernetes-mcp-server-run-", "invalid labels, expected app.kubernetes.io/component")
			s.Equal("kubernetes-mcp-server", labels["app.kubernetes.io/managed-by"], "invalid labels, expected app.kubernetes.io/managed-by")
			s.Equal("kubernetes-mcp-server-run-sandbox", labels["app.kubernetes.io/part-of"], "invalid labels, expected app.kubernetes.io/part-of")
		})
		s.Run("returns pod with nginx container", func() {
			s.Equal("nginx", test.FieldString(&decodedNilNamespace[0], "spec.containers[0].image"), "invalid container image, expected nginx")
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
			pod := &decodedNamespaceAndPort[0]
			s.Equal(int64(80), test.FieldInt(pod, "spec.containers[0].ports[0].containerPort"), "invalid container port, expected 80")
		})
		s.Run("returns service with port and selector", func() {
			svc := &decodedNamespaceAndPort[1]
			s.Equal(int64(80), test.FieldInt(svc, "spec.ports[0].port"), "invalid service port, expected 80")
			s.Equal(int64(80), test.FieldInt(svc, "spec.ports[0].targetPort"), "invalid service target port, expected 80")
			selectorMap := test.FieldValue(svc, "spec.selector").(map[string]interface{})
			s.NotEqual("", selectorMap["app.kubernetes.io/name"], "invalid service selector, expected app.kubernetes.io/name")
			s.Equal("kubernetes-mcp-server", selectorMap["app.kubernetes.io/managed-by"], "invalid service selector, expected app.kubernetes.io/managed-by")
			s.Equal("kubernetes-mcp-server-run-sandbox", selectorMap["app.kubernetes.io/part-of"], "invalid service selector, expected app.kubernetes.io/part-of")
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
			msg := podsRun.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to run pod  in namespace :(.+:)? resource not allowed: /v1, Kind=Pod"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *PodsRunSuite) TestPodsRunInOpenShift() {
	s.Require().NoError(EnvTestInOpenShift(s.T().Context()), "Expected to configure test for OpenShift")
	s.T().Cleanup(func() {
		s.Require().NoError(EnvTestInOpenShiftClear(s.T().Context()), "Expected to clear OpenShift test configuration")
	})
	s.InitMcpClient()

	s.Run("pods_run(image=nginx, namespace=nil, port=80) returns route with port", func() {
		podsRunInOpenShift, err := s.CallTool("pods_run", map[string]interface{}{"image": "nginx", "port": 80})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsRunInOpenShift.IsError, "call tool failed")
		})
		var decodedPodServiceRoute []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(podsRunInOpenShift.Content[0].(mcp.TextContent).Text), &decodedPodServiceRoute)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns 3 items (Pod + Service + Route)", func() {
			s.Lenf(decodedPodServiceRoute, 3, "invalid pods count, expected 3, got %v", len(decodedPodServiceRoute))
			s.Equalf("Pod", decodedPodServiceRoute[0].GetKind(), "invalid pod kind, expected Pod, got %v", decodedPodServiceRoute[0].GetKind())
			s.Equalf("Service", decodedPodServiceRoute[1].GetKind(), "invalid service kind, expected Service, got %v", decodedPodServiceRoute[1].GetKind())
			s.Equalf("Route", decodedPodServiceRoute[2].GetKind(), "invalid route kind, expected Route, got %v", decodedPodServiceRoute[2].GetKind())
		})
		s.Run("returns route with port", func() {
			route := &decodedPodServiceRoute[2]
			s.Equal(int64(80), test.FieldInt(route, "spec.port.targetPort"), "invalid route target port, expected 80")
		})
	})
}

func TestPodsRun(t *testing.T) {
	suite.Run(t, new(PodsRunSuite))
}
