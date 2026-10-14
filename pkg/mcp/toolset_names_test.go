package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

// namedToolset is a registrable toolset with a fixed name and tool list.
type namedToolset struct {
	name  string
	tools []api.ServerTool
}

func (t *namedToolset) GetName() string        { return t.name }
func (t *namedToolset) GetDescription() string { return "test toolset " + t.name }
func (t *namedToolset) GetTools(context.Context, api.ToolsetContext) []api.ServerTool {
	return t.tools
}
func (t *namedToolset) GetPrompts(context.Context, api.ToolsetContext) []api.ServerPrompt { return nil }
func (t *namedToolset) GetResources(context.Context, api.ToolsetContext) []api.ServerResource {
	return nil
}
func (t *namedToolset) GetResourceTemplates(context.Context, api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

func (s *ToolsetsSuite) TestDuplicateToolNamesAcrossToolsetsAreRejected() {
	// The SDK keeps the last tool registered under a name; two toolsets
	// exposing the same name would silently replace one with the other.
	toolsets.Clear()
	for _, name := range []string{"dup-a", "dup-b"} {
		toolsets.Register(&namedToolset{name: name, tools: []api.ServerTool{{
			Tool: api.Tool{Name: "shared_tool", Description: "from " + name, InputSchema: &jsonschema.Schema{Type: "object"}},
			Handler: func(api.ToolHandlerParams) (*api.ToolCallResult, error) {
				return api.NewToolCallResult("ok", nil), nil
			},
		}}})
	}
	s.Cfg.Toolsets.SetForTest([]string{"dup-a", "dup-b"})
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err)
	defer provider.Close()
	_, err = NewServer(s.T().Context(), Configuration{Config: s.Cfg}, provider)
	s.Require().Error(err, "a server with two tools named shared_tool must not start")
	s.ErrorContains(err, `duplicate tool name "shared_tool"`)
}
