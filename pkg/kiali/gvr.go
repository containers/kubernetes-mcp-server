package kiali

import (
	"context"
	"strings"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

// KialiGVK is the GroupVersionKind for the Kiali custom resource installed by
// the Kiali Operator (kialis.kiali.io CRD).
//
// Target-compatibility filtering enables Kiali tools when:
//   - [toolset_configs.kiali].url is set and GET {url}/api/status succeeds, or
//   - no URL is set, a Kiali CRD/CR path can discover a working in-cluster
//     Service URL via ClusterInspector (or well-known Service DNS when the
//     Kiali GVK is present but no cluster client is available).
var KialiGVK = schema.GroupVersionKind{
	Group:   "kiali.io",
	Version: "v1alpha1",
	Kind:    "Kiali",
}

// discoveredURL holds an in-cluster URL found when toolset_configs.kiali.url
// is empty. NewKiali reads it when the parsed config has no URL.
var discoveredURL atomic.Pointer[string]

func setDiscoveredURL(url string) {
	url = strings.TrimSpace(url)
	if url == "" {
		discoveredURL.Store(nil)
		return
	}
	discoveredURL.Store(&url)
}

func getDiscoveredURL() string {
	if p := discoveredURL.Load(); p != nil {
		return *p
	}
	return ""
}

// HasKiali reports whether Kiali is reachable: either a configured URL passes
// GET /api/status, or an in-cluster Service URL can be discovered and probed.
func HasKiali(ctx context.Context, cfg *config.Config, inspector api.ClusterInspector) bool {
	return evaluateKialiAvailability(ctx, cfg, inspector)
}

func evaluateKialiAvailability(ctx context.Context, cfg *config.Config, inspector api.ClusterInspector) bool {
	kialiCfg, cfgOK := kialiConfigFrom(cfg)
	if cfgOK && strings.TrimSpace(kialiCfg.Url) != "" {
		setDiscoveredURL("")
		token := bearerTokenFromInspector(ctx, inspector)
		ok := probeStatusURL(ctx, kialiCfg.Url, kialiCfg, cfg, token)
		if !ok {
			klogutil.FromContext(ctx).V(1).Info("configured Kiali URL failed /api/status probe; disabling Kiali tools",
				"url", kialiCfg.Url)
		}
		return ok
	}

	if inspector != nil {
		return discoverInjectAndProbe(ctx, cfg, kialiCfg, inspector)
	}

	setDiscoveredURL("")
	return false
}

func discoverInjectAndProbe(ctx context.Context, serverCfg *config.Config, kialiCfg *Config, inspector api.ClusterInspector) bool {
	token := bearerTokenFromInspector(ctx, inspector)
	url, ok := discoverAndValidateInternalURL(ctx, inspector, kialiCfg, serverCfg, token)
	if !ok {
		url, ok = probeCandidateURLs(ctx, wellKnownInternalURLs(), kialiCfg, serverCfg, token)
	}
	if !ok {
		setDiscoveredURL("")
		return false
	}
	storeDiscoveredURL(ctx, url)
	return true
}

func storeDiscoveredURL(ctx context.Context, url string) {
	setDiscoveredURL(url)
	klogutil.FromContext(ctx).V(1).Info("stored discovered Kiali URL for client use", "url", url)
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

func bearerTokenFromInspector(ctx context.Context, inspector api.ClusterInspector) string {
	if inspector == nil {
		return ""
	}
	token, err := inspector.BearerToken(ctx).Default()
	if err != nil {
		return ""
	}
	return token
}

func wellKnownInternalURLs() []string {
	hosts := []string{
		"kiali.istio-system.svc",
		"kiali.kiali.svc",
		"kiali.openshift-operators.svc",
	}
	out := make([]string, 0, len(hosts)*2)
	for _, host := range hosts {
		base := "http://" + host + ":20001"
		out = append(out, base, base+"/kiali")
	}
	return out
}
