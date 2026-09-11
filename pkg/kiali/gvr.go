package kiali

import (
	"context"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// HasKiali reports whether a configured Kiali URL responds to GET /api/status.
//
// fp supplies toolset config via ExtendedConfigProvider; kp is the Kubernetes
// provider used for bearer tokens (may be nil in tests).
func HasKiali(ctx context.Context, fp api.FilteringProvider, kp kubernetes.Provider) bool {
	cfg, ok := kialiConfigFromProvider(fp)
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

func kialiConfigFromProvider(fp api.FilteringProvider) (*Config, bool) {
	cfgProvider, ok := fp.(api.ExtendedConfigProvider)
	if !ok || cfgProvider == nil {
		return nil, false
	}
	ext, ok := cfgProvider.GetToolsetConfig("kiali")
	if !ok {
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
