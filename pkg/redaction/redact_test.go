package redaction

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type RedactorSuite struct {
	suite.Suite
}

func (s *RedactorSuite) TestSecretDataOpaque() {
	redactor := NewSecretRedactor("opaque")

	s.Run("redacts data and stringData values", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "my-secret",
					"namespace": "default",
				},
				"type": "Opaque",
				"data": map[string]interface{}{
					"DATABASE_PASSWORD": "c2VjcmV0cGFzc3dvcmQ=",
					"API_KEY":           "bXlhcGlrZXk=",
				},
				"stringData": map[string]interface{}{
					"config.yaml": "sensitive: true\npassword: hunter2",
				},
			},
		}

		redactor.Apply(obj)

		data := obj.Object["data"].(map[string]interface{})
		s.Equal("[REDACTED]", data["DATABASE_PASSWORD"])
		s.Equal("[REDACTED]", data["API_KEY"])

		stringData := obj.Object["stringData"].(map[string]interface{})
		s.Equal("[REDACTED]", stringData["config.yaml"])
	})

	s.Run("preserves metadata and type", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "my-secret",
					"namespace": "default",
				},
				"type": "Opaque",
				"data": map[string]interface{}{
					"KEY": "value",
				},
			},
		}

		redactor.Apply(obj)

		metadata := obj.Object["metadata"].(map[string]interface{})
		s.Equal("my-secret", metadata["name"])
		s.Equal("default", metadata["namespace"])
		s.Equal("Opaque", obj.Object["type"])
	})
}

func (s *RedactorSuite) TestSecretDataHashed() {
	redactor := NewSecretRedactor("hashed")

	s.Run("produces hashed redaction markers", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name": "my-secret",
				},
				"data": map[string]interface{}{
					"PASSWORD": "secret123",
					"TOKEN":    "secret123",
				},
			},
		}

		redactor.Apply(obj)

		data := obj.Object["data"].(map[string]interface{})
		passwordVal := data["PASSWORD"].(string)
		tokenVal := data["TOKEN"].(string)

		s.True(strings.HasPrefix(passwordVal, "[REDACTED:gen_"))
		s.True(strings.HasPrefix(tokenVal, "[REDACTED:gen_"))
	})

	s.Run("same input values produce same hash", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name": "my-secret",
				},
				"data": map[string]interface{}{
					"PASSWORD": "secret123",
					"TOKEN":    "secret123",
				},
			},
		}

		redactor.Apply(obj)

		data := obj.Object["data"].(map[string]interface{})
		s.Equal(data["PASSWORD"], data["TOKEN"])
	})
}

func (s *RedactorSuite) TestNonSecretNotRedacted() {
	s.Run("ConfigMap is not redacted", func() {
		redactor := NewSecretRedactor("opaque")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"data": map[string]interface{}{
					"config": "visible-value",
				},
			},
		}

		redactor.Apply(obj)

		data := obj.Object["data"].(map[string]interface{})
		s.Equal("visible-value", data["config"])
	})

	s.Run("Deployment is not redacted", func() {
		redactor := NewSecretRedactor("opaque")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "app"},
			},
		}

		redactor.Apply(obj)
		s.Equal("Deployment", obj.GetKind())
	})
}

func (s *RedactorSuite) TestLastAppliedConfigStripped() {
	s.Run("strips last-applied-configuration annotation", func() {
		redactor := NewSecretRedactor("opaque")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name": "my-secret",
					"annotations": map[string]interface{}{
						"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"v1","data":{"PASSWORD":"c2VjcmV0"},"kind":"Secret"}`,
						"other-annotation": "keep-this",
					},
				},
				"data": map[string]interface{}{
					"PASSWORD": "c2VjcmV0",
				},
			},
		}

		redactor.Apply(obj)

		annotations := obj.GetAnnotations()
		s.NotContains(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		s.Equal("keep-this", annotations["other-annotation"])
	})

	s.Run("removes annotations key when last-applied-configuration is the only annotation", func() {
		redactor := NewSecretRedactor("opaque")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name": "my-secret",
					"annotations": map[string]interface{}{
						"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"KEY":"val"}}`,
					},
				},
				"data": map[string]interface{}{
					"KEY": "val",
				},
			},
		}

		redactor.Apply(obj)

		metadata := obj.Object["metadata"].(map[string]interface{})
		_, hasAnnotations := metadata["annotations"]
		s.False(hasAnnotations, "annotations should be removed entirely when empty")
	})
}

