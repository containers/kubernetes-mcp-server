package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

	s.Run("all resources appear in list with correct metadata", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 2)

		byURI := make(map[string]*mcp.Resource)
		for _, r := range result.Resources {
			byURI[r.URI] = r
		}

		s.Require().Contains(byURI, "test://example/resource1")
		s.Equal("Resource One", byURI["test://example/resource1"].Name)
		s.Equal("First", byURI["test://example/resource1"].Description)
		s.Equal("text/plain", byURI["test://example/resource1"].MIMEType)

		s.Require().Contains(byURI, "test://example/resource2")
		s.Equal("Resource Two", byURI["test://example/resource2"].Name)
		s.Equal("Second", byURI["test://example/resource2"].Description)
		s.Equal("application/json", byURI["test://example/resource2"].MIMEType)
	})

	s.Run("each resource has correct content and mimeType", func() {
		result1, err := s.ReadResource("test://example/resource1")
		s.Require().NoError(err)
		s.Require().Len(result1.Contents, 1)
		s.Equal(txt1, result1.Contents[0].Text)
		s.Empty(result1.Contents[0].Blob)
		s.Equal("text/plain", result1.Contents[0].MIMEType)

		result2, err := s.ReadResource("test://example/resource2")
		s.Require().NoError(err)
		s.Require().Len(result2.Contents, 1)
		s.Equal(json2, result2.Contents[0].Text)
		s.Empty(result2.Contents[0].Blob)
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
		s.Require().NoError(err)
		s.Require().Len(result.ResourceTemplates, 1)
		s.Equal(uriTempl, result.ResourceTemplates[0].URITemplate)
		s.Equal(txtFoo, result.ResourceTemplates[0].Name)
		s.Equal(txtFoo, result.ResourceTemplates[0].Description)
		s.Equal("text/plain", result.ResourceTemplates[0].MIMEType)
	})

	s.Run("handler receives correct URI for different URIs", func() {
		uri1 := "test://example/foo"
		result1, err := s.ReadResource(uri1)
		s.Require().NoError(err)
		s.Require().Len(result1.Contents, 1)
		s.Equal(uri1, result1.Contents[0].URI)
		s.Equal("content for: "+uri1, result1.Contents[0].Text)
		s.Empty(result1.Contents[0].Blob)

		uri2 := "test://example/bar"
		result2, err := s.ReadResource(uri2)
		s.Require().NoError(err)
		s.Require().Len(result2.Contents, 1)
		s.Equal(uri2, result2.Contents[0].URI)
		s.Equal("content for: "+uri2, result2.Contents[0].Text)
		s.Empty(result2.Contents[0].Blob)
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

func (s *ResourceSuite) TestNilContentReturnsError() {
	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/nil-content",
					Name:     "Nil Content Resource",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return nil, nil
				},
			},
		},
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: "test://example/nil-template/{id}",
					Name:        "Nil Content Template",
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context, _ string) (*api.ResourceContent, error) {
					return nil, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("static resource nil content returns error", func() {
		result, err := s.ReadResource("test://example/nil-content")
		s.Error(err)
		s.Nil(result)
	})

	s.Run("template resource nil content returns error", func() {
		result, err := s.ReadResource("test://example/nil-template/123")
		s.Error(err)
		s.Nil(result)
	})
}

func (s *ResourceSuite) TestReloadRemovesResources() {
	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/removable",
					Name:     "Removable",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "will be removed"}, nil
				},
			},
		},
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: "test://example/removable/{id}",
					Name:        "Removable Template",
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context, uri string) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "template: " + uri}, nil
				},
			},
		},
	}

	emptyToolset := &mockResourceToolset{name: "resource-test-empty"}

	toolsets.Clear()
	toolsets.Register(testToolset)
	toolsets.Register(emptyToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("resources present before reload", func() {
		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Require().Len(result.Resources, 1)
		s.Equal("test://example/removable", result.Resources[0].URI)

		tmpl, err := s.ListResourceTemplates()
		s.Require().NoError(err)
		s.Require().Len(tmpl.ResourceTemplates, 1)
	})

	s.Run("resources removed after reload", func() {
		newConfig := config.Default()
		newConfig.Toolsets = []string{"resource-test-empty"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		err := s.mcpServer.ReloadConfiguration(newConfig)
		s.Require().NoError(err)

		result, err := s.ListResources()
		s.Require().NoError(err)
		s.Empty(result.Resources)

		tmpl, err := s.ListResourceTemplates()
		s.Require().NoError(err)
		s.Empty(tmpl.ResourceTemplates)
	})
}

func (s *ResourceSuite) TestReloadNotifiesResourceListChanged() {
	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/notify",
					Name:     "Notify Resource",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "notify"}, nil
				},
			},
		},
	}

	emptyToolset := &mockResourceToolset{name: "resource-test-empty"}

	toolsets.Clear()
	toolsets.Register(testToolset)
	toolsets.Register(emptyToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	capture := s.StartCapturingNotifications()

	newConfig := config.Default()
	newConfig.Toolsets = []string{"resource-test-empty"}
	newConfig.KubeConfig = s.Cfg.KubeConfig

	err := s.mcpServer.ReloadConfiguration(newConfig)
	s.Require().NoError(err)

	notification := capture.RequireNotification(s.T(), 2*time.Second, "notifications/resources/list_changed")
	s.NotNil(notification)
}

