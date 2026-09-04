package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
)

type RBACSuite struct {
	suite.Suite
}

func (s *RBACSuite) TestRBACNone() {
	s.Equal(&RBACMetadata{
		Version: RBACVersionV1Alpha1,
		None:    &NoRBAC{},
	}, RBACNone())
}

func (s *RBACSuite) TestRBACBounded() {
	requirement := RBACRequirement{
		Verbs:  []string{"get"},
		Target: validResourceTarget(),
	}

	s.Equal(&RBACMetadata{
		Version: RBACVersionV1Alpha1,
		Bounded: &BoundedRBAC{Requirements: []RBACRequirement{requirement}},
	}, RBACBounded(requirement))
}

func (s *RBACSuite) TestRBACUnbounded() {
	s.Equal(&RBACMetadata{
		Version:   RBACVersionV1Alpha1,
		Unbounded: &UnboundedRBAC{Reason: "depends on external content"},
	}, RBACUnbounded("depends on external content"))
}

func (s *RBACSuite) TestValidate() {
	s.Run("accepts no RBAC", func() {
		metadata := &RBACMetadata{
			Version: RBACVersionV1Alpha1,
			None:    &NoRBAC{},
		}

		s.NoError(metadata.Validate())
	})

	s.Run("accepts argument-derived GVK values", func() {
		metadata := validBoundedRBAC()

		s.NoError(metadata.Validate())
	})

	s.Run("accepts unbounded RBAC with a reason", func() {
		metadata := &RBACMetadata{
			Version:   RBACVersionV1Alpha1,
			Unbounded: &UnboundedRBAC{Reason: "depends on chart contents"},
		}

		s.NoError(metadata.Validate())
	})

	tests := []struct {
		name     string
		metadata *RBACMetadata
	}{
		{
			name:     "rejects nil metadata",
			metadata: nil,
		},
		{
			name: "rejects unsupported version",
			metadata: &RBACMetadata{
				Version: "v2",
				None:    &NoRBAC{},
			},
		},
		{
			name: "rejects missing declaration",
			metadata: &RBACMetadata{
				Version: RBACVersionV1Alpha1,
			},
		},
		{
			name: "rejects multiple declarations",
			metadata: &RBACMetadata{
				Version: RBACVersionV1Alpha1,
				None:    &NoRBAC{},
				Bounded: &BoundedRBAC{},
			},
		},
		{
			name: "rejects bounded metadata without requirements",
			metadata: &RBACMetadata{
				Version: RBACVersionV1Alpha1,
				Bounded: &BoundedRBAC{},
			},
		},
		{
			name: "rejects unbounded metadata without a reason",
			metadata: &RBACMetadata{
				Version:   RBACVersionV1Alpha1,
				Unbounded: &UnboundedRBAC{},
			},
		},
		{
			name:     "rejects requirements without verbs",
			metadata: boundedRBACWithRequirement(RBACRequirement{Target: validResourceTarget()}),
		},
		{
			name: "rejects multiple target forms",
			metadata: boundedRBACWithRequirement(RBACRequirement{
				Verbs: []string{"get"},
				Target: RBACTarget{
					Resource: &RBACResourceTarget{Resource: "pods"},
					Manifest: &RBACManifestTarget{Argument: "resource"},
				},
			}),
		},
		{
			name: "rejects resource targets without a resource",
			metadata: boundedRBACWithRequirement(RBACRequirement{
				Verbs:  []string{"get"},
				Target: RBACTarget{Resource: &RBACResourceTarget{}},
			}),
		},
		{
			name: "rejects GVK targets without an API version argument",
			metadata: boundedRBACWithRequirement(RBACRequirement{
				Verbs:  []string{"get"},
				Target: RBACTarget{GVK: &RBACGVKTarget{KindArgument: "kind"}},
			}),
		},
		{
			name: "rejects GVK targets without a kind argument",
			metadata: boundedRBACWithRequirement(RBACRequirement{
				Verbs:  []string{"get"},
				Target: RBACTarget{GVK: &RBACGVKTarget{APIVersionArgument: "apiVersion"}},
			}),
		},
		{
			name: "rejects manifest targets without an argument",
			metadata: boundedRBACWithRequirement(RBACRequirement{
				Verbs:  []string{"patch"},
				Target: RBACTarget{Manifest: &RBACManifestTarget{}},
			}),
		},
		{
			name: "rejects ambiguous namespaces",
			metadata: boundedRBACWithRequirement(RBACRequirement{
				Verbs:     []string{"get"},
				Target:    validResourceTarget(),
				Namespace: &RBACNamespace{Name: "default", Argument: "namespace"},
			}),
		},
		{
			name: "rejects ambiguous resource names",
			metadata: boundedRBACWithRequirement(RBACRequirement{
				Verbs:        []string{"get"},
				Target:       validResourceTarget(),
				ResourceName: &RBACResourceName{Name: "pod", Argument: "name"},
			}),
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Error(tt.metadata.Validate())
		})
	}
}

func (s *RBACSuite) TestJSONSerialization() {
	metadata := validBoundedRBAC()

	serialized, err := json.Marshal(metadata)
	s.Require().NoError(err)
	s.JSONEq(`{
		"version": "v1alpha1",
		"bounded": {
			"requirements": [{
				"verbs": ["get"],
				"target": {
					"gvk": {
						"apiVersionArgument": "apiVersion",
						"kindArgument": "kind"
					}
				},
				"namespace": {
					"argument": "namespace"
				},
				"resourceName": {
					"argument": "name"
				}
			}]
		}
	}`, string(serialized))
}

func validBoundedRBAC() *RBACMetadata {
	return boundedRBACWithRequirement(RBACRequirement{
		Verbs: []string{"get"},
		Target: RBACTarget{GVK: &RBACGVKTarget{
			APIVersionArgument: "apiVersion",
			KindArgument:       "kind",
		}},
		Namespace:    &RBACNamespace{Argument: "namespace"},
		ResourceName: &RBACResourceName{Argument: "name"},
	})
}

func boundedRBACWithRequirement(requirement RBACRequirement) *RBACMetadata {
	return &RBACMetadata{
		Version: RBACVersionV1Alpha1,
		Bounded: &BoundedRBAC{
			Requirements: []RBACRequirement{requirement},
		},
	}
}

func validResourceTarget() RBACTarget {
	return RBACTarget{Resource: &RBACResourceTarget{Resource: "pods"}}
}

func TestRBAC(t *testing.T) {
	suite.Run(t, new(RBACSuite))
}
