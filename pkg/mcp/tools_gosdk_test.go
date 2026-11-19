package mcp

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type ToolsGoSdkSuite struct {
	suite.Suite
}

func (s *ToolsGoSdkSuite) TestInputSchemaValidation() {
	s.Run("nil input schema returns error", func() {
		tool := api.ServerTool{
			Tool: api.Tool{Name: "no-schema"},
		}
		_, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().Error(err)
		s.Contains(err.Error(), "no-schema")
		s.Contains(err.Error(), "input schema")
	})

	s.Run("non-object input schema returns error", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "string-schema",
				InputSchema: &jsonschema.Schema{Type: "string"},
			},
		}
		_, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().Error(err)
		s.Contains(err.Error(), "string-schema")
		s.Contains(err.Error(), `"object"`)
	})

	s.Run("object input schema converts successfully", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "good-tool",
				InputSchema: &jsonschema.Schema{Type: "object"},
			},
		}
		mcpTool, handler, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().NoError(err)
		s.NotNil(mcpTool)
		s.NotNil(handler)
		s.Equal("good-tool", mcpTool.Name)
	})
}

func (s *ToolsGoSdkSuite) TestRBACMetadataConversion() {
	s.Run("adds declared RBAC while preserving existing metadata", func() {
		existingMeta := map[string]any{"ui": map[string]any{"resourceUri": "ui://server/app.html"}}
		rbac := api.RBACNone()
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "rbac-tool",
				InputSchema: &jsonschema.Schema{Type: "object"},
				Meta:        existingMeta,
			},
			RBAC: rbac,
		}

		mcpTool, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().NoError(err)
		s.Equal(existingMeta["ui"], mcpTool.Meta["ui"])
		s.Same(rbac, mcpTool.Meta[api.RBACMetadataKey])
		s.NotContains(existingMeta, api.RBACMetadataKey)
	})

	s.Run("leaves metadata unchanged when RBAC is not declared", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "no-rbac-metadata",
				InputSchema: &jsonschema.Schema{Type: "object"},
				Meta:        map[string]any{"existing": true},
			},
		}

		mcpTool, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().NoError(err)
		s.Equal(mcp.Meta{"existing": true}, mcpTool.Meta)
	})

	s.Run("rejects invalid RBAC metadata", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "invalid-rbac",
				InputSchema: &jsonschema.Schema{Type: "object"},
			},
			RBAC: api.RBACBounded(),
		}

		_, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid RBAC metadata")
	})

	s.Run("rejects RBAC metadata set directly on the tool without declared RBAC", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "direct-rbac",
				InputSchema: &jsonschema.Schema{Type: "object"},
				Meta:        map[string]any{api.RBACMetadataKey: map[string]any{}},
			},
		}

		_, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().Error(err)
		s.Contains(err.Error(), "ServerTool.RBAC")
	})

	s.Run("rejects RBAC metadata set directly on the tool with declared RBAC", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "duplicate-rbac",
				InputSchema: &jsonschema.Schema{Type: "object"},
				Meta:        map[string]any{api.RBACMetadataKey: map[string]any{}},
			},
			RBAC: api.RBACNone(),
		}

		_, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().Error(err)
		s.Contains(err.Error(), "ServerTool.RBAC")
	})
}

func (s *ToolsGoSdkSuite) TestOutputSchemaValidation() {
	s.Run("nil output schema converts successfully", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "no-output-schema",
				InputSchema: &jsonschema.Schema{Type: "object"},
			},
		}
		mcpTool, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().NoError(err)
		s.Nil(mcpTool.OutputSchema)
	})

	s.Run("nil output schema registers with the SDK without panicking", func() {
		// Regression guard for the conditional assignment in ServerToolToGoSdkTool:
		// a nil *jsonschema.Schema assigned to the SDK's any-typed OutputSchema
		// field is a non-nil interface, which makes AddTool dereference a nil
		// pointer and panic. s.Nil(mcpTool.OutputSchema) above cannot detect that
		// regression (reflect.IsNil sees through the interface), so exercise the
		// real registration path here.
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "register-no-output-schema",
				InputSchema: &jsonschema.Schema{Type: "object"},
			},
		}
		mcpTool, handler, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().NoError(err)
		server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
		s.NotPanics(func() { server.AddTool(mcpTool, handler) })
	})

	s.Run("non-object output schema returns error", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:         "string-schema",
				InputSchema:  &jsonschema.Schema{Type: "object"},
				OutputSchema: &jsonschema.Schema{Type: "string"},
			},
		}
		_, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().Error(err)
		s.Contains(err.Error(), "string-schema")
		s.Contains(err.Error(), `"object"`)
	})

	s.Run("object output schema converts successfully", func() {
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:        "with-output-schema",
				InputSchema: &jsonschema.Schema{Type: "object"},
				OutputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"result": {Type: "string"},
					},
				},
			},
		}
		mcpTool, _, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().NoError(err)
		s.Equal(tool.Tool.OutputSchema, mcpTool.OutputSchema)
	})

	s.Run("object output schema with nil properties converts and registers without panicking", func() {
		// The output path intentionally does not initialize Properties the way the
		// input path does (that init is an OpenAI-input-specific workaround, #717).
		// An object schema with nil Properties is still valid and must convert and
		// register without error.
		tool := api.ServerTool{
			Tool: api.Tool{
				Name:         "nil-properties-output-schema",
				InputSchema:  &jsonschema.Schema{Type: "object"},
				OutputSchema: &jsonschema.Schema{Type: "object"},
			},
		}
		mcpTool, handler, err := ServerToolToGoSdkTool(nil, tool)
		s.Require().NoError(err)
		s.Equal(tool.Tool.OutputSchema, mcpTool.OutputSchema)
		server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
		s.NotPanics(func() { server.AddTool(mcpTool, handler) })
	})
}

func TestToolsGoSdkSuite(t *testing.T) {
	suite.Run(t, new(ToolsGoSdkSuite))
}
