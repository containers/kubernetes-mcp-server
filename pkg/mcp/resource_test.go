package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/stretchr/testify/suite"
)

type ResourceSuite struct {
	BaseMcpSuite
	originalToolsets []api.Toolset
}

func (s *ResourceSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.originalToolsets = toolsets.Toolsets()
}

func (s *ResourceSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	toolsets.Clear()
	for _, toolset := range s.originalToolsets {
		toolsets.Register(toolset)
	}
}

func (s *ResourceSuite) TestResources() {
	txt1 := "Content 1"
	json2 := `{"key": "value"}`

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:         "test://example/resource1",
					Name:        "Resource One",
					Description: "First",
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: txt1}, nil
				},
			},
			{
				Resource: api.Resource{
					URI:         "test://example/resource2",
					Name:        "Resource Two",
					Description: "Second",
					MIMEType:    "application/json",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: json2}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("all resources appear in list", func() {
		result, err := s.ListResources()
		s.NoError(err)
		s.Require().Len(result.Resources, 2)

		uris := make(map[string]bool)
		for _, r := range result.Resources {
			uris[r.URI] = true
		}
		s.True(uris["test://example/resource1"])
		s.True(uris["test://example/resource2"])
	})

	s.Run("each resource has correct content and mimeType", func() {
		result1, err := s.ReadResource("test://example/resource1")
		s.NoError(err)
		s.Require().Len(result1.Contents, 1)
		s.Equal(txt1, result1.Contents[0].Text)
		s.Equal("text/plain", result1.Contents[0].MIMEType)

		result2, err := s.ReadResource("test://example/resource2")
		s.NoError(err)
		s.Require().Len(result2.Contents, 1)
		s.Equal(json2, result2.Contents[0].Text)
		s.Equal("application/json", result2.Contents[0].MIMEType)
	})
}

func (s *ResourceSuite) TestResourceTemplates() {
	uriTempl := "test://example/{name}"
	txtFoo := "foo-dynamic-resource"

	testToolset := &mockResourceToolset{
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: uriTempl,
					Name:        txtFoo,
					Description: txtFoo,
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context, uri string) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "content for: " + uri}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("template appears in list", func() {
		result, err := s.ListResourceTemplates()
		s.NoError(err)
		s.Require().Len(result.ResourceTemplates, 1)
		s.Equal(uriTempl, result.ResourceTemplates[0].URITemplate)
		s.Equal(txtFoo, result.ResourceTemplates[0].Name)
		s.Equal(txtFoo, result.ResourceTemplates[0].Description)
		s.Equal("text/plain", result.ResourceTemplates[0].MIMEType)
	})

	s.Run("handler receives correct URI for different URIs", func() {
		uri1 := "test://example/foo"
		result1, err := s.ReadResource(uri1)
		s.NoError(err)
		s.Require().Len(result1.Contents, 1)
		s.Equal(uri1, result1.Contents[0].URI)
		s.Equal("content for: "+uri1, result1.Contents[0].Text)

		uri2 := "test://example/bar"
		result2, err := s.ReadResource(uri2)
		s.NoError(err)
		s.Require().Len(result2.Contents, 1)
		s.Equal(uri2, result2.Contents[0].URI)
		s.Equal("content for: "+uri2, result2.Contents[0].Text)
	})
}

func (s *ResourceSuite) TestHandlerErrors() {
	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/error",
					Name:     "Error Resource",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return nil, errors.New("permission denied")
				},
			},
		},
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: "test://example/template/{id}",
					Name:        "Template with Error",
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context, uri string) (*api.ResourceContent, error) {
					return nil, errors.New("permission denied")
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("static resource handler error propagates", func() {
		result, err := s.ReadResource("test://example/error")
		s.Error(err)
		s.Nil(result)
	})

	s.Run("template resource handler error propagates", func() {
		result, err := s.ReadResource("test://example/template/123")
		s.Error(err)
		s.Nil(result)
	})
}

type mockResourceToolset struct {
	resources         []api.ServerResource
	resourceTemplates []api.ServerResourceTemplate
}

func (m *mockResourceToolset) GetName() string                           { return "resource-test" }
func (m *mockResourceToolset) GetDescription() string                    { return "Test toolset for resources" }
func (m *mockResourceToolset) GetTools(_ api.Openshift) []api.ServerTool { return nil }
func (m *mockResourceToolset) GetPrompts() []api.ServerPrompt            { return nil }
func (m *mockResourceToolset) GetResources() []api.ServerResource        { return m.resources }
func (m *mockResourceToolset) GetResourceTemplates() []api.ServerResourceTemplate {
	return m.resourceTemplates
}

func TestResourceSuite(t *testing.T) {
	suite.Run(t, new(ResourceSuite))
}
