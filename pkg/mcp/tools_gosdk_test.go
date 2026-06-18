package mcp

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
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
}

func TestToolsGoSdkSuite(t *testing.T) {
	suite.Run(t, new(ToolsGoSdkSuite))
}