func (s *ResourceSuite) TestBlobResource() {
	blobData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/image",
					Name:     "Image Resource",
					MIMEType: "image/png",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Blob: blobData}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("blob content is returned correctly", func() {
		result, err := s.ReadResource("test://example/image")
		s.Require().NoError(err)
		s.Require().Len(result.Contents, 1)
		s.Equal("image/png", result.Contents[0].MIMEType)
		s.Equal(blobData, result.Contents[0].Blob)
		s.Empty(result.Contents[0].Text)
	})
}

func (s *ResourceSuite) TestMIMETypeOverride() {
	testToolset := &mockResourceToolset{
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/override",
					Name:     "Override Resource",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{
						Text:     `{"overridden": true}`,
						MIMEType: "application/json",
					}, nil
				},
			},
		},
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: "test://example/tmpl-override/{id}",
					Name:        "Override Template",
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context, uri string) (*api.ResourceContent, error) {
					return &api.ResourceContent{
						Text:     `{"uri": "` + uri + `"}`,
						MIMEType: "application/json",
					}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"resource-test"}
	s.InitMcpClient()

	s.Run("handler MIMEType overrides resource-level MIMEType", func() {
		result, err := s.ReadResource("test://example/override")
		s.Require().NoError(err)
		s.Require().Len(result.Contents, 1)
		s.Equal("application/json", result.Contents[0].MIMEType)
		s.Equal(`{"overridden": true}`, result.Contents[0].Text)
	})

	s.Run("handler MIMEType overrides template-level MIMEType", func() {
		result, err := s.ReadResource("test://example/tmpl-override/42")
		s.Require().NoError(err)
		s.Require().Len(result.Contents, 1)
		s.Equal("application/json", result.Contents[0].MIMEType)
	})
}

func (s *ResourceSuite) TestInvalidURITemplateReturnsError() {
	goodToolset := &mockResourceToolset{
		name: "resource-test-good",
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/good",
					Name:     "Good Resource",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "still up"}, nil
				},
			},
		},
	}

	badTemplateToolset := &mockResourceToolset{
		name: "resource-test-bad",
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: "{{invalid",
					Name:        "Bad Template",
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context, _ string) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "unreachable"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(goodToolset)
	toolsets.Register(badTemplateToolset)
	s.Cfg.Toolsets = []string{"resource-test-good"}
	s.InitMcpClient()

	s.Run("invalid resource template URI returns error without panic", func() {
		newConfig := config.Default()
		newConfig.Toolsets = []string{"resource-test-bad"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		s.NotPanics(func() {
			err := s.mcpServer.ReloadConfiguration(newConfig)
			s.Require().Error(err)
			s.Contains(err.Error(), "{{invalid")
		})
	})

	s.Run("previously registered resource still served after failed reload", func() {
		// Reload aborted in the convert phase, so SDK state must be unchanged
		// from the pre-reload snapshot — the good resource is still readable.
		result, err := s.ReadResource("test://example/good")
		s.Require().NoError(err)
		s.Require().Len(result.Contents, 1)
		s.Equal("still up", result.Contents[0].Text)
	})

	s.Run("configuration rolled back after failed reload", func() {
		// SDK state is unchanged after the failed reload, so s.configuration
		// must mirror that — otherwise downstream reads (rate limit, list
		// output, confirmation rules, ...) would see a config that disagrees
		// with what the SDK is actually serving.
		s.Equal([]string{"resource-test-good"}, s.mcpServer.configuration.StaticConfig.Toolsets)
	})

	s.Run("server accepts a subsequent valid reload", func() {
		recoveryConfig := config.Default()
		recoveryConfig.Toolsets = []string{"resource-test-good"}
		recoveryConfig.KubeConfig = s.Cfg.KubeConfig

		s.Require().NoError(s.mcpServer.ReloadConfiguration(recoveryConfig))

		result, err := s.ReadResource("test://example/good")
		s.Require().NoError(err)
		s.Require().Len(result.Contents, 1)
		s.Equal("still up", result.Contents[0].Text)
	})
}

