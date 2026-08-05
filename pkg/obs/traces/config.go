package traces

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"

	"github.com/rhobs/obs-mcp/pkg/auth"
	"github.com/rhobs/obs-mcp/pkg/instrumentation"
	"github.com/rhobs/obs-mcp/pkg/traces/discovery"
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
	//
	// Deprecated: set Resolver instead. UseRoute is kept for backward-compatible TOML/flag parsing;
	// main.go translates it into a Resolver at startup.
	UseRoute bool `toml:"use_route,omitempty"`

	// Resolver performs cluster-based endpoint discovery (e.g., OpenShift Routes).
	// When nil, plain service DNS is used. Not exposed in TOML; set programmatically.
	Resolver discovery.EndpointResolver `toml:"-"`

	// ClientMetrics holds HTTP client metrics for instrumenting outbound requests.
	ClientMetrics *instrumentation.ClientMetrics `toml:"-"`
}

var _ api.ExtendedConfig = (*Config)(nil)

var defaultConfig = &Config{
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

func getToolsetConfig(params api.ToolHandlerParams) *Config {
	if cfg, ok := params.GetToolsetConfig(ToolsetName); ok {
		if tempoCfg, ok := cfg.(*Config); ok {
			return tempoCfg
		}
	}

	// Return default config if not found
	return defaultConfig
}
