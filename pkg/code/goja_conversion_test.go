package code

import (
	"encoding/json"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GojaConversionSuite investigates how Goja converts JavaScript objects to Go types.
// This helps understand the root cause of metadata handling issues.
type GojaConversionSuite struct {
	suite.Suite
	vm *goja.Runtime
}

func (s *GojaConversionSuite) SetupTest() {
	s.vm = goja.New()
	s.vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
}

// Test basic struct conversion with simple fields
func (s *GojaConversionSuite) TestBasicStructConversion() {
	// A simple struct with json tags
	type SimpleStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	var received SimpleStruct
	_ = s.vm.Set("captureStruct", func(obj SimpleStruct) {
		received = obj
	})

	s.Run("simple object converts correctly", func() {
		_, err := s.vm.RunString(`captureStruct({name: "test", value: 42})`)
		s.NoError(err)
		s.Equal("test", received.Name)
		s.Equal(42, received.Value)
	})
}

// Test nested struct conversion
func (s *GojaConversionSuite) TestNestedStructConversion() {
	type Inner struct {
		Name string `json:"name"`
	}
	type Outer struct {
		Inner Inner `json:"inner"`
		Value int   `json:"value"`
	}

	var received Outer
	_ = s.vm.Set("captureNested", func(obj Outer) {
		received = obj
	})

	s.Run("nested object converts correctly", func() {
		_, err := s.vm.RunString(`captureNested({inner: {name: "nested"}, value: 99})`)
		s.NoError(err)
		s.T().Logf("Received: %+v", received)
		s.Equal("nested", received.Inner.Name)
		s.Equal(99, received.Value)
	})
}

// Test embedded struct conversion (like ObjectMeta in Pod)
func (s *GojaConversionSuite) TestEmbeddedStructConversion() {
	type EmbeddedMeta struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
	}
	type ResourceWithMeta struct {
		EmbeddedMeta `json:"metadata"`
		Spec         string `json:"spec"`
	}

	var received ResourceWithMeta
	_ = s.vm.Set("captureEmbedded", func(obj ResourceWithMeta) {
		received = obj
	})

	// BUG DOCUMENTATION: This test documents a Goja bug where embedded structs
	// with json tags (like `json:"metadata"`) don't map nested JS objects correctly.
	// Goja's ExportTo checks field.Anonymous and passes the entire object, ignoring
	// the mapped field name from FieldNameMapper. This means {metadata: {name: "x"}}
	// doesn't work - the metadata property is never looked up.
	s.Run("BUG: embedded struct with json tag fails to map nested object", func() {
		_, err := s.vm.RunString(`captureEmbedded({metadata: {name: "test-name", labels: {app: "nginx"}}, spec: "test-spec"})`)
		s.NoError(err)
		s.T().Logf("Received: %+v", received)
		// BUG: Name should be "test-name" but is empty because Goja doesn't look up "metadata"
		s.Equal("", received.Name, "Goja bug: embedded struct with json tag doesn't map nested object")
		s.Equal("test-spec", received.Spec)
		// BUG: Labels should have "nginx" but is empty
		s.Empty(received.Labels, "Goja bug: labels not populated from nested metadata")
	})

	s.Run("flat fields also work for embedded struct", func() {
		received = ResourceWithMeta{} // reset
		_, err := s.vm.RunString(`captureEmbedded({name: "flat-name", spec: "flat-spec"})`)
		s.NoError(err)
		s.T().Logf("Received with flat fields: %+v", received)
		s.Equal("flat-name", received.Name)
		s.Equal("flat-spec", received.Spec)
	})
}

