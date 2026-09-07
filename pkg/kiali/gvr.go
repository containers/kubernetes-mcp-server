package kiali

import (
	"context"
	"strings"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// KialiGVK is the GroupVersionKind for the Kiali custom resource installed by
// the Kiali Operator (kialis.kiali.io CRD).
//
// Target-compatibility filtering enables Kiali tools when:
//   - [toolset_configs.kiali].url is set and GET {url}/api/status succeeds, or
//   - no URL is set, a Kiali CRD/CR path can discover a working in-cluster
//     Service URL via kubernetes.Provider (or well-known Service DNS when the
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
//
// fp supplies config and optional GVK discovery; kp is the Kubernetes provider
// used for CR listing and bearer tokens (may be nil in tests).
func HasKiali(ctx context.Context, fp api.FilteringProvider, kp kubernetes.Provider) bool {
	return evaluateKialiAvailability(ctx, fp, kp)
}

func evaluateKialiAvailability(ctx context.Context, fp api.FilteringProvider, kp kubernetes.Provider) bool {
	cfg, cfgOK := kialiConfigFromProvider(fp)
	if cfgOK && strings.TrimSpace(cfg.Url) != "" {
		setDiscoveredURL("")
		token := bearerTokenFromProvider(ctx, kp)
		ok := probeStatusURL(ctx, cfg.Url, cfg, token)
		if !ok {
			klogutil.FromContext(ctx).V(1).Info("configured Kiali URL failed /api/status probe; disabling Kiali tools",
				"url", cfg.Url)
		}
		return ok
	}

	// No configured URL: discover + probe an in-cluster Service URL.
	if kp != nil {
		return discoverInjectAndProbe(ctx, cfg, kp)
	}

	// No kubernetes provider (typical in unit tests): fall back to GVK presence +
	// well-known Service DNS candidates.
	if fp == nil || !fp.AnyTargetHasGVKs(ctx, []schema.GroupVersionKind{KialiGVK}) {
		setDiscoveredURL("")
		return false
	}
	token := bearerTokenFromProvider(ctx, nil)
	url, ok := probeCandidateURLs(ctx, wellKnownInternalURLs(), cfg, token)
	if !ok {
		klogutil.FromContext(ctx).V(1).Info("Kiali GVK present but no reachable well-known in-cluster URL")
		setDiscoveredURL("")
		return false
	}
	storeDiscoveredURL(ctx, url)
	return true
}

func discoverInjectAndProbe(ctx context.Context, cfg *Config, kp kubernetes.Provider) bool {
	k8s, err := kp.GetDerivedKubernetes(ctx, kp.GetDefaultTarget())
	if err != nil || k8s == nil {
		klogutil.FromContext(ctx).V(2).Info("Kiali discovery skipped: cannot derive Kubernetes client", "error", err)
		setDiscoveredURL("")
		return false
	}
	token := ""
	if rc := k8s.RESTConfig(); rc != nil {
		token = rc.BearerToken
	}
	url, ok := discoverAndValidateInternalURL(ctx, k8s.DynamicClient(), cfg, token)
	if !ok {
		// CR list failed or unreachable — still try well-known Service DNS.
		url, ok = probeCandidateURLs(ctx, wellKnownInternalURLs(), cfg, token)
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

// wellKnownInternalURLs are tried when a Kiali CR cannot be listed through the
// kubernetes provider but the Kiali GVK is present.
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
