package kiali

import (
	"context"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

// HasKiali reports whether a configured Kiali URL responds to GET /api/status.
// When the probe hits a temporary DNS resolution failure, returns true so tools
// remain visible (fail open).
//
// cfg supplies toolset_configs.kiali; bearerToken is sent on the status probe
// (empty in tests or when unavailable).
func HasKiali(ctx context.Context, cfg *config.Config, bearerToken string) bool {
	kialiCfg, ok := kialiConfigFrom(cfg)
	if !ok || strings.TrimSpace(kialiCfg.Url) == "" {
		return false
	}

	result := probeStatusURL(ctx, kialiCfg.Url, kialiCfg, bearerToken)
	if result == nil {
		return true
	}
	if !*result {
		klogutil.FromContext(ctx).V(1).Info("configured Kiali URL failed /api/status probe; disabling Kiali tools",
			"url", kialiCfg.Url)
	}
	return *result
}

func kialiConfigFrom(cfg *config.Config) (*Config, bool) {
	if cfg == nil {
		return nil, false
	}
	return kialiConfigFromExtended(cfg.GetToolsetConfig("kiali"))
}

func kialiConfigFromExtended(ext config.ExtendedConfig, ok bool) (*Config, bool) {
	if !ok || ext == nil {
		return nil, false
	}
	kc, ok := ext.(*Config)
	if !ok || kc == nil {
		return nil, false
	}
	return kc, true
}
