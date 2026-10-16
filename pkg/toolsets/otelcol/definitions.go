package otelcol

import (
	"context"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// componentTypeList returns a comma-separated list of valid component types for documentation.
// componentTypeEnum is the enum of the component_type argument.
func componentTypeEnum() []any {
	types := ValidComponentTypes()
	out := make([]any, len(types))
	for i, t := range types {
		out[i] = string(t)
	}
	return out
}

func componentTypeList() string {
	types := ValidComponentTypes()
	strs := make([]string, len(types))
	for i, t := range types {
		strs[i] = string(t)
	}
	return strings.Join(strs, ", ")
}

var (
	listComponentsOutputSchema     = MustSchema[ListComponentsOutput]()
	getComponentSchemaOutputSchema = MustSchema[GetComponentSchemaOutput]()
	validateConfigOutputSchema     = MustSchema[ValidateConfigOutput]()
	getVersionsOutputSchema        = MustSchema[GetVersionsOutput]()

	openTelemetryCollectorGVK = schema.GroupVersionKind{
		Group:   "opentelemetry.io",
		Version: "v1beta1",
		Kind:    "OpenTelemetryCollector",
	}
)

func hasOpenTelemetryCollectorCRD(p api.ClusterInspector) func() bool {
	return func() bool {
		return api.AnyTargetHasGVK(context.TODO(), p, openTelemetryCollectorGVK)
	}
}

func initListComponents(p api.ClusterInspector) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "otelcol_list_components",
			Description: "List the components of an OpenTelemetry Collector version by name: receivers, processors, exporters, extensions and connectors. Defaults to the latest version.",
			Examples:    []api.Example{{Summary: "Components of the latest Collector"}},
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
		RBAC:    rbacNoKubernetes(),
		TargetCompatibilityFilters: []func() bool{
			hasOpenTelemetryCollectorCRD(p),
		},
	}
}

func initGetComponentSchema(p api.ClusterInspector) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "otelcol_get_component_schema",
			Description: "Show the JSON schema of one Collector component's configuration: every option with its type and default. Use it before writing or validating a component's config.",
			Examples:    []api.Example{{Summary: "Schema of the OTLP receiver", Arguments: map[string]any{"componentType": "receiver", "componentName": "otlp"}}},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"componentType": {
						Type:        "string",
						Enum:        componentTypeEnum(),
						Description: "Component type: " + componentTypeList(),
					},
					"componentName": {
						Type:        "string",
						Description: "Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')",
					},
					"version": {
						Type:        "string",
						Description: "Collector version (e.g., 'v0.100.0'). Defaults to latest available.",
					},
				},
				Required: []string{"componentType", "componentName"},
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
		RBAC:    rbacNoKubernetes(),
		TargetCompatibilityFilters: []func() bool{
			hasOpenTelemetryCollectorCRD(p),
		},
	}
}

func initValidateConfig(p api.ClusterInspector) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "otelcol_validate_config",
			Description: "Validate one Collector component's configuration, as YAML or JSON, against its schema and report what is wrong. Use it before applying a Collector config.",
			Examples:    []api.Example{{Summary: "Validate a batch processor config", Arguments: map[string]any{"componentType": "processor", "componentName": "batch", "config": "timeout: 5s\\nsend_batch_size: 1024"}}},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"componentType": {
						Type:        "string",
						Enum:        componentTypeEnum(),
						Description: "Component type: " + componentTypeList(),
					},
					"componentName": {
						Type:        "string",
						Description: "Component name from otelcol_list_components (e.g., 'otlp', 'batch', 'debug')",
					},
					"config": {
						Type:        "string",
						Description: "Configuration to validate as YAML or JSON string",
					},
					"format": {
						Type:        "string",
						Enum:        []any{"yaml", "json"},
						Description: "Config format: 'yaml' (default) or 'json'",
					},
					"version": {
						Type:        "string",
						Description: "Collector version (e.g., 'v0.100.0'). Defaults to latest available.",
					},
				},
				Required: []string{"componentType", "componentName", "config"},
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
		RBAC:    rbacNoKubernetes(),
		TargetCompatibilityFilters: []func() bool{
			hasOpenTelemetryCollectorCRD(p),
		},
	}
}

func initGetVersions(p api.ClusterInspector) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "otelcol_get_versions",
			Description: "List the OpenTelemetry Collector versions whose component schemas are available, and which one is the latest.",
			Examples:    []api.Example{{Summary: "Which Collector versions are known"}},
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
		RBAC:    rbacNoKubernetes(),
		TargetCompatibilityFilters: []func() bool{
			hasOpenTelemetryCollectorCRD(p),
		},
	}
}

// MustSchema returns the JSON Schema for type T, panicking on error.
func MustSchema[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	return s
}
