package traces

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"

	toolsetconfig "github.com/rhobs/obs-mcp/pkg/toolset/config"
)

func init() {
	serverconfig.RegisterToolsetConfig(ToolsetName, tempoToolsetParser)
}

type Config struct {
	// AuthMode controls where the bearer token is obtained for authenticating against Tempo endpoints.
	// Valid values: "header" (default), "kubeconfig".
	AuthMode toolsetconfig.AuthMode `toml:"auth_mode,omitempty"`

	// Insecure controls whether to skip TLS certificate verification.
	Insecure bool `toml:"insecure,omitempty"`

	// UseRoute controls whether to use OpenShift Routes for discovering Tempo endpoints.
	UseRoute bool `toml:"useRoute,omitempty"`
}

var _ api.ExtendedConfig = (*Config)(nil)

var DefaultConfig = &Config{
	UseRoute: false,
}

func (c *Config) Validate() error {
	if c.AuthMode != "" && c.AuthMode != toolsetconfig.AuthModeHeader && c.AuthMode != toolsetconfig.AuthModeKubeConfig {
		return fmt.Errorf("invalid auth_mode: %q (valid options: %q, %q)", c.AuthMode, toolsetconfig.AuthModeHeader, toolsetconfig.AuthModeKubeConfig)
	}
	return nil
}

func (c *Config) GetAuthMode() toolsetconfig.AuthMode {
	if c.AuthMode == "" {
		return toolsetconfig.AuthModeHeader
	}
	return c.AuthMode
}

func tempoToolsetParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func GetConfig(params api.ToolHandlerParams) *Config {
	if cfg, ok := params.GetToolsetConfig(ToolsetName); ok {
		if tempoCfg, ok := cfg.(*Config); ok {
			return tempoCfg
		}
	}

	// Return default config if not found
	return DefaultConfig
}
