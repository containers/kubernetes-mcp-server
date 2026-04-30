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

func TestToolsGoSdkSuite(t *testing.T) {
	suite.Run(t, new(ToolsGoSdkSuite))
}
