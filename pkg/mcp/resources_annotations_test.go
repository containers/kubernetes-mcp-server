package mcp

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type ResourceAnnotationsSuite struct {
	BaseMcpSuite
	originalToolsets []api.Toolset
}

func (s *ResourceAnnotationsSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.originalToolsets = toolsets.Toolsets()
}

func (s *ResourceAnnotationsSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	toolsets.Clear()
	for _, toolset := range s.originalToolsets {
		toolsets.Register(toolset)
	}
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsAllFieldsSet() {
	priority := 0.8
	lastModified := "2026-05-26T10:00:00Z"

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:          "test://example/annotated",
					Name:         "Annotated Resource",
					Description:  "Has all annotation fields",
					MIMEType:     "text/plain",
					Audience:     []string{"user", "assistant"},
					Priority:     &priority,
					LastModified: &lastModified,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("all annotation fields are set correctly", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Require().NotNil(resource.Annotations, "annotations should not be nil when fields are set")
		s.Require().Len(resource.Annotations.Audience, 2)
		s.Equal(mcp.Role("user"), resource.Annotations.Audience[0])
		s.Equal(mcp.Role("assistant"), resource.Annotations.Audience[1])
		s.Equal(0.8, resource.Annotations.Priority)
		s.Equal("2026-05-26T10:00:00Z", resource.Annotations.LastModified)
	})
}

func (s *ResourceAnnotationsSuite) TestResourceTemplateAnnotationsAllFieldsSet() {
	priority := 0.5
	lastModified := "2026-05-26T12:00:00Z"

	testToolset := &mockResourceToolset{
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate:  "test://example/{id}",
					Name:         "Annotated Template",
					Description:  "Has all annotation fields",
					MIMEType:     "application/json",
					Audience:     []string{"assistant"},
					Priority:     &priority,
					LastModified: &lastModified,
				},
				Handler: func(_ context.Context, uri string) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: `{"uri": "` + uri + `"}`}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("all template annotation fields are set correctly", func() {
		result, err := s.ListResourceTemplates()
		s.Require().NoError(err)
		s.Require().Len(result.ResourceTemplates, 1)

		template := result.ResourceTemplates[0]
		s.Require().NotNil(template.Annotations, "annotations should not be nil when fields are set")
		s.Require().Len(template.Annotations.Audience, 1)
		s.Equal(mcp.Role("assistant"), template.Annotations.Audience[0])
		s.Equal(0.5, template.Annotations.Priority)
		s.Equal("2026-05-26T12:00:00Z", template.Annotations.LastModified)
	})
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsNilWhenAllOmitted() {
	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:         "test://example/no-annotations",
					Name:        "No Annotations",
					Description: "Has no annotation fields set",
					MIMEType:    "text/plain",
					// Audience, Priority, LastModified all omitted
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("annotations are nil when all fields omitted", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Nil(resource.Annotations, "annotations should be nil when no fields are set")
	})
}

