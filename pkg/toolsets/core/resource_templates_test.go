package core

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ResourceTemplatesSuite struct {
	suite.Suite
}

func (s *ResourceTemplatesSuite) TestParseCRDNameFromURI() {
	s.Run("valid URIs", func() {
		testCases := []struct {
			uri      string
			expected string
		}{
			{"k8s://crds/virtualmachines.kubevirt.io/openapi", "virtualmachines.kubevirt.io"},
			{"k8s://crds/pods.core.kubernetes.io/openapi", "pods.core.kubernetes.io"},
			{"k8s://crds/simple/openapi", "simple"},
		}
		for _, tc := range testCases {
			s.Run(tc.uri, func() {
				name, err := parseCRDNameFromURI(tc.uri)
				s.NoError(err)
				s.Equal(tc.expected, name)
			})
		}
	})

	s.Run("invalid URIs", func() {
		testCases := []struct {
			uri         string
			errContains string
		}{
			{"k8s://pods/default/mypod", "expected prefix"},
			{"k8s://crds//openapi", "CRD name cannot be empty"},
			{"k8s://crds/myresource/other", "expected suffix"},
			{"https://example.com/crd", "expected prefix"},
		}
		for _, tc := range testCases {
			s.Run(tc.uri, func() {
				_, err := parseCRDNameFromURI(tc.uri)
				s.Error(err)
				s.Contains(err.Error(), tc.errContains)
			})
		}
	})
}

func (s *ResourceTemplatesSuite) TestBuildCRDOpenAPIResponse() {
	s.Run("builds response with multiple versions", func() {
		response := buildCRDOpenAPIResponse("example.com", "Example", nil)
		s.Equal("example.com", response.Group)
		s.Equal("Example", response.Kind)
		s.Empty(response.Versions)
	})
}

func TestResourceTemplates(t *testing.T) {
	suite.Run(t, new(ResourceTemplatesSuite))
}
