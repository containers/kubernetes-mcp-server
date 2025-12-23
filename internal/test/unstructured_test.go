package test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type UnstructuredSuite struct {
	suite.Suite
}

func (s *UnstructuredSuite) TestFieldString() {
	s.Run("simple field access", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"runStrategy": "Halted",
				},
			},
		}
		s.Run("returns field value", func() {
			s.Equal("Halted", FieldString(obj, "spec.runStrategy"))
		})
	})

	s.Run("nested field access", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"instancetype": map[string]interface{}{
						"kind": "VirtualMachineClusterInstancetype",
						"name": "u1.medium",
					},
				},
			},
		}
		s.Run("returns nested field value", func() {
			s.Equal("VirtualMachineClusterInstancetype", FieldString(obj, "spec.instancetype.kind"))
		})
		s.Run("returns deeply nested field value", func() {
			s.Equal("u1.medium", FieldString(obj, "spec.instancetype.name"))
		})
	})

	s.Run("array indexing", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "vol1",
									"containerDisk": map[string]interface{}{
										"image": "quay.io/containerdisks/fedora:latest",
									},
								},
							},
						},
					},
				},
			},
		}
		s.Run("returns field from first array element", func() {
			s.Equal("vol1", FieldString(obj, "spec.template.spec.volumes[0].name"))
		})
		s.Run("returns nested field from array element", func() {
			s.Equal("quay.io/containerdisks/fedora:latest", FieldString(obj, "spec.template.spec.volumes[0].containerDisk.image"))
		})
	})

	s.Run("non-existent fields", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"runStrategy": "Halted",
				},
			},
		}
		s.Run("returns empty string for non-existent field", func() {
			s.Equal("", FieldString(obj, "spec.nonexistent"))
		})
		s.Run("returns empty string for invalid array index", func() {
			s.Equal("", FieldString(obj, "spec.volumes[999].name"))
		})
	})

	s.Run("edge cases", func() {
		s.Run("returns empty string for empty path", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"runStrategy": "Halted",
					},
				},
			}
			s.Equal("", FieldString(obj, ""))
		})
		s.Run("returns empty string for nil object", func() {
			s.Equal("", FieldString(nil, "spec.runStrategy"))
		})
		s.Run("returns empty string when trying to index non-array", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"runStrategy": "Halted",
					},
				},
			}
			s.Equal("", FieldString(obj, "spec.runStrategy[0]"))
		})
		s.Run("returns empty string when trying to access field on non-map", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							"simple-string",
						},
					},
				},
			}
			s.Equal("", FieldString(obj, "spec.volumes[0].name"))
		})
		s.Run("returns empty string for negative array index", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{"name": "vol1"},
						},
					},
				},
			}
			s.Equal("", FieldString(obj, "spec.volumes[-1].name"))
		})
		s.Run("returns empty string when intermediate field is nil", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": nil,
				},
			}
			// Can't traverse through nil intermediate field
			s.Equal("", FieldString(obj, "spec.foo.bar"))
			// But spec itself exists (with nil value)
			s.True(FieldExists(obj, "spec"))
		})
		s.Run("handles path with consecutive dots gracefully", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"runStrategy": "Halted",
					},
				},
			}
			// Parser skips empty segments, so consecutive dots are ignored
			s.Equal("Halted", FieldString(obj, "spec..runStrategy"))
		})
		s.Run("handles path starting with dot gracefully", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"runStrategy": "Halted",
					},
				},
			}
			// Parser skips empty segments at the start
			s.Equal("Halted", FieldString(obj, ".spec.runStrategy"))
		})
		s.Run("handles path ending with dot", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"runStrategy": "Halted",
					},
				},
			}
			s.Equal("Halted", FieldString(obj, "spec.runStrategy."))
		})
		s.Run("handles dot inside brackets gracefully", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{"name": "vol1"},
						},
					},
				},
			}
			// Parser treats "0.5" as the index string, fmt.Sscanf will parse it as 0
			s.Equal("vol1", FieldString(obj, "spec.volumes[0.5].name"))
		})
		s.Run("returns empty string for non-numeric array index", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{"name": "vol1"},
						},
					},
				},
			}
			// Parser fails to parse "abc" as integer, defaults to -1 which is invalid
			s.Equal("", FieldString(obj, "spec.volumes[abc].name"))
		})
	})
}