func (s *ResourceAnnotationsSuite) TestResourceTemplateAnnotationsNilWhenAllOmitted() {
	testToolset := &mockResourceToolset{
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: "test://example/{id}",
					Name:        "No Annotations Template",
					Description: "Has no annotation fields set",
					MIMEType:    "text/plain",
					// Audience, Priority, LastModified all omitted
				},
				Handler: func(_ context.Context, uri string) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "template: " + uri}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("template annotations are nil when all fields omitted", func() {
		result, err := s.ListResourceTemplates()
		s.Require().NoError(err)
		s.Require().Len(result.ResourceTemplates, 1)

		template := result.ResourceTemplates[0]
		s.Nil(template.Annotations, "annotations should be nil when no fields are set")
	})
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsOnlyAudience() {
	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:         "test://example/only-audience",
					Name:        "Only Audience",
					MIMEType:    "text/plain",
					Audience:    []string{"user"},
					Priority:    nil,
					LastModified: nil,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("only audience field is set", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Require().NotNil(resource.Annotations)
		s.Require().Len(resource.Annotations.Audience, 1)
		s.Equal(mcp.Role("user"), resource.Annotations.Audience[0])
		s.Equal(float64(0), resource.Annotations.Priority, "priority should be zero when not set")
		s.Empty(resource.Annotations.LastModified, "lastModified should be empty when not set")
	})
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsOnlyPriority() {
	priority := 0.9

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:          "test://example/only-priority",
					Name:         "Only Priority",
					MIMEType:     "text/plain",
					Audience:     nil,
					Priority:     &priority,
					LastModified: nil,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("only priority field is set", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Require().NotNil(resource.Annotations)
		s.Empty(resource.Annotations.Audience, "audience should be empty when not set")
		s.Equal(0.9, resource.Annotations.Priority)
		s.Empty(resource.Annotations.LastModified, "lastModified should be empty when not set")
	})
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsOnlyLastModified() {
	lastModified := "2026-05-26T15:30:00Z"

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:          "test://example/only-lastmod",
					Name:         "Only LastModified",
					MIMEType:     "text/plain",
					Audience:     nil,
					Priority:     nil,
					LastModified: &lastModified,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("only lastModified field is set", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Require().NotNil(resource.Annotations)
		s.Empty(resource.Annotations.Audience, "audience should be empty when not set")
		s.Equal(float64(0), resource.Annotations.Priority, "priority should be zero when not set")
		s.Equal("2026-05-26T15:30:00Z", resource.Annotations.LastModified)
	})
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsZeroPriority() {
	zeroPriority := 0.0

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/zero-priority",
					Name:     "Zero Priority",
					MIMEType: "text/plain",
					Priority: &zeroPriority,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("zero priority is preserved and annotations are not nil", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Require().NotNil(resource.Annotations, "annotations should not be nil even for zero priority")
		s.Equal(0.0, resource.Annotations.Priority, "zero priority should be preserved")
	})
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsEmptyAudienceArray() {
	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/empty-audience",
					Name:     "Empty Audience Array",
					MIMEType: "text/plain",
					Audience: []string{}, // Empty array, not nil
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("empty audience array results in nil annotations", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Nil(resource.Annotations, "annotations should be nil when audience is empty array")
	})
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsMultipleAudienceRoles() {
	priority := 0.7

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/multi-audience",
					Name:     "Multiple Audience",
					MIMEType: "text/plain",
					Audience: []string{"user", "assistant", "system"},
					Priority: &priority,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("multiple audience roles are preserved in order", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Require().NotNil(resource.Annotations)
		s.Require().Len(resource.Annotations.Audience, 3)
		s.Equal(mcp.Role("user"), resource.Annotations.Audience[0])
		s.Equal(mcp.Role("assistant"), resource.Annotations.Audience[1])
		s.Equal(mcp.Role("system"), resource.Annotations.Audience[2])
	})
}

func (s *ResourceAnnotationsSuite) TestResourceAnnotationsPriorityBoundaries() {
	minPriority := 0.0
	maxPriority := 1.0

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/min-priority",
					Name:     "Min Priority",
					MIMEType: "text/plain",
					Priority: &minPriority,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "min"}, nil
				},
			},
			{
				Resource: api.Resource{
					URI:      "test://example/max-priority",
					Name:     "Max Priority",
					MIMEType: "text/plain",
					Priority: &maxPriority,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "max"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("boundary priority values 0.0 and 1.0 are preserved", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 2)

		byURI := make(map[string]*mcp.Resource)
		for _, r := range result.Resources {
			byURI[r.URI] = r
		}

		minRes := byURI["test://example/min-priority"]
		s.Require().NotNil(minRes.Annotations)
		s.Equal(0.0, minRes.Annotations.Priority)

		maxRes := byURI["test://example/max-priority"]
		s.Require().NotNil(maxRes.Annotations)
		s.Equal(1.0, maxRes.Annotations.Priority)
	})
}

func (s *ResourceAnnotationsSuite) TestAnnotationsPersistAcrossReload() {
	priority := 0.6
	lastModified := "2026-05-26T16:00:00Z"

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:          "test://example/persistent",
					Name:         "Persistent Annotations",
					MIMEType:     "text/plain",
					Audience:     []string{"user"},
					Priority:     &priority,
					LastModified: &lastModified,
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "persistent"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("annotations present before reload", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)
		s.Require().NotNil(result.Resources[0].Annotations)
		s.Equal(0.6, result.Resources[0].Annotations.Priority)
	})

	s.Run("annotations persist after reload", func() {
		newConfig := config.Default()
		newConfig.Toolsets = []string{"resource-test"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		err := s.mcpServer.ReloadConfiguration(newConfig)
		s.Require().NoError(err)

		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)

		resource := result.Resources[0]
		s.Require().NotNil(resource.Annotations)
		s.Require().Len(resource.Annotations.Audience, 1)
		s.Equal(mcp.Role("user"), resource.Annotations.Audience[0])
		s.Equal(0.6, resource.Annotations.Priority)
		s.Equal("2026-05-26T16:00:00Z", resource.Annotations.LastModified)
	})
}

func TestResourceAnnotationsSuite(t *testing.T) {
	suite.Run(t, new(ResourceAnnotationsSuite))
}
