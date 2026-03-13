package output

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PruneSuite struct {
	suite.Suite
}

func (s *PruneSuite) TestSingleObject() {
	s.Run("removes managedFields", func() {
		obj := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":          "test-pod",
				"namespace":     "default",
				"managedFields": []interface{}{"field-manager-data"},
			},
		}
		result := PruneForStructuredOutput(obj).(map[string]interface{})
		metadata := result["metadata"].(map[string]interface{})
		s.Equal("test-pod", metadata["name"])
		s.Nil(metadata["managedFields"])
	})

	s.Run("removes last-applied-configuration annotation", func() {
		obj := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod",
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/last-applied-configuration": `{"big":"json"}`,
					"app.kubernetes.io/name":                           "myapp",
				},
			},
		}
		result := PruneForStructuredOutput(obj).(map[string]interface{})
		metadata := result["metadata"].(map[string]interface{})
		annotations := metadata["annotations"].(map[string]interface{})
		s.Nil(annotations["kubectl.kubernetes.io/last-applied-configuration"])
		s.Equal("myapp", annotations["app.kubernetes.io/name"])
	})

	s.Run("removes annotations map when last-applied-configuration was the only annotation", func() {
		obj := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod",
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/last-applied-configuration": `{"big":"json"}`,
				},
			},
		}
		result := PruneForStructuredOutput(obj).(map[string]interface{})
		metadata := result["metadata"].(map[string]interface{})
		s.Nil(metadata["annotations"])
	})

	s.Run("preserves all other fields", func() {
		obj := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "test-pod",
				"namespace": "default",
				"labels":    map[string]interface{}{"app": "nginx"},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "nginx", "image": "nginx:latest"},
				},
			},
			"status": map[string]interface{}{
				"phase": "Running",
			},
		}
		result := PruneForStructuredOutput(obj).(map[string]interface{})
		s.Equal("v1", result["apiVersion"])
		s.Equal("Pod", result["kind"])
		s.Equal("Running", result["status"].(map[string]interface{})["phase"])
		metadata := result["metadata"].(map[string]interface{})
		s.Equal("nginx", metadata["labels"].(map[string]interface{})["app"])
	})
}

func (s *PruneSuite) TestList() {
	s.Run("prunes items in a Kubernetes list", func() {
		list := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PodList",
			"metadata":   map[string]interface{}{},
			"items": []interface{}{
				map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":          "pod-1",
						"managedFields": []interface{}{"data"},
						"annotations": map[string]interface{}{
							"kubectl.kubernetes.io/last-applied-configuration": `{}`,
						},
					},
				},
				map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":          "pod-2",
						"managedFields": []interface{}{"data"},
						"annotations": map[string]interface{}{
							"kubectl.kubernetes.io/last-applied-configuration": `{}`,
							"custom-annotation": "keep-me",
						},
					},
				},
			},
		}
		result := PruneForStructuredOutput(list).(map[string]interface{})
		items := result["items"].([]interface{})
		s.Len(items, 2)

		meta1 := items[0].(map[string]interface{})["metadata"].(map[string]interface{})
		s.Equal("pod-1", meta1["name"])
		s.Nil(meta1["managedFields"])
		s.Nil(meta1["annotations"])

		meta2 := items[1].(map[string]interface{})["metadata"].(map[string]interface{})
		s.Equal("pod-2", meta2["name"])
		s.Nil(meta2["managedFields"])
		s.Equal("keep-me", meta2["annotations"].(map[string]interface{})["custom-annotation"])
	})
}

func (s *PruneSuite) TestSliceOfObjects() {
	s.Run("prunes each object in a slice", func() {
		objects := []map[string]interface{}{
			{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":          "pod-1",
					"managedFields": []interface{}{"data"},
				},
			},
			{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name":          "svc-1",
					"managedFields": []interface{}{"data"},
				},
			},
		}
		result := PruneForStructuredOutput(objects).([]map[string]interface{})
		s.Len(result, 2)
		s.Nil(result[0]["metadata"].(map[string]interface{})["managedFields"])
		s.Nil(result[1]["metadata"].(map[string]interface{})["managedFields"])
	})
}

func (s *PruneSuite) TestNoOpCases() {
	s.Run("handles object without metadata", func() {
		obj := map[string]interface{}{
			"data": "no-metadata-here",
		}
		result := PruneForStructuredOutput(obj)
		s.Equal(obj, result)
	})

	s.Run("handles nil input", func() {
		result := PruneForStructuredOutput(nil)
		s.Nil(result)
	})

	s.Run("handles non-map input", func() {
		result := PruneForStructuredOutput("a string")
		s.Equal("a string", result)
	})

	s.Run("handles object with no fields to prune", func() {
		obj := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "clean-pod",
			},
		}
		result := PruneForStructuredOutput(obj).(map[string]interface{})
		s.Equal("clean-pod", result["metadata"].(map[string]interface{})["name"])
	})
}

func TestPruneSuite(t *testing.T) {
	suite.Run(t, new(PruneSuite))
}
