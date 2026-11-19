package otelcol

import "slices"

// ComponentType represents the type of OpenTelemetry component.
type ComponentType string

const (
	ComponentTypeReceiver  ComponentType = "receiver"
	ComponentTypeProcessor ComponentType = "processor"
	ComponentTypeExporter  ComponentType = "exporter"
	ComponentTypeExtension ComponentType = "extension"
	ComponentTypeConnector ComponentType = "connector"
)

// ValidComponentTypes returns all valid component types.
func ValidComponentTypes() []ComponentType {
	return []ComponentType{
		ComponentTypeReceiver,
		ComponentTypeProcessor,
		ComponentTypeExporter,
		ComponentTypeExtension,
		ComponentTypeConnector,
	}
}

// IsValid checks if the component type is valid.
func (ct ComponentType) IsValid() bool {
	return slices.Contains(ValidComponentTypes(), ct)
}

// ListComponentsInput defines the input parameters for ListComponentsHandler.
type ListComponentsInput struct {
	Version string `json:"version,omitempty"`
}

// ListComponentsOutput defines the output schema for the otelcol_list_components tool.
type ListComponentsOutput struct {
	Version    string              `json:"version" jsonschema:"The OpenTelemetry Collector version"`
	Receivers  []string            `json:"receivers" jsonschema:"List of available receiver component names"`
	Processors []string            `json:"processors" jsonschema:"List of available processor component names"`
	Exporters  []string            `json:"exporters" jsonschema:"List of available exporter component names"`
	Extensions []string            `json:"extensions" jsonschema:"List of available extension component names"`
	Connectors []string            `json:"connectors" jsonschema:"List of available connector component names"`
	Components map[string][]string `json:"components" jsonschema:"Map of component type to component names"`
}

// GetComponentSchemaInput defines the input parameters for GetComponentSchemaHandler.
type GetComponentSchemaInput struct {
	ComponentType ComponentType `json:"component_type"`
	ComponentName string        `json:"component_name"`
	Version       string        `json:"version,omitempty"`
}

// GetComponentSchemaOutput defines the output schema for the otelcol_get_component_schema tool.
type GetComponentSchemaOutput struct {
	Name    string         `json:"name" jsonschema:"The component name"`
	Type    string         `json:"type" jsonschema:"The component type (receiver, processor, exporter, extension, connector)"`
	Version string         `json:"version" jsonschema:"The OpenTelemetry Collector version"`
	Schema  map[string]any `json:"schema" jsonschema:"The JSON schema for the component configuration"`
}

// ValidateConfigInput defines the input parameters for ValidateConfigHandler.
type ValidateConfigInput struct {
	ComponentType ComponentType `json:"component_type"`
	ComponentName string        `json:"component_name"`
	Config        string        `json:"config"`
	Format        string        `json:"format,omitempty"`
	Version       string        `json:"version,omitempty"`
}

// ValidateConfigOutput defines the output schema for the otelcol_validate_config tool.
type ValidateConfigOutput struct {
	Valid   bool              `json:"valid" jsonschema:"Whether the configuration is valid"`
	Errors  []ValidationError `json:"errors,omitempty" jsonschema:"List of validation errors if invalid"`
	Version string            `json:"version" jsonschema:"The OpenTelemetry Collector version used for validation"`
}

// ValidationError represents a single validation error.
type ValidationError struct {
	Field       string `json:"field" jsonschema:"The field path that has the error"`
	Description string `json:"description" jsonschema:"Description of the validation error"`
	Type        string `json:"type,omitempty" jsonschema:"The error type"`
}

// GetVersionsInput defines the input parameters for GetVersionsHandler.
type GetVersionsInput struct{}

// GetVersionsOutput defines the output schema for the otelcol_get_versions tool.
type GetVersionsOutput struct {
	Versions      []string `json:"versions" jsonschema:"List of available OpenTelemetry Collector versions"`
	LatestVersion string   `json:"latest_version" jsonschema:"The latest available version"`
}
