package tools

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/mark3labs/mcp-go/mcp"
)

// ParamDef defines a tool parameter
type ParamDef struct {
	Name        string
	Type        ParamType
	Description string
	Required    bool
	Pattern     string
}

// ParamType represents the type of a parameter
type ParamType string

const (
	ParamTypeString  ParamType = "string"
	ParamTypeBoolean ParamType = "boolean"
)

// ToolDef defines a tool that can be converted to different formats (MCP, Toolset, etc.)
type ToolDef struct {
	Name        string
	Description string
	Title       string
	Params      []ParamDef
	ReadOnly    bool
	Destructive bool
	Idempotent  bool
	OpenWorld   bool
}

// ToMCPTool converts a ToolDef to an mcp.Tool
func (d ToolDef) ToMCPTool() mcp.Tool {
	opts := []mcp.ToolOption{mcp.WithDescription(d.Description)}

	for _, param := range d.Params {
		switch param.Type {
		case ParamTypeString:
			stringOpts := []mcp.PropertyOption{mcp.Description(param.Description)}
			if param.Required {
				stringOpts = append(stringOpts, mcp.Required())
			}
			if param.Pattern != "" {
				stringOpts = append(stringOpts, mcp.Pattern(param.Pattern))
			}
			opts = append(opts, mcp.WithString(param.Name, stringOpts...))

		case ParamTypeBoolean:
			boolOpts := []mcp.PropertyOption{mcp.Description(param.Description)}
			if param.Required {
				boolOpts = append(boolOpts, mcp.Required())
			}
			opts = append(opts, mcp.WithBoolean(param.Name, boolOpts...))
		}
	}

	tool := mcp.NewTool(d.Name, opts...)

	// Workaround for tools with no parameters
	// See https://github.com/containers/kubernetes-mcp-server/pull/341/files
	if len(d.Params) == 0 {
		tool.InputSchema = mcp.ToolInputSchema{}
		tool.RawInputSchema = []byte(`{"type":"object","properties":{}}`)
	}

	return tool
}

// ToServerTool converts a ToolDef to an api.ServerTool
func (d ToolDef) ToServerTool(handler func(api.ToolHandlerParams) (*api.ToolCallResult, error)) api.ServerTool {
	properties := make(map[string]*jsonschema.Schema)
	var required []string

	for _, param := range d.Params {
		schema := &jsonschema.Schema{
			Description: param.Description,
		}

		switch param.Type {
		case ParamTypeString:
			schema.Type = "string"
			if param.Pattern != "" {
				schema.Pattern = param.Pattern
			}
		case ParamTypeBoolean:
			schema.Type = "boolean"
		}

		properties[param.Name] = schema

		if param.Required {
			required = append(required, param.Name)
		}
	}

	inputSchema := &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
	}

	if len(required) > 0 {
		inputSchema.Required = required
	}

	return api.ServerTool{
		Tool: api.Tool{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: inputSchema,
			Annotations: api.ToolAnnotations{
				Title:           d.Title,
				ReadOnlyHint:    ptr.To(d.ReadOnly),
				DestructiveHint: ptr.To(d.Destructive),
				IdempotentHint:  ptr.To(d.Idempotent),
				OpenWorldHint:   ptr.To(d.OpenWorld),
			},
		},
		Handler: handler,
	}
}
