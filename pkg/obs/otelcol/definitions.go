package otelcol

import (
	"strings"

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

// All tool definitions for OpenTelemetry Collector
var (
	ListComponents = tools.ToolDef[ListComponentsOutput]{
		Name:        "otelcol_list_components",
		Description: "List available OpenTelemetry Collector components (receivers, processors, exporters, extensions, connectors) for a given version.",
		Title:       "List OpenTelemetry Collector Components",
		Params: []tools.ParamDef{
			{
				Name:        "version",
				Type:        tools.ParamTypeString,
				Description: "Collector version (e.g., 'v0.100.0'). Defaults to latest available.",
				Required:    false,
			},
		},
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
	}

	GetComponentSchema = tools.ToolDef[GetComponentSchemaOutput]{
		Name:        "otelcol_get_component_schema",
		Description: "Get the JSON schema for an OpenTelemetry Collector component's configuration options.",
		Title:       "Get OpenTelemetry Collector Component Schema",
		Params: []tools.ParamDef{
			{
				Name:        "component_type",
				Type:        tools.ParamTypeString,
				Description: "Component type: " + componentTypeList(),
				Required:    true,
			},
			{
				Name:        "component_name",
				Type:        tools.ParamTypeString,
				Description: "Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')",
				Required:    true,
			},
			{
				Name:        "version",
				Type:        tools.ParamTypeString,
				Description: "Collector version (e.g., 'v0.100.0'). Defaults to latest available.",
				Required:    false,
			},
		},
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
	}

	ValidateConfig = tools.ToolDef[ValidateConfigOutput]{
		Name:        "otelcol_validate_config",
		Description: "Validate an OpenTelemetry Collector component configuration against its JSON schema.",
		Title:       "Validate OpenTelemetry Collector Component Configuration",
		Params: []tools.ParamDef{
			{
				Name:        "component_type",
				Type:        tools.ParamTypeString,
				Description: "Component type: " + componentTypeList(),
				Required:    true,
			},
			{
				Name:        "component_name",
				Type:        tools.ParamTypeString,
				Description: "Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')",
				Required:    true,
			},
			{
				Name:        "config",
				Type:        tools.ParamTypeString,
				Description: "Configuration to validate as YAML or JSON string",
				Required:    true,
			},
			{
				Name:        "format",
				Type:        tools.ParamTypeString,
				Description: "Config format: 'yaml' (default) or 'json'",
				Required:    false,
			},
			{
				Name:        "version",
				Type:        tools.ParamTypeString,
				Description: "Collector version (e.g., 'v0.100.0'). Defaults to latest available.",
				Required:    false,
			},
		},
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
	}

	GetVersions = tools.ToolDef[GetVersionsOutput]{
		Name:        "otelcol_get_versions",
		Description: "List available OpenTelemetry Collector versions and identify the latest.",
		Title:       "Get Available OpenTelemetry Collector Versions",
		Params:      []tools.ParamDef{},
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
	}
)

// AllTools returns all otelcol tool definitions.
func AllTools() []tools.ToolDefInterface {
	return []tools.ToolDefInterface{
		ListComponents,
		GetComponentSchema,
		ValidateConfig,
		GetVersions,
	}
}
