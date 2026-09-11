package kubernetes

import (
	"context"
	"fmt"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

// KubeConfigProviderConfig holds per-target token exchange configuration for
// the kubeconfig cluster provider. Registered via cluster_provider_configs:
//
//	[cluster_provider_configs.kubeconfig]
//	strategy = "keycloak-v2"
//
//	[cluster_provider_configs.kubeconfig.targets.spoke-context]
//	audience = "spoke-cluster"
//	scopes = ["mcp:spoke"]
type KubeConfigProviderConfig struct {
	Strategy string                            `toml:"strategy"`
	Targets  map[string]KubeConfigTargetConfig `toml:"targets"`
}

// KubeConfigTargetConfig holds per-target token exchange overrides. Fields that
// are empty fall back to the global token exchange config on BaseConfig.
type KubeConfigTargetConfig struct {
	Audience     string   `toml:"audience"`
	Scopes       []string `toml:"scopes,omitempty"`
	ClientId     string   `toml:"client_id,omitempty"`
	ClientSecret string   `toml:"client_secret,omitempty"`
}

var _ api.ExtendedConfig = (*KubeConfigProviderConfig)(nil)

func (c *KubeConfigProviderConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("kubeconfig provider config is nil")
	}
	if c.Strategy != "" {
		strategies := tokenexchange.GetRegisteredStrategies()
		if !slices.Contains(strategies, c.Strategy) {
			return fmt.Errorf("invalid strategy %q, valid values: %v", c.Strategy, strategies)
		}
	}
	if c.Strategy == "" && len(c.Targets) > 0 {
		return fmt.Errorf("strategy is required when targets are configured")
	}
	for name, target := range c.Targets {
		if target.Audience == "" {
			return fmt.Errorf("target %q: audience is required", name)
		}
	}
	return nil
}

func kubeconfigProviderConfigParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	var cfg KubeConfigProviderConfig
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func init() {
	config.RegisterProviderConfig(api.ClusterProviderKubeConfig, kubeconfigProviderConfigParser)
}
