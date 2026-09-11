package kiali

import (
	"context"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// toolsetConfigProvider is an optional test/helper surface that exposes
// GetToolsetConfig. Production code obtains Kiali config from the derived
// Kubernetes client's live *config.Config instead of widening shared providers.
type toolsetConfigProvider interface {
	GetToolsetConfig(name string) (config.ExtendedConfig, bool)
}

// HasKiali reports whether a configured Kiali URL responds to GET /api/status.
//
// fp may stub GetToolsetConfig in tests; kp supplies live config and bearer
// tokens in production (may be nil in tests).
func HasKiali(ctx context.Context, fp api.FilteringProvider, kp kubernetes.Provider) bool {
	cfg, ok := kialiConfigFrom(ctx, fp, kp)
	if !ok || strings.TrimSpace(cfg.Url) == "" {
		return false
	}

	token := bearerTokenFromProvider(ctx, kp)
	ok = probeStatusURL(ctx, cfg.Url, cfg, token)
	if !ok {
		klogutil.FromContext(ctx).V(1).Info("configured Kiali URL failed /api/status probe; disabling Kiali tools",
			"url", cfg.Url)
	}
	return ok
}

func kialiConfigFrom(ctx context.Context, fp api.FilteringProvider, kp kubernetes.Provider) (*Config, bool) {
	// Unit tests may stub GetToolsetConfig on the FilteringProvider.
	if cfgProvider, ok := fp.(toolsetConfigProvider); ok && cfgProvider != nil {
		if kc, ok := kialiConfigFromExtended(cfgProvider.GetToolsetConfig("kiali")); ok {
			return kc, true
		}
	}
	// Production: use the live config already attached to the derived client.
	if kp == nil {
		return nil, false
	}
	k8s, err := kp.GetDerivedKubernetes(ctx, kp.GetDefaultTarget())
	if err != nil || k8s == nil {
		return nil, false
	}
	cfg := k8s.Config()
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
