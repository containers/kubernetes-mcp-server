package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type CRDOpenAPISuite struct {
	BaseMcpSuite
}

func (s *CRDOpenAPISuite) TestCRDOpenAPISpecResourceTemplate() {
	s.Require().NoError(EnvTestEnableCRD(s.T().Context(), "kubevirt.io", "v1", "virtualmachines"))
	s.T().Cleanup(func() {
		s.Require().NoError(EnvTestDisableCRD(s.T().Context(), "kubevirt.io", "v1", "virtualmachines"))
	})
	s.InitMcpClient()

	s.Run("returns OpenAPI spec for existing CRD", func() {
		result, err := s.ReadResource("k8s://crds/virtualmachines.kubevirt.io/openapi")
		s.Require().NoError(err, "reading resource should not fail")
		s.Require().NotNil(result, "result should not be nil")
		s.Require().Len(result.Contents, 1, "expected exactly one content")

		textContent, ok := mcp.AsTextResourceContents(result.Contents[0])
		s.Require().True(ok, "expected text resource contents")
		s.Equal("k8s://crds/virtualmachines.kubevirt.io/openapi", textContent.URI)
		s.Equal("application/json", textContent.MIMEType)

		// Parse and verify the JSON structure
		var response struct {
			Group    string `json:"group"`
			Kind     string `json:"kind"`
			Versions []struct {
				Name          string      `json:"name"`
				Served        bool        `json:"served"`
				Storage       bool        `json:"storage"`
				OpenAPISchema interface{} `json:"openAPIV3Schema,omitempty"`
			} `json:"versions"`
		}
		err = json.Unmarshal([]byte(textContent.Text), &response)
		s.Require().NoError(err, "response should be valid JSON")
		s.Equal("kubevirt.io", response.Group)
		s.Equal("VirtualMachine", response.Kind)
		s.NotEmpty(response.Versions, "expected at least one version")

		// Check the version details
		foundV1 := false
		for _, v := range response.Versions {
			if v.Name == "v1" {
				foundV1 = true
				s.NotNil(v.OpenAPISchema, "v1 should have an OpenAPI schema")
			}
		}
		s.True(foundV1, "expected to find v1 version")
	})

	s.Run("returns error for nonexistent CRD", func() {
		_, err := s.ReadResource("k8s://crds/nonexistent.example.com/openapi")
		s.Error(err, "should return error for nonexistent CRD")
		s.Contains(err.Error(), "not found", "error should indicate CRD not found")
	})
}

func TestCRDOpenAPI(t *testing.T) {
	suite.Run(t, new(CRDOpenAPISuite))
}
