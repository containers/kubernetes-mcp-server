package kubernetes

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// KubeconfigProviderConfig is the [cluster_provider_configs.kubeconfig] TOML
// section. It enables per-target token exchange decisions for the kubeconfig
// cluster provider based on glob patterns matched against the kubeconfig
// cluster.server URL host.
//
// A target whose server matches any pattern in SkipExchangeServers bypasses
// token exchange (the original bearer token is forwarded to the apiserver).
// All other targets get exchanged using the top-level sts_* settings.
//
// IMPORTANT: For SkipExchangeServers to actually skip exchange, the top-level
// token_exchange_strategy must be left unset. A non-empty top-level strategy
// causes the global STS path to run for skipped targets, defeating the skip.
type KubeconfigProviderConfig struct {
	// TokenExchangeStrategy selects the registered exchanger (e.g. "rfc8693").
	TokenExchangeStrategy string `toml:"token_exchange_strategy,omitempty"`

	// SkipExchangeServers is a list of filepath.Match globs evaluated against
	// the host portion of each context's cluster.server URL. A match means the
	// target's bearer token is forwarded as-is, no exchange.
	SkipExchangeServers []string `toml:"skip_exchange_servers,omitempty"`
}

var _ api.ExtendedConfig = (*KubeconfigProviderConfig)(nil)

func (c *KubeconfigProviderConfig) Validate() error {
	for i, pattern := range c.SkipExchangeServers {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return fmt.Errorf("skip_exchange_servers[%d]: invalid glob %q: %w", i, pattern, err)
		}
	}
	return nil
}

func kubeConfigProviderConfigParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	cfg := &KubeconfigProviderConfig{}
	if err := md.PrimitiveDecode(primitive, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func init() {
	config.RegisterProviderConfig(api.ClusterProviderKubeConfig, kubeConfigProviderConfigParser)
}
