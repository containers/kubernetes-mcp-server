package traces

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"

	"github.com/rhobs/obs-mcp/pkg/auth"
)

func init() {
	serverconfig.RegisterToolsetConfig(ToolsetName, tempoToolsetParser)
}

type Config struct {
	// AuthMode controls where the bearer token is obtained for authenticating against Tempo endpoints.
	// Valid values: "header" (default), "kubeconfig".
	AuthMode auth.AuthMode `toml:"auth_mode,omitempty"`

	// Insecure controls whether to skip TLS certificate verification.
	Insecure bool `toml:"insecure,omitempty"`

	// TempoURL is the URL of the Tempo API endpoint.
	// When set, it is used directly instead of discovering Tempo instances via Kubernetes.
	TempoURL string `toml:"tempo_url,omitempty"`

	// UseRoute controls whether to use OpenShift Routes for discovering Tempo endpoints.
	UseRoute bool `toml:"use_route,omitempty"`
}

var _ api.ExtendedConfig = (*Config)(nil)

var DefaultConfig = &Config{
	UseRoute: false,
}

func (c *Config) Validate() error {
	if c.AuthMode != "" && c.AuthMode != auth.AuthModeHeader && c.AuthMode != auth.AuthModeKubeConfig {
		return fmt.Errorf("invalid auth_mode: %q (valid options: %q, %q)", c.AuthMode, auth.AuthModeHeader, auth.AuthModeKubeConfig)
	}
	return nil
}

func (c *Config) GetAuthMode() auth.AuthMode {
	if c.AuthMode == "" {
		return auth.AuthModeHeader
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
