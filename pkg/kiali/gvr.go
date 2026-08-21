package kiali

import (
	"context"
	"strings"
	"sync"
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
//     Service URL (via kubernetes.Provider when available, otherwise well-known
//     Service DNS candidates after the Kiali GVK is detected).
//
// URL / config checks use api.ExtendedConfigProvider on the FilteringProvider
// (kubernetes.Provider embeds api.BaseConfig). Cluster listing for CRs is
// optional and only used when the provider also implements
// kubernetes.Provider — no generic MCP changes are required for the URL-probe path.
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

// HasKiali returns a TargetCompatibilityFilter that is true when Kiali is
// reachable: either a configured URL passes GET /api/status, or an in-cluster
// Service URL can be discovered and probed.
//
// The returned closure memoizes the result (sync.Once). Callers that register
// the filter on many tools should share one closure (see Toolset.GetTools).
func HasKiali(p api.FilteringProvider) func() bool {
	var once sync.Once
	var available bool
	return func() bool {
		once.Do(func() {
			available = evaluateKialiAvailability(context.TODO(), p)
		})
		return available
	}
}

func evaluateKialiAvailability(ctx context.Context, p api.FilteringProvider) bool {
	cfg, cfgOK := kialiConfigFromProvider(p)
	if cfgOK && strings.TrimSpace(cfg.Url) != "" {
		setDiscoveredURL("")
		token := bearerTokenFromProvider(ctx, p)
		ok := probeStatusURL(ctx, cfg.Url, cfg, token)
		if !ok {
			klogutil.FromContext(ctx).V(1).Info("configured Kiali URL failed /api/status probe; disabling Kiali tools",
				"url", cfg.Url)
		}
		return ok
	}

	// No configured URL: discover + probe an in-cluster Service URL.
	if kp, ok := p.(kubernetes.Provider); ok {
		return discoverInjectAndProbe(ctx, cfg, cfgOK, kp)
	}

	// FilteringProvider wrappers may not expose kubernetes.Provider. Fall back to
	// GVK presence + well-known Service DNS candidates (no generic MCP API change).
	if p == nil || !p.AnyTargetHasGVKs(ctx, []schema.GroupVersionKind{KialiGVK}) {
		setDiscoveredURL("")
		return false
	}
	token := bearerTokenFromProvider(ctx, p)
	url, ok := probeCandidateURLs(ctx, wellKnownInternalURLs(), cfg, token)
	if !ok {
		klogutil.FromContext(ctx).V(1).Info("Kiali GVK present but no reachable well-known in-cluster URL")
		setDiscoveredURL("")
		return false
	}
	injectDiscoveredURL(cfg, cfgOK, url)
	return true
}

func discoverInjectAndProbe(ctx context.Context, cfg *Config, cfgOK bool, kp kubernetes.Provider) bool {
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
	injectDiscoveredURL(cfg, cfgOK, url)
	return true
}

func injectDiscoveredURL(cfg *Config, cfgOK bool, url string) {
	setDiscoveredURL(url)
	if cfgOK && cfg != nil {
		cfg.Url = url
		klogutil.FromContext(context.TODO()).V(1).Info("injected discovered Kiali URL into toolset config", "url", url)
		return
	}
	klogutil.FromContext(context.TODO()).V(1).Info("stored discovered Kiali URL for client use", "url", url)
}

func kialiConfigFromProvider(p api.FilteringProvider) (*Config, bool) {
	cfgProvider, ok := p.(api.ExtendedConfigProvider)
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

func bearerTokenFromProvider(ctx context.Context, p api.FilteringProvider) string {
	kp, ok := p.(kubernetes.Provider)
	if !ok {
		return ""
	}
	k8s, err := kp.GetDerivedKubernetes(ctx, kp.GetDefaultTarget())
	if err != nil || k8s == nil || k8s.RESTConfig() == nil {
		return ""
	}
	return k8s.RESTConfig().BearerToken
}

// wellKnownInternalURLs are tried when a Kiali CR cannot be listed through the
// FilteringProvider (typical MCP wrapper) but the Kiali GVK is present.
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

func probeCandidateURLs(ctx context.Context, candidates []string, cfg *Config, bearerToken string) (string, bool) {
	for _, candidate := range candidates {
		if probeStatusURL(ctx, candidate, cfg, bearerToken) {
			return candidate, true
		}
	}
	return "", false
}