func (s *RedactorSuite) TestVersionAgnosticMatching() {
	s.Run("matches Secret regardless of API version", func() {
		redactor := NewSecretRedactor("opaque")

		// Hypothetical v1beta1 Secret
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1beta1",
				"kind":       "Secret",
				"metadata":   map[string]interface{}{"name": "my-secret"},
				"data": map[string]interface{}{
					"PASSWORD": "plaintext",
				},
			},
		}

		redactor.Apply(obj)

		data := obj.Object["data"].(map[string]interface{})
		s.Equal("[REDACTED]", data["PASSWORD"])
	})
}

func (s *RedactorSuite) TestApplyToList() {
	s.Run("redacts all Secret items in list", func() {
		redactor := NewSecretRedactor("opaque")

		list := &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata":   map[string]interface{}{"name": "secret-1"},
						"data":       map[string]interface{}{"KEY": "value1"},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata":   map[string]interface{}{"name": "secret-2"},
						"data":       map[string]interface{}{"KEY": "value2"},
					},
				},
			},
		}

		redactor.ApplyToList(list)

		for _, item := range list.Items {
			data := item.Object["data"].(map[string]interface{})
			s.Equal("[REDACTED]", data["KEY"])
		}
	})

	s.Run("skips non-Secret items in mixed list", func() {
		redactor := NewSecretRedactor("opaque")

		list := &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata":   map[string]interface{}{"name": "secret-1"},
						"data":       map[string]interface{}{"KEY": "secret-value"},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "config-1"},
						"data":       map[string]interface{}{"KEY": "visible-value"},
					},
				},
			},
		}

		redactor.ApplyToList(list)

		secretData := list.Items[0].Object["data"].(map[string]interface{})
		s.Equal("[REDACTED]", secretData["KEY"])

		configMapData := list.Items[1].Object["data"].(map[string]interface{})
		s.Equal("visible-value", configMapData["KEY"])
	})
}

func (s *RedactorSuite) TestHashedGenerationID() {
	s.Run("hashed value contains generation ID", func() {
		redactor := NewSecretRedactor("hashed")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"data": map[string]interface{}{
					"KEY": "value",
				},
			},
		}

		redactor.Apply(obj)

		data := obj.Object["data"].(map[string]interface{})
		val := data["KEY"].(string)

		genID := redactor.salt.GenerationID()
		s.Contains(val, "gen_"+genID)
	})
}

func (s *RedactorSuite) TestMissingDataField() {
	s.Run("Secret without data field does not panic", func() {
		redactor := NewSecretRedactor("opaque")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]interface{}{"name": "empty-secret"},
			},
		}

		s.NotPanics(func() {
			redactor.Apply(obj)
		})
	})
}

func (s *RedactorSuite) TestSecretWithoutMetadata() {
	s.Run("Secret without metadata key does not panic", func() {
		redactor := NewSecretRedactor("opaque")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"data": map[string]interface{}{
					"KEY": "value",
				},
			},
		}

		s.NotPanics(func() {
			redactor.Apply(obj)
		})

		data := obj.Object["data"].(map[string]interface{})
		s.Equal("[REDACTED]", data["KEY"])
	})
}

func (s *RedactorSuite) TestNonStringDataValues() {
	s.Run("non-string data values are redacted without panic", func() {
		redactor := NewSecretRedactor("opaque")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]interface{}{"name": "weird-secret"},
				"data": map[string]interface{}{
					"string-val":  "normal",
					"numeric-val": 42,
					"bool-val":    true,
					"nil-val":     nil,
				},
			},
		}

		s.NotPanics(func() {
			redactor.Apply(obj)
		})

		data := obj.Object["data"].(map[string]interface{})
		for key, val := range data {
			s.Equal("[REDACTED]", val, "data key %s should be redacted", key)
		}
	})

	s.Run("hashed mode handles non-string values deterministically", func() {
		redactor := NewSecretRedactor("hashed")

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"data": map[string]interface{}{
					"num1": 42,
					"num2": 42,
				},
			},
		}

		redactor.Apply(obj)

		data := obj.Object["data"].(map[string]interface{})
		s.Equal(data["num1"], data["num2"], "same numeric values should produce same hash")
	})
}

func (s *RedactorSuite) TestNilObject() {
	s.Run("nil object does not panic", func() {
		redactor := NewSecretRedactor("opaque")

		s.NotPanics(func() {
			redactor.Apply(nil)
		})
	})
}

func (s *RedactorSuite) TestNilRedactor() {
	s.Run("nil redactor does not panic", func() {
		redactor := NewSecretRedactor("")
		s.Nil(redactor)

		// Calling Apply on nil should not panic
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"data":       map[string]interface{}{"KEY": "value"},
			},
		}

		s.NotPanics(func() {
			var r *Redactor
			r.Apply(obj)
		})
	})
}

func TestRedactor(t *testing.T) {
	suite.Run(t, new(RedactorSuite))
}
