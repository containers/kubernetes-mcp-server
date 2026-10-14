package traces

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"

	"github.com/containers/kubernetes-mcp-server/pkg/obs/auth"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/instrumentation"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/openshift"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/toolcfg"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/traces/discovery"
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
	// When true and Resolver is nil, an OpenShift RouteClient is installed
	// automatically during TOML parse and Validate (before handlers run). Prefer
	// setting Resolver directly for custom discovery; UseRoute remains for
	// backward-compatible TOML/flag parsing.
	UseRoute bool `toml:"use_route,omitempty"`

	// Resolver performs cluster-based endpoint discovery (e.g., OpenShift Routes).
	// When nil, plain service DNS is used. Not exposed in TOML; set programmatically
	// or via UseRoute.
	Resolver discovery.EndpointResolver `toml:"-"`

	// ClientMetrics holds HTTP client metrics for instrumenting outbound requests.
	ClientMetrics *instrumentation.ClientMetrics `toml:"-"`
}

var _ serverconfig.ExtendedConfig = (*Config)(nil)

var defaultConfig = &Config{
	UseRoute: false,
}

func (c *Config) Validate() error {
	if c.AuthMode != "" && c.AuthMode != auth.AuthModeHeader && c.AuthMode != auth.AuthModeKubeConfig {
		return fmt.Errorf("invalid auth_mode: %q (valid options: %q, %q)", c.AuthMode, auth.AuthModeHeader, auth.AuthModeKubeConfig)
	}
	// Install resolver at validation time so programmatic Config{UseRoute: true}
	// is ready before concurrent handlers run (no lazy init in getToolsetConfig).
	c.applyUseRouteResolver()
	return nil
}

func (c *Config) GetAuthMode() auth.AuthMode {
	if c.AuthMode == "" {
		return auth.AuthModeHeader
	}
	return c.AuthMode
}

// applyUseRouteResolver installs the OpenShift route resolver when UseRoute is
// set and no Resolver has been provided yet. Call only during config setup
// (parser/Validate), not from concurrent request handlers.
func (c *Config) applyUseRouteResolver() {
	if c == nil || !c.UseRoute || c.Resolver != nil {
		return
	}
	c.Resolver = &openshift.RouteClient{}
}

func tempoToolsetParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (serverconfig.ExtendedConfig, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	cfg.applyUseRouteResolver()
	return &cfg, nil
}

func getToolsetConfig(params api.ToolHandlerParams) *Config {
	if cfg, ok := toolcfg.From(params, ToolsetName); ok {
		if tempoCfg, ok := cfg.(*Config); ok {
			return tempoCfg
		}
	}

	// Return default config if not found
	return defaultConfig
}
