package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Toolset interface {
	GetName() string
	GetDescription() string
	GetTools(s *Server) []ServerTool
}

type ServerTool struct {
	Tool    Tool
	Handler ToolHandlerFunc
}

type ToolHandlerFunc func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

type Tool struct {
	// The name of the tool.
	// Intended for programmatic or logical use, but used as a display name in past
	// specs or fallback (if title isn't present).
	Name string `json:"name"`
	// A human-readable description of the tool.
	//
	// This can be used by clients to improve the LLM's understanding of available
	// tools. It can be thought of like a "hint" to the model.
	Description string `json:"description,omitempty"`
	// Additional tool information.
	Annotations ToolAnnotations `json:"annotations"`
	// A JSON Schema object defining the expected parameters for the tool.
	InputSchema *jsonschema.Schema
}

type ToolAnnotations struct {
	// Human-readable title for the tool
	Title string `json:"title,omitempty"`
	// If true, the tool does not modify its environment.
	ReadOnlyHint *bool `json:"readOnlyHint,omitempty"`
	// If true, the tool may perform destructive updates to its environment. If
	// false, the tool performs only additive updates.
	//
	// (This property is meaningful only when ReadOnlyHint == false.)
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	// If true, calling the tool repeatedly with the same arguments will have no
	// additional effect on its environment.
	//
	// (This property is meaningful only when ReadOnlyHint == false.)
	IdempotentHint *bool `json:"idempotentHint,omitempty"`
	// If true, this tool may interact with an "open world" of external entities. If
	// false, the tool's domain of interaction is closed. For example, the world of
	// a web search tool is open, whereas that of a memory tool is not.
	OpenWorldHint *bool `json:"openWorldHint,omitempty"`
}

var toolsets []Toolset

func Register(toolset Toolset) {
	toolsets = append(toolsets, toolset)
}

func Toolsets() []Toolset {
	return toolsets
}

func ToolsetNames() []string {
	names := make([]string, 0)
	for _, toolset := range Toolsets() {
		names = append(names, toolset.GetName())
	}
	return names
}

func ToolsetFromString(name string) Toolset {
	for _, toolset := range Toolsets() {
		if toolset.GetName() == name {
			return toolset
		}
	}
	return nil
}

func ToRawMessage(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func ServerToolToM3LabsServerTool(tools []ServerTool) ([]server.ServerTool, error) {
	m3labTools := make([]server.ServerTool, 0)
	for _, tool := range tools {
		m3labTool := mcp.Tool{
			Name:        tool.Tool.Name,
			Description: tool.Tool.Description,
			Annotations: mcp.ToolAnnotation{
				Title:           tool.Tool.Annotations.Title,
				ReadOnlyHint:    tool.Tool.Annotations.ReadOnlyHint,
				DestructiveHint: tool.Tool.Annotations.DestructiveHint,
				IdempotentHint:  tool.Tool.Annotations.IdempotentHint,
				OpenWorldHint:   tool.Tool.Annotations.OpenWorldHint,
			},
		}
		if tool.Tool.InputSchema != nil {
			schema, err := json.Marshal(tool.Tool.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool input schema for tool %s: %v", tool.Tool.Name, err)
			}
			m3labTool.RawInputSchema = schema
		}
		m3labTools = append(m3labTools, server.ServerTool{Tool: m3labTool, Handler: server.ToolHandlerFunc(tool.Handler)})
	}
	return m3labTools, nil
}

type Full struct{}

var _ Toolset = (*Full)(nil)

func (p *Full) GetName() string {
	return "full"
}
func (p *Full) GetDescription() string {
	return "Complete toolset with all tools and extended outputs"
}
func (p *Full) GetTools(s *Server) []ServerTool {
	return slices.Concat(
		s.initConfiguration(),
		s.initEvents(),
		s.initNamespaces(),
		s.initPods(),
		s.initResources(),
		s.initHelm(),
	)
}

func init() {
	Register(&Full{})
}
