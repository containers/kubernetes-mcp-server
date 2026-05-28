package redaction

import (
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type RedactorSuite struct {
	suite.Suite
}

func (s *RedactorSuite) TestSecretDataOpaque() {
	redactor := NewRedactor([]api.RedactedResource{
		{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
			Fields:  []string{"data.*", "stringData.*"},
			Mode:    "opaque",
		},
	})

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
	redactor := NewRedactor([]api.RedactedResource{
		{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
			Fields:  []string{"data.*"},
			Mode:    "hashed",
		},
	})

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

func (s *RedactorSuite) TestDeploymentEnvValues() {
	redactor := NewRedactor([]api.RedactedResource{
		{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
			Fields:  []string{"spec.template.spec.containers.*.env.*.value"},
			Mode:    "opaque",
		},
	})

	s.Run("redacts plain env values", func() {
		obj := s.deploymentWithEnv()
		redactor.Apply(obj)

		env := s.getContainerEnv(obj, 0)

		env0 := env[0].(map[string]interface{})
		s.Equal("PORT", env0["name"])
		s.Equal("[REDACTED]", env0["value"])

		env1 := env[1].(map[string]interface{})
		s.Equal("NODE_ENV", env1["name"])
		s.Equal("[REDACTED]", env1["value"])
	})

	s.Run("preserves secretKeyRef entries", func() {
		obj := s.deploymentWithEnv()
		redactor.Apply(obj)

		env := s.getContainerEnv(obj, 0)

		env2 := env[2].(map[string]interface{})
		s.Equal("DB_PASSWORD", env2["name"])
		s.Nil(env2["value"], "secretKeyRef env should not have value field added")
		secretRef := env2["valueFrom"].(map[string]interface{})["secretKeyRef"].(map[string]interface{})
		s.Equal("db-secret", secretRef["name"])
		s.Equal("password", secretRef["key"])
	})
}

func (s *RedactorSuite) TestNoMatchingGVK() {
	s.Run("non-matching GVK is not redacted", func() {
		redactor := NewRedactor([]api.RedactedResource{
			{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
				Fields:  []string{"data.*"},
			},
		})

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
}

func (s *RedactorSuite) TestEmptyRedactedFields() {
	s.Run("no fields configured means no redaction", func() {
		redactor := NewRedactor([]api.RedactedResource{
			{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
			},
		})

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"data": map[string]interface{}{
					"PASSWORD": "should-stay",
				},
			},
		}

		redactor.Apply(obj)
		data := obj.Object["data"].(map[string]interface{})
		s.Equal("should-stay", data["PASSWORD"])
	})
}

func (s *RedactorSuite) TestDefaultsToOpaque() {
	s.Run("empty mode defaults to opaque redaction", func() {
		redactor := NewRedactor([]api.RedactedResource{
			{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
				Fields:  []string{"data.*"},
			},
		})

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
		s.Equal("[REDACTED]", data["KEY"])
	})
}

func (s *RedactorSuite) TestApplyToList() {
	s.Run("redacts all items in list", func() {
		redactor := NewRedactor([]api.RedactedResource{
			{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
				Fields:  []string{"data.*"},
			},
		})

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
}

func (s *RedactorSuite) TestInitContainers() {
	s.Run("redacts both init and main container env values", func() {
		redactor := NewRedactor([]api.RedactedResource{
			{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
				Fields: []string{
					"spec.template.spec.containers.*.env.*.value",
					"spec.template.spec.initContainers.*.env.*.value",
				},
			},
		})

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "app"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"initContainers": []interface{}{
								map[string]interface{}{
									"name": "init",
									"env": []interface{}{
										map[string]interface{}{
											"name":  "INIT_SECRET",
											"value": "init-secret-val",
										},
									},
								},
							},
							"containers": []interface{}{
								map[string]interface{}{
									"name": "main",
									"env": []interface{}{
										map[string]interface{}{
											"name":  "APP_SECRET",
											"value": "app-secret-val",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		redactor.Apply(obj)

		spec := obj.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})

		initEnv := spec["initContainers"].([]interface{})[0].(map[string]interface{})["env"].([]interface{})[0].(map[string]interface{})
		s.Equal("INIT_SECRET", initEnv["name"])
		s.Equal("[REDACTED]", initEnv["value"])

		mainEnv := spec["containers"].([]interface{})[0].(map[string]interface{})["env"].([]interface{})[0].(map[string]interface{})
		s.Equal("APP_SECRET", mainEnv["name"])
		s.Equal("[REDACTED]", mainEnv["value"])
	})
}

func (s *RedactorSuite) TestHashedGenerationID() {
	s.Run("hashed value contains generation ID", func() {
		redactor := NewRedactor([]api.RedactedResource{
			{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
				Fields:  []string{"data.*"},
				Mode:    "hashed",
			},
		})

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

func (s *RedactorSuite) TestMissingField() {
	s.Run("non-existent field path does not panic", func() {
		redactor := NewRedactor([]api.RedactedResource{
			{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
				Fields:  []string{"nonexistent.path.*"},
			},
		})

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"data": map[string]interface{}{
					"KEY": "should-remain",
				},
			},
		}

		s.NotPanics(func() {
			redactor.Apply(obj)
		})

		data := obj.Object["data"].(map[string]interface{})
		s.Equal("should-remain", data["KEY"])
	})
}

func (s *RedactorSuite) TestNilObject() {
	s.Run("nil object does not panic", func() {
		redactor := NewRedactor([]api.RedactedResource{
			{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
				Fields:  []string{"data.*"},
			},
		})

		s.NotPanics(func() {
			redactor.Apply(nil)
		})
	})
}

// Helper to create a deployment with env vars for testing
func (s *RedactorSuite) deploymentWithEnv() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "web-app",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "myapp:latest",
								"env": []interface{}{
									map[string]interface{}{
										"name":  "PORT",
										"value": "3000",
									},
									map[string]interface{}{
										"name":  "NODE_ENV",
										"value": "production",
									},
									map[string]interface{}{
										"name": "DB_PASSWORD",
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": "db-secret",
												"key":  "password",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Helper to extract container env from a deployment
func (s *RedactorSuite) getContainerEnv(obj *unstructured.Unstructured, containerIndex int) []interface{} {
	containers := obj.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})
	return containers[containerIndex].(map[string]interface{})["env"].([]interface{})
}

func TestRedactor(t *testing.T) {
	suite.Run(t, new(RedactorSuite))
}
