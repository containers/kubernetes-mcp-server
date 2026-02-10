package kubernetes

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type mockRESTMapper struct {
	mappings map[schema.GroupKind]*meta.RESTMapping
}

func (m *mockRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	if mapping, ok := m.mappings[gk]; ok {
		return mapping, nil
	}
	return nil, &meta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{Group: gk.Group, Resource: gk.Kind}}
}

func (m *mockRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return nil, nil
}

func (m *mockRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

func (m *mockRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, nil
}

func (m *mockRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, nil
}

func (m *mockRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, nil
}

func (m *mockRESTMapper) ResourceSingularizer(resource string) (singular string, err error) {
	return "", nil
}

type ResourceValidatorTestSuite struct {
	suite.Suite
}

func (s *ResourceValidatorTestSuite) TestName() {
	v := NewResourceValidator(nil)
	s.Equal("resource", v.Name())
}

func (s *ResourceValidatorTestSuite) TestValidate() {
	testCases := []struct {
		name        string
		req         *api.HTTPValidationRequest
		mapper      meta.RESTMapper
		expectError bool
		errorCode   api.ValidationErrorCode
	}{
		{
			name:        "nil GVK passes validation",
			req:         &api.HTTPValidationRequest{GVK: nil},
			mapper:      nil,
			expectError: false,
		},
		{
			name: "nil RESTMapper passes validation",
			req: &api.HTTPValidationRequest{
				GVK: &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			},
			mapper:      nil,
			expectError: false,
		},
		{
			name: "existing resource passes validation",
			req: &api.HTTPValidationRequest{
				GVK: &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			},
			mapper: &mockRESTMapper{
				mappings: map[schema.GroupKind]*meta.RESTMapping{
					{Group: "", Kind: "Pod"}: {
						Resource: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "non-existent resource fails validation",
			req: &api.HTTPValidationRequest{
				GVK: &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "FakeResource"},
			},
			mapper: &mockRESTMapper{
				mappings: map[schema.GroupKind]*meta.RESTMapping{},
			},
			expectError: true,
			errorCode:   api.ErrorCodeResourceNotFound,
		},
		{
			name: "apps group resource passes validation",
			req: &api.HTTPValidationRequest{
				GVK: &schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			},
			mapper: &mockRESTMapper{
				mappings: map[schema.GroupKind]*meta.RESTMapping{
					{Group: "apps", Kind: "Deployment"}: {
						Resource: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			v := NewResourceValidator(func() meta.RESTMapper { return tc.mapper })
			err := v.Validate(context.Background(), tc.req)

			if tc.expectError {
				s.Error(err)
				if ve, ok := err.(*api.ValidationError); ok {
					s.Equal(tc.errorCode, ve.Code)
				}
			} else {
				s.NoError(err)
			}
		})
	}
}

func TestResourceValidator(t *testing.T) {
	suite.Run(t, new(ResourceValidatorTestSuite))
}
