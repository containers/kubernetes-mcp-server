package alertmanagement

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"

	"github.com/containers/kubernetes-mcp-server/pkg/obs/auth"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/instrumentation"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/toolcfg"
)

func init() {
	serverconfig.RegisterToolsetConfig(ToolsetName, alertManagementToolsetParser)
}

// Config holds alert management toolset configuration.
type Config struct {
	// AuthMode controls where the bearer token is obtained for authenticating
	// against the monitoring-plugin management API.
	AuthMode auth.AuthMode `toml:"auth_mode,omitempty"`

	// ManagementAPIURL is the base URL of the monitoring-plugin management API.
	ManagementAPIURL string `toml:"management_api_url,omitempty"`

	// ClientMetrics holds HTTP client metrics for instrumenting outbound requests.
	ClientMetrics *instrumentation.ClientMetrics `toml:"-"`
}

var _ serverconfig.ExtendedConfig = (*Config)(nil)

var DefaultConfig = &Config{}

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

func alertManagementToolsetParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (serverconfig.ExtendedConfig, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetConfig retrieves the alert management toolset configuration from params.
func GetConfig(params api.ToolHandlerParams) *Config {
	if cfg, ok := toolcfg.From(params, ToolsetName); ok {
		if amCfg, ok := cfg.(*Config); ok {
			return amCfg
		}
	}
	return DefaultConfig
}