func (s *ResourceSuite) TestInvalidResourceURIReturnsError() {
	goodToolset := &mockResourceToolset{
		name: "resource-test-good",
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/good",
					Name:     "Good Resource",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "still up"}, nil
				},
			},
		},
	}

	badURIToolset := &mockResourceToolset{
		name: "resource-test-bad-uri",
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					// Invalid percent-escape: triggers url.Parse error.
					URI:      "test://%zz",
					Name:     "Bad URI Resource",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "unreachable"}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(goodToolset)
	toolsets.Register(badURIToolset)
	s.Cfg.Toolsets = []string{"resource-test-good"}
	s.InitMcpClient()

	s.Run("invalid resource URI returns error without panic", func() {
		newConfig := config.Default()
		newConfig.Toolsets = []string{"resource-test-bad-uri"}
		newConfig.KubeConfig = s.Cfg.KubeConfig

		s.NotPanics(func() {
			err := s.mcpServer.ReloadConfiguration(newConfig)
			s.Require().Error(err)
			s.Contains(err.Error(), "%zz")
		})
	})

	s.Run("previously registered resource still served after failed reload", func() {
		// Reload aborted in the convert phase, so SDK state must be unchanged
		// from the pre-reload snapshot — the good resource is still readable.
		result, err := s.ReadResource("test://example/good")
		s.Require().NoError(err)
		s.Require().Len(result.Contents, 1)
		s.Equal("still up", result.Contents[0].Text)
	})
}

func (s *ResourceSuite) TestResourceContentInvariant() {
	bothEmptyToolset := &mockResourceToolset{
		name: "resource-test-both-empty",
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/both-empty",
					Name:     "Both Empty",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{}, nil
				},
			},
		},
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: "test://example/tmpl-both-empty/{id}",
					Name:        "Tmpl Both Empty",
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context, _ string) (*api.ResourceContent, error) {
					return &api.ResourceContent{}, nil
				},
			},
		},
	}

	bothSetToolset := &mockResourceToolset{
		name: "resource-test-both-set",
		resources: []api.ServerResource{
			{
				Resource: api.Resource{
					URI:      "test://example/both-set",
					Name:     "Both Set",
					MIMEType: "text/plain",
				},
				Handler: func(_ context.Context) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "x", Blob: []byte{0x01}}, nil
				},
			},
		},
		resourceTemplates: []api.ServerResourceTemplate{
			{
				ResourceTemplate: api.ResourceTemplate{
					URITemplate: "test://example/tmpl-both-set/{id}",
					Name:        "Tmpl Both Set",
					MIMEType:    "text/plain",
				},
				Handler: func(_ context.Context, _ string) (*api.ResourceContent, error) {
					return &api.ResourceContent{Text: "x", Blob: []byte{0x01}}, nil
				},
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(bothEmptyToolset)
	toolsets.Register(bothSetToolset)
	s.Cfg.Toolsets = []string{"resource-test-both-empty", "resource-test-both-set"}
	s.InitMcpClient()

	s.Run("static resource with both Text and Blob empty returns error", func() {
		result, err := s.ReadResource("test://example/both-empty")
		s.Error(err)
		s.Nil(result)
	})

	s.Run("template resource with both Text and Blob empty returns error", func() {
		result, err := s.ReadResource("test://example/tmpl-both-empty/123")
		s.Error(err)
		s.Nil(result)
	})

	s.Run("static resource with both Text and Blob set returns error", func() {
		result, err := s.ReadResource("test://example/both-set")
		s.Error(err)
		s.Nil(result)
	})

	s.Run("template resource with both Text and Blob set returns error", func() {
		result, err := s.ReadResource("test://example/tmpl-both-set/123")
		s.Error(err)
		s.Nil(result)
	})
}

type mockResourceToolset struct {
	name              string
	resources         []api.ServerResource
	resourceTemplates []api.ServerResourceTemplate
}

func (m *mockResourceToolset) GetName() string {
	if m.name != "" {
		return m.name
	}
	return "resource-test"
}
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
