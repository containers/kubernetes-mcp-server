package otelcol

import (
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/pavolloffay/opentelemetry-mcp-server/modules/schemagen"
)

// normalizeVersion removes the leading "v" prefix from version strings if present.
// This allows users to specify versions as "v0.144.0" or "0.144.0" interchangeably.
func normalizeVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}

// SchemaLoader defines the interface for loading OpenTelemetry Collector schemas.
type SchemaLoader interface {
	GetComponentSchema(componentType schemagen.ComponentType, componentName string, version string) (*schemagen.ComponentSchema, error)
	GetComponentSchemaJSON(componentType schemagen.ComponentType, componentName string, version string) ([]byte, error)
	ListAvailableComponents(version string) (map[schemagen.ComponentType][]string, error)
	ValidateComponentYAML(componentType schemagen.ComponentType, componentName string, version string, yamlData []byte) (*ValidationResult, error)
	ValidateComponentJSON(componentType schemagen.ComponentType, componentName string, version string, jsonData []byte) (*ValidationResult, error)
	GetLatestVersion() (string, error)
	GetAllVersions() ([]string, error)
}

// ValidationResult wraps the validation result from JSON schema validation.
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// schemaManagerWrapper wraps schemagen.SchemaManager to implement SchemaLoader.
type schemaManagerWrapper struct {
	manager *schemagen.SchemaManager
}

// NewSchemaLoaderFromFS creates a new SchemaLoader using schemas from the provided filesystem.
// This allows using an embed.FS or any other fs.FS implementation.
// The basePath should be the path within the filesystem where version subdirectories are located.
func NewSchemaLoaderFromFS(filesystem fs.FS, basePath string) SchemaLoader {
	return &schemaManagerWrapper{
		manager: schemagen.NewSchemaManagerFromFS(filesystem, basePath),
	}
}

func (w *schemaManagerWrapper) GetComponentSchema(componentType schemagen.ComponentType, componentName, version string) (*schemagen.ComponentSchema, error) {
	return w.manager.GetComponentSchema(componentType, componentName, version)
}

func (w *schemaManagerWrapper) GetComponentSchemaJSON(componentType schemagen.ComponentType, componentName, version string) ([]byte, error) {
	return w.manager.GetComponentSchemaJSON(componentType, componentName, version)
}

func (w *schemaManagerWrapper) ListAvailableComponents(version string) (map[schemagen.ComponentType][]string, error) {
	return w.manager.ListAvailableComponents(version)
}

func (w *schemaManagerWrapper) ValidateComponentYAML(componentType schemagen.ComponentType, componentName, version string, yamlData []byte) (*ValidationResult, error) {
	result, err := w.manager.ValidateComponentYAML(componentType, componentName, version, yamlData)
	if err != nil {
		return nil, err
	}
	validationResult := &ValidationResult{
		Valid:  result.Valid(),
		Errors: make([]ValidationError, 0),
	}
	for _, e := range result.Errors() {
		validationResult.Errors = append(validationResult.Errors, ValidationError{
			Field:       e.Field(),
			Description: e.Description(),
			Type:        e.Type(),
		})
	}
	return validationResult, nil
}

func (w *schemaManagerWrapper) ValidateComponentJSON(componentType schemagen.ComponentType, componentName, version string, jsonData []byte) (*ValidationResult, error) {
	result, err := w.manager.ValidateComponentJSON(componentType, componentName, version, jsonData)
	if err != nil {
		return nil, err
	}
	validationResult := &ValidationResult{
		Valid:  result.Valid(),
		Errors: make([]ValidationError, 0),
	}
	for _, e := range result.Errors() {
		validationResult.Errors = append(validationResult.Errors, ValidationError{
			Field:       e.Field(),
			Description: e.Description(),
			Type:        e.Type(),
		})
	}
	return validationResult, nil
}

func (w *schemaManagerWrapper) GetLatestVersion() (string, error) {
	return w.manager.GetLatestVersion()
}

func (w *schemaManagerWrapper) GetAllVersions() ([]string, error) {
	return w.manager.GetAllVersions()
}

// Handler implementations

func ListComponentsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	version := p.OptionalString("version", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list components: %w", err)), nil
	}

	loader, err := getSchemaLoader(getConfig(params))
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	slog.Info("ListComponentsHandler called")

	version = normalizeVersion(version)
	if version == "" {
		version, err = loader.GetLatestVersion()
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to get latest version: %w", err)), nil
		}
	}

	components, err := loader.ListAvailableComponents(version)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list components: %w", err)), nil
	}

	output := ListComponentsOutput{
		Version:    version,
		Receivers:  components[schemagen.ComponentTypeReceiver],
		Processors: components[schemagen.ComponentTypeProcessor],
		Exporters:  components[schemagen.ComponentTypeExporter],
		Extensions: components[schemagen.ComponentTypeExtension],
		Connectors: components[schemagen.ComponentTypeConnector],
		Components: make(map[string][]string),
	}

	for k, v := range components {
		output.Components[string(k)] = v
	}

	slog.Info("ListComponentsHandler executed successfully",
		"receivers", len(output.Receivers),
		"processors", len(output.Processors),
		"exporters", len(output.Exporters))

	return api.NewToolCallResultStructured(output, nil), nil
}

func GetComponentSchemaHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	componentType := p.RequiredString("component_type")
	componentName := p.RequiredString("component_name")
	version := p.OptionalString("version", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get component schema: %w", err)), nil
	}

	loader, err := getSchemaLoader(getConfig(params))
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	slog.Info("GetComponentSchemaHandler called")

	ct := ComponentType(componentType)
	if !ct.IsValid() {
		return api.NewToolCallResult("", fmt.Errorf("invalid component_type: %s, must be one of: receiver, processor, exporter, extension, connector", ct)), nil
	}
	if componentName == "" {
		return api.NewToolCallResult("", fmt.Errorf("component_name is required")), nil
	}

	version = normalizeVersion(version)
	if version == "" {
		version, err = loader.GetLatestVersion()
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to get latest version: %w", err)), nil
		}
	}

	schema, err := loader.GetComponentSchema(schemagen.ComponentType(ct), componentName, version)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get component schema: %w", err)), nil
	}

	output := GetComponentSchemaOutput{
		Name:    schema.Name,
		Type:    string(schema.Type),
		Version: schema.Version,
		Schema:  schema.Schema,
	}

	slog.Info("GetComponentSchemaHandler executed successfully", "component", componentName)
	return api.NewToolCallResultStructured(output, nil), nil
}

func ValidateConfigHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	componentType := p.RequiredString("component_type")
	componentName := p.RequiredString("component_name")
	config := p.RequiredString("config")
	format := p.OptionalString("format", "yaml")
	version := p.OptionalString("version", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to validate config: %w", err)), nil
	}

	loader, err := getSchemaLoader(getConfig(params))
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	slog.Info("ValidateConfigHandler called")
	slog.Debug("ValidateConfigHandler params", "componentType", componentType, "componentName", componentName)

	ct := ComponentType(componentType)
	if !ct.IsValid() {
		return api.NewToolCallResult("", fmt.Errorf("invalid component_type: %s, must be one of: receiver, processor, exporter, extension, connector", ct)), nil
	}
	if componentName == "" {
		return api.NewToolCallResult("", fmt.Errorf("component_name is required")), nil
	}
	if config == "" {
		return api.NewToolCallResult("", fmt.Errorf("config is required")), nil
	}

	version = normalizeVersion(version)
	if version == "" {
		version, err = loader.GetLatestVersion()
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to get latest version: %w", err)), nil
		}
	}

	configData := []byte(config)

	var result *ValidationResult

	if format == "" {
		format = "yaml"
	}

	switch format {
	case "yaml":
		result, err = loader.ValidateComponentYAML(schemagen.ComponentType(ct), componentName, version, configData)
	case "json":
		result, err = loader.ValidateComponentJSON(schemagen.ComponentType(ct), componentName, version, configData)
	default:
		return api.NewToolCallResult("", fmt.Errorf("invalid format: %s, must be 'yaml' or 'json'", format)), nil
	}

	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to validate config: %w", err)), nil
	}

	output := ValidateConfigOutput{
		Valid:   result.Valid,
		Errors:  result.Errors,
		Version: version,
	}

	slog.Info("ValidateConfigHandler executed successfully", "valid", output.Valid, "errorCount", len(output.Errors))
	return api.NewToolCallResultStructured(output, nil), nil
}

func GetVersionsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	loader, err := getSchemaLoader(getConfig(params))
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	slog.Info("GetVersionsHandler called")

	versions, err := loader.GetAllVersions()
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get versions: %w", err)), nil
	}

	latestVersion, err := loader.GetLatestVersion()
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get latest version: %w", err)), nil
	}

	output := GetVersionsOutput{
		Versions:      versions,
		LatestVersion: latestVersion,
	}

	slog.Info("GetVersionsHandler executed successfully", "versionCount", len(output.Versions))
	return api.NewToolCallResultStructured(output, nil), nil
}
