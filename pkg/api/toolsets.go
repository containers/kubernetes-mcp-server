package api

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
)

type ServerTool struct {
	Tool               Tool
	Handler            ToolHandlerFunc
	ClusterAware       *bool
	TargetListProvider *bool
}

// IsClusterAware indicates whether the tool can accept a "cluster" or "context" parameter
// to operate on a specific Kubernetes cluster context.
// Defaults to true if not explicitly set
func (s *ServerTool) IsClusterAware() bool {
	if s.ClusterAware != nil {
		return *s.ClusterAware
	}
	return true
}

// IsTargetListProvider indicates whether the tool is used to provide a list of targets (clusters/contexts)
// Defaults to false if not explicitly set
func (s *ServerTool) IsTargetListProvider() bool {
	if s.TargetListProvider != nil {
		return *s.TargetListProvider
	}
	return false
}

type Toolset interface {
	// GetName returns the name of the toolset.
	// Used to identify the toolset in configuration, logs, and command-line arguments.
	// Examples: "core", "metrics", "helm"
	GetName() string
	// GetDescription returns a human-readable description of the toolset.
	// Will be used to generate documentation and help text.
	GetDescription() string
	GetTools(o Openshift) []ServerTool
	// GetPrompts returns the prompts provided by this toolset.
	// Returns nil if the toolset doesn't provide any prompts.
	GetPrompts() []ServerPrompt
}

type ToolCallRequest interface {
	GetArguments() map[string]any
}

type ToolCallResult struct {
	// Raw content returned by the tool.
	Content string
	// StructuredContent is an optional JSON-serializable value for MCP Apps UI rendering.
	// When set, it is passed as structuredContent in the MCP CallToolResult alongside Content.
	// Must be completely omitted (nil) when not used.
	StructuredContent any
	// Error (non-protocol) to send back to the LLM.
	Error error
}

// NewToolCallResult creates a ToolCallResult with text content only.
// Use this for tools that return human-readable text output.
func NewToolCallResult(content string, err error) *ToolCallResult {
	return NewToolCallResultFull(content, nil, err)
}

// NewToolCallResultFull creates a ToolCallResult with both human-readable text
// and structured content.
// Use this when the text representation differs from a JSON serialization of the
// structured content (e.g., YAML or Table text alongside extracted structured data).
func NewToolCallResultFull(text string, structured any, err error) *ToolCallResult {
	return &ToolCallResult{
		Content:           text,
		StructuredContent: structured,
		Error:             err,
	}
}

// NewToolCallResultStructured creates a ToolCallResult with structured content.
// The structured value is automatically JSON-serialized into the Content field
// for backward compatibility with MCP clients that don't support structuredContent.
//
// Per the MCP specification:
// "For backwards compatibility, a tool that returns structured content SHOULD
// also return the serialized JSON in a TextContent block."
// https://modelcontextprotocol.io/specification/2025-11-25/server/tools#structured-content
//
// Use this for tools that return typed/structured data (maps, slices, structs)
// that MCP clients can parse programmatically.
func NewToolCallResultStructured(structured any, err error) *ToolCallResult {
	content := ""
	if structured != nil {
		if b, jsonErr := json.Marshal(structured); jsonErr == nil {
			content = string(b)
		}
	}
	return NewToolCallResultFull(content, structured, err)
}

type ToolHandlerParams struct {
	context.Context
	BaseConfig
	KubernetesClient
	ToolCallRequest
	ListOutput output.Output
	Elicitor
}

type ToolHandlerFunc func(params ToolHandlerParams) (*ToolCallResult, error)

// ErrElicitationNotSupported is returned when the MCP client does not support elicitation.
var ErrElicitationNotSupported = errors.New("client does not support elicitation")

// Elicitor provides a mechanism for tools and prompts to request additional information
// from the user during execution via the MCP elicitation protocol.
// It supports two modes:
//   - Form mode: presents a schema-based form to the user (set Message and RequestedSchema in ElicitParams).
//   - URL mode: directs the user to a URL (set Message, URL, and optionally ElicitationID in ElicitParams).
//
// See MCP specification: https://modelcontextprotocol.io/specification/2025-11-25/client/elicitation
type Elicitor interface {
	Elicit(ctx context.Context, params *ElicitParams) (*ElicitResult, error)
}

// ElicitParams contains the parameters for an elicitation request.
// The elicitation mode is inferred from the fields: if URL is set, URL mode is used; otherwise form mode.
type ElicitParams struct {
	// Message is the message to present to the user.
	Message string
	// RequestedSchema is a JSON Schema defining the expected form fields. Used in form mode only.
	RequestedSchema *jsonschema.Schema
	// URL is the URL to present to the user. Used in URL mode only.
	URL string
	// ElicitationID is a tracking identifier for out-of-band URL elicitation completion. Used in URL mode only.
	ElicitationID string
}

// ElicitAction constants define the possible user responses to an elicitation request.
// See MCP specification: https://modelcontextprotocol.io/specification/2025-11-25/client/elicitation
const (
	// ElicitActionAccept indicates the user submitted the form with content.
	ElicitActionAccept = "accept"
	// ElicitActionDecline indicates the user explicitly declined the request.
	ElicitActionDecline = "decline"
	// ElicitActionCancel indicates the user dismissed the form without making a choice.
	ElicitActionCancel = "cancel"
)

// ElicitResult represents the user's response to an elicitation request.
type ElicitResult struct {
	// Action is one of the ElicitAction constants.
	Action string
	// Content contains the submitted form data. Only populated when Action is ElicitActionAccept.
	Content map[string]any
}

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
	// Meta contains additional metadata for the tool (e.g., MCP Apps UI resource URI).
	// Example: map[string]any{"ui": map[string]any{"resourceUri": "ui://server/app.html"}}
	Meta map[string]any `json:"_meta,omitempty"`
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
