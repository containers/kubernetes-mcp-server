package code

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/suite"
)

// EvaluatorParsingSuite tests the full JavaScript → Go client → HTTP chain.
// It uses MockServer to capture actual HTTP request bodies and verify that
// JavaScript objects are correctly converted when sent to the Kubernetes API.
type EvaluatorParsingSuite struct {
	BaseEvaluatorSuite
	*Evaluator
	mu           sync.Mutex
	capturedBody map[string]interface{}
}

func (s *EvaluatorParsingSuite) SetupTest() {
	s.BaseEvaluatorSuite.SetupTest()
	s.capturedBody = nil

	// Reset handlers
	s.mockServer.ResetHandlers()

	// Add capture handler that intercepts mutation requests to resource paths
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only capture POST/PUT/PATCH to resource paths
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
			return
		}

		// Check if this is a resource path (pods, configmaps, etc.)
		isResourcePath := strings.Contains(r.URL.Path, "/pods") ||
			strings.Contains(r.URL.Path, "/configmaps") ||
			strings.Contains(r.URL.Path, "/services") ||
			strings.Contains(r.URL.Path, "/deployments") ||
			strings.Contains(r.URL.Path, "/secrets")

		if !isResourcePath {
			return
		}

		body, err := io.ReadAll(r.Body)
		if err == nil && len(body) > 0 {
			s.mu.Lock()
			_ = json.Unmarshal(body, &s.capturedBody)
			s.mu.Unlock()
		}

		// Return success response with the body echoed back
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))

	// Add discovery handler
	s.mockServer.Handle(test.NewDiscoveryClientHandler())

	var err error
	s.Evaluator, err = NewEvaluator(context.Background(), s.kubernetes)
	s.Require().NoError(err)
}

// getCapturedBody returns the captured body in a thread-safe way
func (s *EvaluatorParsingSuite) getCapturedBody() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.capturedBody
}

// resetCapturedBody clears the captured body before each subtest
func (s *EvaluatorParsingSuite) resetCapturedBody() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capturedBody = nil
}

func (s *EvaluatorParsingSuite) TestMetadataStructure() {
	s.Run("simple pod creation preserves metadata structure", func() {
		s.resetCapturedBody()
		_, err := s.Evaluate(`
			k8s.coreV1().pods("default").create(ctx, {
				apiVersion: "v1",
				kind: "Pod",
				metadata: {
					name: "test-pod"
				},
				spec: {
					containers: [{name: "nginx", image: "nginx"}]
				}
			}, {});
		`, time.Second)

		// The call may fail due to mock limitations, but the body should be captured
		_ = err
		body := s.getCapturedBody()
		s.Require().NotNil(body, "HTTP request body should be captured")

		// Metadata should be preserved in standard Kubernetes structure
		metadata, ok := body["metadata"].(map[string]interface{})
		s.True(ok, "metadata should be a map")
		s.Equal("test-pod", metadata["name"])
	})

	s.Run("metadata with labels is preserved correctly", func() {
		s.resetCapturedBody()
		_, _ = s.Evaluate(`
			k8s.coreV1().pods("default").create(ctx, {
				metadata: {
					name: "labeled-pod",
					labels: {
						"app": "nginx",
						"tier": "frontend"
					}
				},
				spec: {
					containers: [{name: "nginx", image: "nginx"}]
				}
			}, {});
		`, time.Second)

		body := s.getCapturedBody()
		s.Require().NotNil(body)

		metadata, ok := body["metadata"].(map[string]interface{})
		s.True(ok, "metadata should be a map")
		s.Equal("labeled-pod", metadata["name"])

		labels, ok := metadata["labels"].(map[string]interface{})
		s.True(ok, "labels should be a map")
		s.Equal("nginx", labels["app"])
		s.Equal("frontend", labels["tier"])
	})

	s.Run("metadata with annotations is preserved", func() {
		s.resetCapturedBody()
		_, _ = s.Evaluate(`
			k8s.coreV1().pods("default").create(ctx, {
				metadata: {
					name: "annotated-pod",
					annotations: {
						"description": "Test pod",
						"owner": "team-a"
					}
				},
				spec: {
					containers: [{name: "nginx", image: "nginx"}]
				}
			}, {});
		`, time.Second)

		body := s.getCapturedBody()
		s.Require().NotNil(body)

		metadata, ok := body["metadata"].(map[string]interface{})
		s.True(ok, "metadata should be a map")

		annotations, ok := metadata["annotations"].(map[string]interface{})
		s.True(ok)
		s.Equal("Test pod", annotations["description"])
	})
}

