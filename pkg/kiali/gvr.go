package kiali

import (
	"context"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"k8s.io/client-go/rest"
)

// HasKiali reports whether a configured Kiali URL responds to GET /api/status.
// When the probe hits a temporary DNS resolution failure, returns true so tools
// remain visible (fail open).
func HasKiali(ctx context.Context, cfg api.BaseConfig, bearerToken string) bool {
	kc, ok := kialiConfigFromBase(cfg)
	if !ok || strings.TrimSpace(kc.Url) == "" {
		return false
	}

	result := probeStatusURL(ctx, cfg, restConfigFromBearerToken(bearerToken))
	if result == nil {
		return true
	}
	if !*result {
		klogutil.FromContext(ctx).V(1).Info("configured Kiali URL failed /api/status probe; disabling Kiali tools",
			"url", kc.Url)
	}
	return *result
}

func kialiConfigFromBase(cfg api.BaseConfig) (*Config, bool) {
	if cfg == nil {
		return nil, false
	}
	ext, ok := cfg.GetToolsetConfig("kiali")
	if !ok {
		return nil, false
	}
	kc, ok := ext.(*Config)
	if !ok || kc == nil {
		return nil, false
	}
	return kc, true
}

func restConfigFromBearerToken(bearerToken string) *rest.Config {
	if strings.TrimSpace(bearerToken) == "" {
		return &rest.Config{}
	}
	return &rest.Config{BearerToken: bearerToken}
}