// Test actual Kubernetes Pod struct - documents the Goja bug and workaround
func (s *GojaConversionSuite) TestKubernetesPodConversion() {
	var received *v1.Pod
	_ = s.vm.Set("capturePod", func(pod *v1.Pod) {
		received = pod
	})

	// BUG DOCUMENTATION: This test documents the Goja bug with Kubernetes Pods.
	// v1.Pod embeds metav1.ObjectMeta with `json:"metadata"`, which triggers the bug.
	s.Run("BUG: pod with nested metadata fails to populate ObjectMeta", func() {
		received = nil
		_, err := s.vm.RunString(`
			capturePod({
				apiVersion: "v1",
				kind: "Pod",
				metadata: {
					name: "test-pod",
					namespace: "default",
					labels: {"app": "nginx"}
				},
				spec: {
					containers: [{name: "nginx", image: "nginx:latest"}]
				}
			})
		`)
		s.NoError(err)
		s.T().Logf("Pod with nested metadata: %+v", received)
		if received != nil {
			s.T().Logf("  ObjectMeta: %+v", received.ObjectMeta)
			s.T().Logf("  Spec: %+v", received.Spec)
		}
		s.Require().NotNil(received)
		// BUG: Name and Namespace should be populated but are empty
		s.Equal("", received.Name, "Goja bug: metadata.name not mapped to ObjectMeta.Name")
		s.Equal("", received.Namespace, "Goja bug: metadata.namespace not mapped")
	})

	// WORKAROUND: Flat fields work because Goja maps them to embedded struct fields
	s.Run("WORKAROUND: pod with flat metadata fields works correctly", func() {
		received = nil
		_, err := s.vm.RunString(`
			capturePod({
				apiVersion: "v1",
				kind: "Pod",
				name: "flat-pod",
				namespace: "flat-ns",
				spec: {
					containers: [{name: "nginx", image: "nginx:latest"}]
				}
			})
		`)
		s.NoError(err)
		s.T().Logf("Pod with flat metadata: %+v", received)
		if received != nil {
			s.T().Logf("  ObjectMeta: %+v", received.ObjectMeta)
		}
		s.Require().NotNil(received)
		// This works because flat fields are mapped to the embedded ObjectMeta
		s.Equal("flat-pod", received.Name)
		s.Equal("flat-ns", received.Namespace)
	})
}

// Test ObjectMeta directly
func (s *GojaConversionSuite) TestObjectMetaConversion() {
	var received metav1.ObjectMeta
	_ = s.vm.Set("captureObjectMeta", func(meta metav1.ObjectMeta) {
		received = meta
	})

	s.Run("ObjectMeta with direct fields", func() {
		_, err := s.vm.RunString(`
			captureObjectMeta({
				name: "test-name",
				namespace: "test-ns",
				labels: {"app": "test"}
			})
		`)
		s.NoError(err)
		s.T().Logf("ObjectMeta received: %+v", received)
		s.Equal("test-name", received.Name)
		s.Equal("test-ns", received.Namespace)
		s.Equal("test", received.Labels["app"])
	})
}

// Test what happens when we pass map[string]interface{} vs JS object
func (s *GojaConversionSuite) TestMapVsJSObject() {
	var receivedPod *v1.Pod
	_ = s.vm.Set("capturePodFromMap", func(pod *v1.Pod) {
		receivedPod = pod
	})

	// Create a Go map and pass it through JSON round-trip
	_ = s.vm.Set("createFromGoMap", func() map[string]interface{} {
		return map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "map-pod",
				"namespace": "map-ns",
			},
			"spec": map[string]interface{}{
				"containers": []map[string]interface{}{
					{"name": "nginx", "image": "nginx"},
				},
			},
		}
	})

	s.Run("Go map converted to JS then to Pod", func() {
		receivedPod = nil
		_, err := s.vm.RunString(`
			const goMap = createFromGoMap();
			capturePodFromMap(goMap);
		`)
		s.NoError(err)
		s.T().Logf("Pod from Go map: %+v", receivedPod)
		if receivedPod != nil {
			s.T().Logf("  Name: %s, Namespace: %s", receivedPod.Name, receivedPod.Namespace)
		}
	})

	s.Run("JSON parse/stringify then to Pod", func() {
		receivedPod = nil
		_, err := s.vm.RunString(`
			const obj = {
				apiVersion: "v1",
				kind: "Pod",
				metadata: {name: "json-pod", namespace: "json-ns"},
				spec: {containers: [{name: "nginx", image: "nginx"}]}
			};
			const jsonCopy = JSON.parse(JSON.stringify(obj));
			capturePodFromMap(jsonCopy);
		`)
		s.NoError(err)
		s.T().Logf("Pod from JSON round-trip: %+v", receivedPod)
		if receivedPod != nil {
			s.T().Logf("  Name: %s, Namespace: %s", receivedPod.Name, receivedPod.Namespace)
		}
	})
}