func (s *EvaluatorParsingSuite) TestComplexStructures() {
	s.Run("complex spec is preserved", func() {
		s.resetCapturedBody()
		_, err := s.Evaluate(`
			k8s.coreV1().pods("default").create(ctx, {
				metadata: {
					name: "complex-pod"
				},
				spec: {
					containers: [{
						name: "app",
						image: "myapp:latest",
						ports: [{containerPort: 8080}],
						env: [
							{name: "DB_HOST", value: "localhost"},
							{name: "DB_PORT", value: "5432"}
						]
					}],
					volumes: [{
						name: "config",
						configMap: {name: "app-config"}
					}]
				}
			}, {});
		`, time.Second)
		body := s.getCapturedBody()
		if body == nil {
			s.T().Logf("Script evaluation error (if any): %v", err)
		}
		s.Require().NotNil(body, "HTTP request body should be captured")

		// Metadata should be preserved
		metadata, ok := body["metadata"].(map[string]interface{})
		s.True(ok, "metadata should exist")
		s.Equal("complex-pod", metadata["name"])

		// Spec should be preserved as-is
		spec, ok := body["spec"].(map[string]interface{})
		s.True(ok, "spec should exist")

		containers, ok := spec["containers"].([]interface{})
		s.True(ok)
		s.Len(containers, 1)

		container := containers[0].(map[string]interface{})
		s.Equal("app", container["name"])
		s.Equal("myapp:latest", container["image"])

		// Check env vars are preserved
		env := container["env"].([]interface{})
		s.Len(env, 2)
		envVar := env[0].(map[string]interface{})
		s.Equal("DB_HOST", envVar["name"])
		s.Equal("localhost", envVar["value"])
	})

	s.Run("multiple containers are preserved", func() {
		s.resetCapturedBody()
		_, _ = s.Evaluate(`
			k8s.coreV1().pods("default").create(ctx, {
				metadata: {name: "multi-container"},
				spec: {
					containers: [
						{name: "main", image: "main:v1"},
						{name: "sidecar", image: "sidecar:v1"}
					]
				}
			}, {});
		`, time.Second)

		body := s.getCapturedBody()
		s.Require().NotNil(body)

		spec := body["spec"].(map[string]interface{})
		containers := spec["containers"].([]interface{})
		s.Len(containers, 2)
	})
}

func (s *EvaluatorParsingSuite) TestTypePreservation() {
	s.Run("numeric values are preserved", func() {
		s.resetCapturedBody()
		_, _ = s.Evaluate(`
			k8s.coreV1().pods("default").create(ctx, {
				metadata: {name: "numeric-pod"},
				spec: {
					containers: [{
						name: "app",
						image: "app",
						ports: [{containerPort: 8080, hostPort: 80}]
					}],
					terminationGracePeriodSeconds: 30
				}
			}, {});
		`, time.Second)

		body := s.getCapturedBody()
		s.Require().NotNil(body)

		spec := body["spec"].(map[string]interface{})
		// JSON numbers are float64 in Go
		s.Equal(float64(30), spec["terminationGracePeriodSeconds"])

		containers := spec["containers"].([]interface{})
		container := containers[0].(map[string]interface{})
		ports := container["ports"].([]interface{})
		port := ports[0].(map[string]interface{})
		s.Equal(float64(8080), port["containerPort"])
	})

	s.Run("boolean values are preserved", func() {
		s.resetCapturedBody()
		_, _ = s.Evaluate(`
			k8s.coreV1().pods("default").create(ctx, {
				metadata: {name: "bool-pod"},
				spec: {
					containers: [{
						name: "app",
						image: "app",
						stdin: true,
						tty: true
					}],
					hostNetwork: true
				}
			}, {});
		`, time.Second)

		body := s.getCapturedBody()
		s.Require().NotNil(body)

		spec := body["spec"].(map[string]interface{})
		s.Equal(true, spec["hostNetwork"])

		containers := spec["containers"].([]interface{})
		container := containers[0].(map[string]interface{})
		s.Equal(true, container["stdin"])
		s.Equal(true, container["tty"])
	})

	s.Run("special characters in strings are preserved", func() {
		s.resetCapturedBody()
		_, _ = s.Evaluate(`
			k8s.coreV1().pods("default").create(ctx, {
				metadata: {
					name: "special-chars",
					labels: {
						"app.kubernetes.io/name": "my-app"
					},
					annotations: {
						"note": "Contains special chars: !@#$%^&*()"
					}
				},
				spec: {
					containers: [{name: "app", image: "app"}]
				}
			}, {});
		`, time.Second)

		body := s.getCapturedBody()
		s.Require().NotNil(body)

		metadata := body["metadata"].(map[string]interface{})
		labels := metadata["labels"].(map[string]interface{})
		s.Equal("my-app", labels["app.kubernetes.io/name"])

		annotations := metadata["annotations"].(map[string]interface{})
		s.Contains(annotations["note"], "!@#$%^&*()")
	})
}

func (s *EvaluatorParsingSuite) TestMethodCaseInsensitivity() {
	s.Run("uppercase CoreV1 and Create work", func() {
		s.resetCapturedBody()
		_, _ = s.Evaluate(`
			k8s.CoreV1().Pods("default").Create(ctx, {
				metadata: {name: "uppercase-methods"},
				spec: {containers: [{name: "app", image: "app"}]}
			}, {});
		`, time.Second)

		body := s.getCapturedBody()
		s.Require().NotNil(body)

		metadata, ok := body["metadata"].(map[string]interface{})
		s.True(ok, "metadata should be a map")
		s.Equal("uppercase-methods", metadata["name"])
	})

	s.Run("mixed case methods work", func() {
		s.resetCapturedBody()
		_, _ = s.Evaluate(`
			k8s.coreV1().Pods("default").create(ctx, {
				metadata: {name: "mixed-case"},
				spec: {containers: [{name: "app", image: "app"}]}
			}, {});
		`, time.Second)

		body := s.getCapturedBody()
		s.Require().NotNil(body)

		metadata, ok := body["metadata"].(map[string]interface{})
		s.True(ok, "metadata should be a map")
		s.Equal("mixed-case", metadata["name"])
	})
}

func TestEvaluatorParsing(t *testing.T) {
	suite.Run(t, new(EvaluatorParsingSuite))
}
