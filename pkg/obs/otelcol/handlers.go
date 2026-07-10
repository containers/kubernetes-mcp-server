package otelcol

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pavolloffay/opentelemetry-mcp-server/modules/schemagen"

	"github.com/rhobs/obs-mcp/pkg/resultutil"
	"github.com/rhobs/obs-mcp/pkg/tools"
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

// Helper functions for parameter extraction

// BuildListComponentsInput builds input from handler arguments.
func BuildListComponentsInput(args map[string]any) ListComponentsInput {
	return ListComponentsInput{
		Version: tools.GetString(args, "version", ""),
	}
}

// BuildGetComponentSchemaInput builds input from handler arguments.
func BuildGetComponentSchemaInput(args map[string]any) GetComponentSchemaInput {
	return GetComponentSchemaInput{
		ComponentType: ComponentType(tools.GetString(args, "component_type", "")),
		ComponentName: tools.GetString(args, "component_name", ""),
		Version:       tools.GetString(args, "version", ""),
	}
}

// BuildValidateConfigInput builds input from handler arguments.
func BuildValidateConfigInput(args map[string]any) ValidateConfigInput {
	return ValidateConfigInput{
		ComponentType: ComponentType(tools.GetString(args, "component_type", "")),
		ComponentName: tools.GetString(args, "component_name", ""),
		Config:        tools.GetString(args, "config", ""),
		Format:        tools.GetString(args, "format", "yaml"),
		Version:       tools.GetString(args, "version", ""),
	}
}

// BuildGetVersionsInput builds input from handler arguments.
func BuildGetVersionsInput(_ map[string]any) GetVersionsInput {
	return GetVersionsInput{}
}

// ToMCPHandler converts a typed handler function to an MCP tool handler.
// This allows the otelcol handlers to be used directly with the MCP server.
func ToMCPHandler[I, O any](
	config *Config,
	buildInput func(map[string]any) I,
	handler func(context.Context, SchemaLoader, I) *resultutil.Result,
) mcp.ToolHandlerFor[map[string]any, O] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, O, error) {
		var zero O
		loader, err := getSchemaLoaderFromConfig(config)
		if err != nil {
			return nil, zero, err
		}
		input := buildInput(args)
		result := handler(ctx, loader, input)
		output, err := resultutil.Unwrap[O](result)
		if err != nil {
			return nil, zero, err
		}
		return nil, output, nil
	}
}

// getSchemaLoaderFromConfig creates a SchemaLoader from the Config.
func getSchemaLoaderFromConfig(config *Config) (SchemaLoader, error) {
	if config == nil || config.SchemaFS == nil {
		return nil, fmt.Errorf("SchemaFS is required in otelcol config")
	}
	return NewSchemaLoaderFromFS(config.SchemaFS, "schemas"), nil
}

// Handler implementations

// ListComponentsHandler handles the listing of available components.
func ListComponentsHandler(ctx context.Context, loader SchemaLoader, input ListComponentsInput) *resultutil.Result {
	slog.Info("ListComponentsHandler called")
	slog.Debug("ListComponentsHandler params", "input", input)

	version := normalizeVersion(input.Version)
	if version == "" {
		var err error
		version, err = loader.GetLatestVersion()
		if err != nil {
			return resultutil.NewErrorResult(fmt.Errorf("failed to get latest version: %w", err))
		}
	}

	components, err := loader.ListAvailableComponents(version)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to list components: %w", err))
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

	return resultutil.NewSuccessResult(output)
}

// GetComponentSchemaHandler handles getting a component's schema.
func GetComponentSchemaHandler(ctx context.Context, loader SchemaLoader, input GetComponentSchemaInput) *resultutil.Result {
	slog.Info("GetComponentSchemaHandler called")
	slog.Debug("GetComponentSchemaHandler params", "input", input)

	if !input.ComponentType.IsValid() {
		return resultutil.NewErrorResult(fmt.Errorf("invalid component_type: %s, must be one of: receiver, processor, exporter, extension, connector", input.ComponentType))
	}
	if input.ComponentName == "" {
		return resultutil.NewErrorResult(fmt.Errorf("component_name is required"))
	}

	version := normalizeVersion(input.Version)
	if version == "" {
		var err error
		version, err = loader.GetLatestVersion()
		if err != nil {
			return resultutil.NewErrorResult(fmt.Errorf("failed to get latest version: %w", err))
		}
	}

	componentType := schemagen.ComponentType(input.ComponentType)
	schema, err := loader.GetComponentSchema(componentType, input.ComponentName, version)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to get component schema: %w", err))
	}

	output := GetComponentSchemaOutput{
		Name:    schema.Name,
		Type:    string(schema.Type),
		Version: schema.Version,
		Schema:  schema.Schema,
	}

	slog.Info("GetComponentSchemaHandler executed successfully", "component", input.ComponentName)
	return resultutil.NewSuccessResult(output)
}

// ValidateConfigHandler handles validating a component configuration.
func ValidateConfigHandler(ctx context.Context, loader SchemaLoader, input ValidateConfigInput) *resultutil.Result {
	slog.Info("ValidateConfigHandler called")
	slog.Debug("ValidateConfigHandler params", "componentType", input.ComponentType, "componentName", input.ComponentName)

	if !input.ComponentType.IsValid() {
		return resultutil.NewErrorResult(fmt.Errorf("invalid component_type: %s, must be one of: receiver, processor, exporter, extension, connector", input.ComponentType))
	}
	if input.ComponentName == "" {
		return resultutil.NewErrorResult(fmt.Errorf("component_name is required"))
	}
	if input.Config == "" {
		return resultutil.NewErrorResult(fmt.Errorf("config is required"))
	}

	version := normalizeVersion(input.Version)
	if version == "" {
		var err error
		version, err = loader.GetLatestVersion()
		if err != nil {
			return resultutil.NewErrorResult(fmt.Errorf("failed to get latest version: %w", err))
		}
	}

	componentType := schemagen.ComponentType(input.ComponentType)
	configData := []byte(input.Config)

	var result *ValidationResult
	var err error

	format := input.Format
	if format == "" {
		format = "yaml"
	}

	switch format {
	case "yaml":
		result, err = loader.ValidateComponentYAML(componentType, input.ComponentName, version, configData)
	case "json":
		result, err = loader.ValidateComponentJSON(componentType, input.ComponentName, version, configData)
	default:
		return resultutil.NewErrorResult(fmt.Errorf("invalid format: %s, must be 'yaml' or 'json'", format))
	}

	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to validate config: %w", err))
	}

	output := ValidateConfigOutput{
		Valid:   result.Valid,
		Errors:  result.Errors,
		Version: version,
	}

	slog.Info("ValidateConfigHandler executed successfully", "valid", output.Valid, "errorCount", len(output.Errors))
	return resultutil.NewSuccessResult(output)
}

// GetVersionsHandler handles listing available versions.
func GetVersionsHandler(ctx context.Context, loader SchemaLoader, input GetVersionsInput) *resultutil.Result {
	slog.Info("GetVersionsHandler called")

	versions, err := loader.GetAllVersions()
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to get versions: %w", err))
	}

	latestVersion, err := loader.GetLatestVersion()
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to get latest version: %w", err))
	}

	output := GetVersionsOutput{
		Versions:      versions,
		LatestVersion: latestVersion,
	}

	slog.Info("GetVersionsHandler executed successfully", "versionCount", len(output.Versions))
	return resultutil.NewSuccessResult(output)
}