// Test what the actual client interface looks like - documents why wrapping is needed
func (s *GojaConversionSuite) TestClientInterfaceSimulation() {
	// Simulate what CoreV1().Pods().Create() does
	type MockCreateOptions struct {
		DryRun []string `json:"dryRun,omitempty"`
	}

	createdPod := &v1.Pod{}

	mockCreate := func(pod *v1.Pod, opts MockCreateOptions) *v1.Pod {
		createdPod = pod
		// Serialize to JSON to see what we actually receive
		jsonBytes, _ := json.MarshalIndent(pod, "", "  ")
		s.T().Logf("Create received pod JSON:\n%s", string(jsonBytes))
		return pod
	}

	_ = s.vm.Set("mockCreate", mockCreate)

	// BUG DOCUMENTATION: This is why we need wrapper functions that flatten metadata
	s.Run("BUG: direct call fails to populate metadata", func() {
		_, err := s.vm.RunString(`
			mockCreate({
				apiVersion: "v1",
				kind: "Pod",
				metadata: {
					name: "direct-pod",
					namespace: "direct-ns",
					labels: {"app": "test"}
				},
				spec: {
					containers: [{name: "nginx", image: "nginx"}]
				}
			}, {});
		`)
		s.NoError(err)
		s.T().Logf("Created pod Name: %s, Namespace: %s", createdPod.Name, createdPod.Namespace)
		// BUG: Name should be "direct-pod" but is empty due to the embedded struct bug
		s.Equal("", createdPod.Name, "Goja bug: metadata.name not mapped through embedded ObjectMeta")
	})
}

// Test the difference between embedded and named fields
func (s *GojaConversionSuite) TestEmbeddedVsNamed() {
	type Meta struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
	}

	// Named field (not embedded)
	type WithNamedMeta struct {
		Meta Meta   `json:"metadata"`
		Spec string `json:"spec"`
	}

	// Embedded field
	type WithEmbeddedMeta struct {
		Meta `json:"metadata"`
		Spec string `json:"spec"`
	}

	// Embedded without json tag (inline)
	type WithInlineEmbedded struct {
		Meta `json:",inline"`
		Spec string `json:"spec"`
	}

	s.Run("named field converts nested structure correctly", func() {
		var received WithNamedMeta
		_ = s.vm.Set("captureNamed", func(obj WithNamedMeta) {
			received = obj
		})

		_, err := s.vm.RunString(`captureNamed({metadata: {name: "named-test"}, spec: "s"})`)
		s.NoError(err)
		s.T().Logf("Named: %+v", received)
		s.Equal("named-test", received.Meta.Name)
	})

	s.Run("embedded field does NOT convert nested structure", func() {
		var received WithEmbeddedMeta
		_ = s.vm.Set("captureEmbedded", func(obj WithEmbeddedMeta) {
			received = obj
		})

		_, err := s.vm.RunString(`captureEmbedded({metadata: {name: "embedded-test"}, spec: "s"})`)
		s.NoError(err)
		s.T().Logf("Embedded with json tag: %+v", received)
		// This will likely fail - showing the bug
		s.T().Logf("Name is empty: %v", received.Name == "")
	})

	s.Run("embedded with inline converts flat fields", func() {
		var received WithInlineEmbedded
		_ = s.vm.Set("captureInline", func(obj WithInlineEmbedded) {
			received = obj
		})

		_, err := s.vm.RunString(`captureInline({name: "inline-test", spec: "s"})`)
		s.NoError(err)
		s.T().Logf("Inline embedded: %+v", received)
		s.Equal("inline-test", received.Name)
	})
}

// Test if we can work around by using a wrapper
func (s *GojaConversionSuite) TestWorkaroundWithWrapper() {
	type Meta struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
	}

	type PodLike struct {
		Meta `json:"metadata"`
		Spec string `json:"spec"`
	}

	// Wrapper that can receive the JS object as a map and manually convert
	_ = s.vm.Set("manualConvert", func(obj map[string]interface{}) PodLike {
		result := PodLike{}
		if spec, ok := obj["spec"].(string); ok {
			result.Spec = spec
		}
		if metadata, ok := obj["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				result.Name = name
			}
			if labels, ok := metadata["labels"].(map[string]interface{}); ok {
				result.Labels = make(map[string]string)
				for k, v := range labels {
					if s, ok := v.(string); ok {
						result.Labels[k] = s
					}
				}
			}
		}
		return result
	})

	s.Run("manual conversion from map works", func() {
		result, err := s.vm.RunString(`
			const pod = manualConvert({
				metadata: {name: "manual-pod", labels: {app: "test"}},
				spec: "test-spec"
			});
			JSON.stringify(pod);
		`)
		s.NoError(err)
		s.T().Logf("Manual conversion result: %s", result.String())
	})
}

