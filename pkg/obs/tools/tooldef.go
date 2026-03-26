package tools

import (
	"encoding/json"
	"reflect"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolDefInterface defines the common interface for all tool definitions
type ToolDefInterface interface {
	ToMCPTool() *mcp.Tool
}

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
// T is the output schema type for this tool
type ToolDef[T any] struct {
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
func (d ToolDef[T]) ToMCPTool() *mcp.Tool {
	properties := make(map[string]any)
	var required []any

	// Build JSON schema properties for each parameter
	for _, param := range d.Params {
		property := map[string]any{
			"description": param.Description,
		}

		switch param.Type {
		case ParamTypeString:
			property["type"] = "string"
			if param.Pattern != "" {
				property["pattern"] = param.Pattern
			}
		case ParamTypeBoolean:
			property["type"] = "boolean"
		}

		properties[param.Name] = property

		if param.Required {
			required = append(required, param.Name)
		}
	}

	// Create input schema
	inputSchema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		inputSchema["required"] = required
	}

	// Generate output schema from generic type T
	var outputSchema map[string]any
	var zero T
	schema, err := jsonschema.ForType(reflect.TypeOf(zero), nil)
	if err == nil {
		// Convert schema to map[string]any by marshaling/unmarshaling
		if schemaBytes, err := json.Marshal(schema); err == nil {
			_ = json.Unmarshal(schemaBytes, &outputSchema)
		}
	}

	// Create and populate tool
	tool := &mcp.Tool{
		Name:         d.Name,
		Description:  d.Description,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}

	if d.Title != "" {
		tool.Title = d.Title
	}

	return tool
}

// ToServerTool converts a ToolDef to an api.ServerTool
func (d ToolDef[T]) ToServerTool(handler func(api.ToolHandlerParams) (*api.ToolCallResult, error)) api.ServerTool {
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
		// TODO(saswatamcode): Modify this selectively on ACM setups.
		ClusterAware: ptr.To(false),
	}
}
