package kiali

import (
	"context"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// HasKiali reports whether a configured Kiali URL responds to GET /api/status.
// When the probe hits a temporary DNS resolution failure, returns true so tools
// remain visible (fail open).
//
// cfg supplies toolset_configs.kiali; kp supplies bearer tokens in production
// (may be nil in tests).
func HasKiali(ctx context.Context, cfg *config.Config, kp kubernetes.Provider) bool {
	kialiCfg, ok := kialiConfigFrom(cfg)
	if !ok || strings.TrimSpace(kialiCfg.Url) == "" {
		return false
	}

	token := bearerTokenFromProvider(ctx, kp)
	result := probeStatusURL(ctx, kialiCfg.Url, kialiCfg, token)
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

func bearerTokenFromProvider(ctx context.Context, kp kubernetes.Provider) string {
	if kp == nil {
		return ""
	}
	k8s, err := kp.GetDerivedKubernetes(ctx, kp.GetDefaultTarget())
	if err != nil || k8s == nil || k8s.RESTConfig() == nil {
		return ""
	}
	return k8s.RESTConfig().BearerToken
}