func (s *UnstructuredSuite) TestFieldExists() {
	s.Run("existing fields", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"runStrategy": "Halted",
					"instancetype": map[string]interface{}{
						"kind": "VirtualMachineClusterInstancetype",
					},
				},
			},
		}
		s.Run("returns true for simple field", func() {
			s.True(FieldExists(obj, "spec.runStrategy"))
		})
		s.Run("returns true for nested object", func() {
			s.True(FieldExists(obj, "spec.instancetype"))
		})
		s.Run("returns true for nested field", func() {
			s.True(FieldExists(obj, "spec.instancetype.kind"))
		})
	})

	s.Run("non-existent fields", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"runStrategy": "Halted",
					"instancetype": map[string]interface{}{
						"kind": "VirtualMachineClusterInstancetype",
					},
				},
			},
		}
		s.Run("returns false for non-existent simple field", func() {
			s.False(FieldExists(obj, "spec.preference"))
		})
		s.Run("returns false for non-existent nested field", func() {
			s.False(FieldExists(obj, "spec.instancetype.name"))
		})
	})

	s.Run("edge cases", func() {
		s.Run("returns false for empty path", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"runStrategy": "Halted",
					},
				},
			}
			s.False(FieldExists(obj, ""))
		})
		s.Run("returns false for nil object", func() {
			s.False(FieldExists(nil, "spec.runStrategy"))
		})
	})
}

func (s *UnstructuredSuite) TestFieldInt() {
	s.Run("integer field access", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(3),
					"ports": []interface{}{
						map[string]interface{}{
							"containerPort": int64(80),
						},
					},
				},
			},
		}
		s.Run("returns int64 field value", func() {
			s.Equal(int64(3), FieldInt(obj, "spec.replicas"))
		})
		s.Run("returns int64 from array element", func() {
			s.Equal(int64(80), FieldInt(obj, "spec.ports[0].containerPort"))
		})
	})

	s.Run("different integer types", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"int64Value": int64(100),
					"intValue":   int(200),
					"int32Value": int32(300),
				},
			},
		}
		s.Run("handles int64 type", func() {
			s.Equal(int64(100), FieldInt(obj, "spec.int64Value"))
		})
		s.Run("handles int type", func() {
			s.Equal(int64(200), FieldInt(obj, "spec.intValue"))
		})
		s.Run("handles int32 type", func() {
			s.Equal(int64(300), FieldInt(obj, "spec.int32Value"))
		})
	})

	s.Run("edge cases", func() {
		s.Run("returns 0 for non-existent field", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"replicas": int64(3),
					},
				},
			}
			s.Equal(int64(0), FieldInt(obj, "spec.nonexistent"))
		})
		s.Run("returns 0 for nil object", func() {
			s.Equal(int64(0), FieldInt(nil, "spec.replicas"))
		})
		s.Run("returns 0 for non-integer field", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"name": "test-string",
					},
				},
			}
			s.Equal(int64(0), FieldInt(obj, "spec.name"))
		})
	})
}

func (s *UnstructuredSuite) TestFieldValue() {
	s.Run("returns any field value", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "test",
					},
				},
				"spec": map[string]interface{}{
					"replicas": int64(3),
					"name":     "test-name",
				},
			},
		}
		s.Run("returns map value", func() {
			labels := FieldValue(obj, "metadata.labels")
			labelsMap, ok := labels.(map[string]interface{})
			s.True(ok, "expected map[string]interface{}")
			s.Equal("test", labelsMap["app"])
		})
		s.Run("returns int64 value", func() {
			replicas := FieldValue(obj, "spec.replicas")
			s.Equal(int64(3), replicas)
		})
		s.Run("returns string value", func() {
			name := FieldValue(obj, "spec.name")
			s.Equal("test-name", name)
		})
	})

	s.Run("edge cases", func() {
		s.Run("returns nil for non-existent field", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			}
			s.Nil(FieldValue(obj, "spec.nonexistent"))
		})
		s.Run("returns nil for nil object", func() {
			s.Nil(FieldValue(nil, "spec.name"))
		})
	})
}

func TestUnstructured(t *testing.T) {
	suite.Run(t, new(UnstructuredSuite))
}