// Test if we can use JSON as intermediary
func (s *GojaConversionSuite) TestJSONIntermediary() {
	type Meta struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
	}

	type PodLike struct {
		Meta `json:"metadata"`
		Spec string `json:"spec"`
	}

	// Receive as string (JSON), unmarshal in Go
	_ = s.vm.Set("createFromJSON", func(jsonStr string) (*PodLike, error) {
		var result PodLike
		err := json.Unmarshal([]byte(jsonStr), &result)
		if err != nil {
			return nil, err
		}
		s.T().Logf("Created from JSON: %+v", result)
		return &result, nil
	})

	s.Run("JSON string to struct works", func() {
		result, err := s.vm.RunString(`
			const pod = {
				metadata: {name: "json-pod", labels: {app: "test"}},
				spec: "test-spec"
			};
			createFromJSON(JSON.stringify(pod));
		`)
		s.NoError(err)
		// The result should be a *PodLike with correct values
		if exported := result.Export(); exported != nil {
			if pod, ok := exported.(*PodLike); ok {
				s.Equal("json-pod", pod.Name)
				s.Equal("test", pod.Labels["app"])
			}
		}
	})
}

// Test the proposed solution: wrap typed functions to use JSON intermediary
func (s *GojaConversionSuite) TestJSONWrapperSolution() {
	var createdPod *v1.Pod

	// Original function that expects *v1.Pod
	originalCreate := func(pod *v1.Pod) *v1.Pod {
		createdPod = pod
		return pod
	}

	// Wrapper that receives JSON string and unmarshals
	wrappedCreate := func(jsonStr string) (*v1.Pod, error) {
		var pod v1.Pod
		if err := json.Unmarshal([]byte(jsonStr), &pod); err != nil {
			return nil, err
		}
		return originalCreate(&pod), nil
	}

	_ = s.vm.Set("createPod", wrappedCreate)

	s.Run("JSON wrapper correctly populates Pod", func() {
		createdPod = nil
		_, err := s.vm.RunString(`
			const pod = {
				apiVersion: "v1",
				kind: "Pod",
				metadata: {
					name: "test-pod",
					namespace: "test-ns",
					labels: {"app": "nginx", "tier": "frontend"}
				},
				spec: {
					containers: [{
						name: "nginx",
						image: "nginx:latest",
						ports: [{containerPort: 80}]
					}]
				}
			};
			createPod(JSON.stringify(pod));
		`)
		s.NoError(err)
		s.Require().NotNil(createdPod)
		s.Equal("test-pod", createdPod.Name)
		s.Equal("test-ns", createdPod.Namespace)
		s.Equal("nginx", createdPod.Labels["app"])
		s.Equal("frontend", createdPod.Labels["tier"])
		s.Len(createdPod.Spec.Containers, 1)
		s.Equal("nginx", createdPod.Spec.Containers[0].Name)
		s.Equal("nginx:latest", createdPod.Spec.Containers[0].Image)

		// Verify JSON output
		jsonBytes, _ := json.MarshalIndent(createdPod, "", "  ")
		s.T().Logf("Created Pod JSON:\n%s", string(jsonBytes))
	})
}

// Test how this could be integrated with the k8s client proxy
func (s *GojaConversionSuite) TestProxyWithJSONSolution() {
	var capturedJSON string

	// Simulate what the real client would do
	mockPodsCreate := func(jsonStr string) (string, error) {
		capturedJSON = jsonStr
		// In real implementation, unmarshal and call actual client
		var pod v1.Pod
		if err := json.Unmarshal([]byte(jsonStr), &pod); err != nil {
			return "", err
		}
		// Return the created resource
		return jsonStr, nil
	}

	_ = s.vm.Set("__podsCreate", mockPodsCreate)

	// Set up a JavaScript wrapper that looks like the typed API
	_, err := s.vm.RunString(`
		const k8s = {
			coreV1: function() {
				return {
					pods: function(namespace) {
						return {
							create: function(ctx, pod, opts) {
								// Convert to JSON and call Go function
								return JSON.parse(__podsCreate(JSON.stringify(pod)));
							}
						};
					}
				};
			}
		};
	`)
	s.Require().NoError(err)

	s.Run("proxy with JSON maintains API compatibility", func() {
		result, err := s.vm.RunString(`
			const pod = k8s.coreV1().pods("default").create(null, {
				apiVersion: "v1",
				kind: "Pod",
				metadata: {
					name: "proxy-pod",
					labels: {"app": "test"}
				},
				spec: {
					containers: [{name: "nginx", image: "nginx"}]
				}
			}, {});
			pod.metadata.name;
		`)
		s.NoError(err)
		s.Equal("proxy-pod", result.Export())

		// Verify the JSON was correct
		var pod v1.Pod
		err = json.Unmarshal([]byte(capturedJSON), &pod)
		s.NoError(err)
		s.Equal("proxy-pod", pod.Name)
		s.Equal("test", pod.Labels["app"])
	})
}

func TestGojaConversion(t *testing.T) {
	suite.Run(t, new(GojaConversionSuite))
}
