package otelcol

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/os-observability/redhat-opentelemetry-collector/configschemas"
)

const ToolsetName = "observability/otelcol"

// Config holds OpenTelemetry Collector toolset configuration.
type Config struct {
	// SchemaFS is an embedded filesystem containing component schemas.
	// Expected structure: schemas/0.143.0/receivers/..., schemas/0.144.0/...
	SchemaFS fs.FS
}

var _ api.ExtendedConfig = (*Config)(nil)

// NewDefaultConfig returns the default otelcol configuration using the
// embedded schemas from redhat-opentelemetry-collector.
func NewDefaultConfig() *Config {
	return &Config{
		SchemaFS: configschemas.Schemas,
	}
}

// Validate checks that the configuration values are valid.
func (c *Config) Validate() error {
	if c.SchemaFS == nil {
		return fmt.Errorf("SchemaFS is required in otelcol config")
	}
	return nil
}

// Toolset implements the OpenTelemetry Collector toolset.
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

// GetName returns the name of the toolset.
func (t *Toolset) GetName() string {
	return ToolsetName
}

// GetDescription returns a human-readable description of the toolset.
func (t *Toolset) GetDescription() string {
	return "Toolset for OpenTelemetry Collector configuration assistance including schema validation, component documentation, and version management."
}

// GetTools returns all tools provided by this toolset.
func (t *Toolset) GetTools(_ api.FilteringProvider) []api.ServerTool {
	return []api.ServerTool{
		initListComponents(),
		initGetComponentSchema(),
		initValidateConfig(),
		initGetVersions(),
	}
}

// GetPrompts returns prompts provided by this toolset.
func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return nil
}

// GetResources returns resources provided by this toolset.
func (t *Toolset) GetResources() []api.ServerResource {
	return nil
}

// GetResourceTemplates returns resource templates provided by this toolset.
func (t *Toolset) GetResourceTemplates() []api.ServerResourceTemplate {
	return nil
}

// getConfig retrieves the otelcol toolset configuration from params.
func getConfig(params api.ToolHandlerParams) *Config {
	if cfg, ok := params.GetToolsetConfig(ToolsetName); ok {
		if otelcolCfg, ok := cfg.(*Config); ok {
			return otelcolCfg
		}
	}
	return NewDefaultConfig()
}

// getSchemaLoader creates a SchemaLoader using the toolset configuration.
func getSchemaLoader(config *Config) (SchemaLoader, error) {
	if config.SchemaFS == nil {
		return nil, fmt.Errorf("SchemaFS is required in otelcol config")
	}
	return NewSchemaLoaderFromFS(config.SchemaFS, "schemas"), nil
}

func otelColToolsetParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	cfg := *NewDefaultConfig()
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func init() {
	serverconfig.RegisterToolsetConfig(ToolsetName, otelColToolsetParser)
}
