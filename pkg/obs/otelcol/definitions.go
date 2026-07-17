package otelcol

import (
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"

	"github.com/rhobs/obs-mcp/pkg/tools"
)

// componentTypeList returns a comma-separated list of valid component types for documentation.
func componentTypeList() string {
	types := ValidComponentTypes()
	strs := make([]string, len(types))
	for i, t := range types {
		strs[i] = string(t)
	}
	return strings.Join(strs, ", ")
}

var (
	listComponentsOutputSchema     = tools.MustSchema[ListComponentsOutput]()
	getComponentSchemaOutputSchema = tools.MustSchema[GetComponentSchemaOutput]()
	validateConfigOutputSchema     = tools.MustSchema[ValidateConfigOutput]()
	getVersionsOutputSchema        = tools.MustSchema[GetVersionsOutput]()
)

func initListComponents() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "otelcol_list_components",
			Description: "List available OpenTelemetry Collector components (receivers, processors, exporters, extensions, connectors) for a given version.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"version": {
						Type:        "string",
						Description: "Collector version (e.g., 'v0.100.0'). Defaults to latest available.",
					},
				},
			},
			OutputSchema: listComponentsOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "List OpenTelemetry Collector Components",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: ListComponentsHandler,
	}
}

func initGetComponentSchema() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "otelcol_get_component_schema",
			Description: "Get the JSON schema for an OpenTelemetry Collector component's configuration options.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"component_type": {
						Type:        "string",
						Description: "Component type: " + componentTypeList(),
					},
					"component_name": {
						Type:        "string",
						Description: "Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')",
					},
					"version": {
						Type:        "string",
						Description: "Collector version (e.g., 'v0.100.0'). Defaults to latest available.",
					},
				},
				Required: []string{"component_type", "component_name"},
			},
			OutputSchema: getComponentSchemaOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Get OpenTelemetry Collector Component Schema",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: GetComponentSchemaHandler,
	}
}

func initValidateConfig() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "otelcol_validate_config",
			Description: "Validate an OpenTelemetry Collector component configuration against its JSON schema.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"component_type": {
						Type:        "string",
						Description: "Component type: " + componentTypeList(),
					},
					"component_name": {
						Type:        "string",
						Description: "Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')",
					},
					"config": {
						Type:        "string",
						Description: "Configuration to validate as YAML or JSON string",
					},
					"format": {
						Type:        "string",
						Description: "Config format: 'yaml' (default) or 'json'",
					},
					"version": {
						Type:        "string",
						Description: "Collector version (e.g., 'v0.100.0'). Defaults to latest available.",
					},
				},
				Required: []string{"component_type", "component_name", "config"},
			},
			OutputSchema: validateConfigOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Validate OpenTelemetry Collector Component Configuration",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: ValidateConfigHandler,
	}
}

func initGetVersions() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "otelcol_get_versions",
			Description: "List available OpenTelemetry Collector versions and identify the latest.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
			},
			OutputSchema: getVersionsOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Get Available OpenTelemetry Collector Versions",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: GetVersionsHandler,
	}
}
