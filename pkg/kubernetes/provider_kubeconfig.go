package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"k8s.io/klog/v2"
)

// KubeConfigTargetParameterName is the parameter name used to specify
// the kubeconfig context when using the kubeconfig cluster provider strategy.
const KubeConfigTargetParameterName = "context"

// kubeConfigClusterProvider implements Provider for managing multiple
// Kubernetes clusters using different contexts from a kubeconfig file.
// It lazily initializes managers for each context as they are requested.
type kubeConfigClusterProvider struct {
	mu                  sync.RWMutex
	config              api.BaseConfig
	defaultContext      string
	managers            map[string]*Manager
	contextServers      map[string]string
	kubeconfigWatcher   *watcher.Kubeconfig
	clusterStateWatcher *watcher.ClusterState
}

var _ Provider = &kubeConfigClusterProvider{}
var _ TokenExchangeProvider = &kubeConfigClusterProvider{}

func init() {
	RegisterProvider(api.ClusterProviderKubeConfig, newKubeConfigClusterProvider)
}

// newKubeConfigClusterProvider creates a provider that manages multiple clusters
// via kubeconfig contexts.
// Internally, it leverages a KubeconfigManager for each context, initializing them
// lazily when requested.
func newKubeConfigClusterProvider(cfg api.BaseConfig) (Provider, error) {
	ret := &kubeConfigClusterProvider{config: cfg}
	if err := ret.reset(); err != nil {
		return nil, err
	}
	return ret, nil
}

func (p *kubeConfigClusterProvider) reset() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := NewKubeconfigManager(p.config, "")
	if err != nil {
		if errors.Is(err, ErrorKubeconfigInClusterNotAllowed) {
			return fmt.Errorf( //nolint:ST1005 // user-facing error with actionable multi-line guidance
				"kubeconfig ClusterProviderStrategy is invalid for in-cluster deployments: %w\n\n"+
					"If you intend to connect to a different cluster from within a pod, provide the kubeconfig path explicitly:\n"+
					"  --kubeconfig /path/to/kubeconfig --cluster-provider kubeconfig\n\n"+
					"This overrides the in-cluster detection and uses the specified kubeconfig file instead.\n"+
					"See https://github.com/containers/kubernetes-mcp-server/blob/main/docs/configuration.md#cross-cluster-access-from-a-pod",
				err,
			)
		}
		return err
	}

	rawConfig, err := m.kubernetes.clientCmdConfig.RawConfig()
	if err != nil {
		m.Close()
		return err
	}

	// Determine the effective default context.
	// RawConfig() returns the file's current-context which may be empty when
	// NewKubeconfigManager auto-selected the only available context.
	defaultContext := rawConfig.CurrentContext
	if defaultContext == "" && len(rawConfig.Contexts) == 1 {
		for name := range rawConfig.Contexts {
			defaultContext = name
		}
	}

	for _, old := range p.managers {
		if old != nil {
			old.Close()
		}
	}
	p.managers = map[string]*Manager{
		defaultContext: m,
	}

	for name := range rawConfig.Contexts {
		if name == defaultContext {
			continue
		}
		p.managers[name] = nil
	}

	p.contextServers = make(map[string]string, len(rawConfig.Contexts))
	for name, ctxObj := range rawConfig.Contexts {
		if ctxObj == nil {
			continue
		}
		if cluster, ok := rawConfig.Clusters[ctxObj.Cluster]; ok && cluster != nil {
			p.contextServers[name] = cluster.Server
		}
	}

	p.Close()
	p.kubeconfigWatcher = watcher.NewKubeconfig(m.kubernetes.clientCmdConfig)
	p.clusterStateWatcher = watcher.NewClusterState(m.kubernetes.DiscoveryClient())
	p.defaultContext = defaultContext

	return nil
}

func (p *kubeConfigClusterProvider) managerForContext(context string) (*Manager, error) {
	p.mu.RLock()
	m, ok := p.managers[context]
	p.mu.RUnlock()
	if ok && m != nil {
		return m, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	m, ok = p.managers[context]
	if ok && m != nil {
		return m, nil
	}
	baseManager := p.managers[p.defaultContext]

	m, err := NewKubeconfigManager(baseManager.config, context)
	if err != nil {
		return nil, err
	}

	p.managers[context] = m

	return m, nil
}

func (p *kubeConfigClusterProvider) IsOpenShift(ctx context.Context) bool {
	p.mu.RLock()
	m := p.managers[p.defaultContext]
	p.mu.RUnlock()
	if m == nil {
		return false
	}
	return m.IsOpenShift(ctx)
}

func (p *kubeConfigClusterProvider) IsMultiTarget() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.managers) > 1
}

func (p *kubeConfigClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	contextNames := make([]string, 0, len(p.managers))
	for contextName := range p.managers {
		contextNames = append(contextNames, contextName)
	}

	return contextNames, nil
}

func (p *kubeConfigClusterProvider) GetTargetParameterName() string {
	return KubeConfigTargetParameterName
}

func (p *kubeConfigClusterProvider) GetDerivedKubernetes(ctx context.Context, context string) (*Kubernetes, error) {
	m, err := p.managerForContext(context)
	if err != nil {
		return nil, err
	}
	return m.Derived(ctx)
}

func (p *kubeConfigClusterProvider) GetDefaultTarget() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.defaultContext
}

func (p *kubeConfigClusterProvider) WatchTargets(reload McpReload) {
	reloadWithReset := func() error {
		if err := p.reset(); err != nil {
			return err
		}
		p.WatchTargets(reload)
		return reload()
	}
	p.kubeconfigWatcher.Watch(reloadWithReset)
	p.clusterStateWatcher.Watch(reload)
}

// providerConfig returns the parsed [cluster_provider_configs.kubeconfig]
// section, or nil if the operator did not configure one.
func (p *kubeConfigClusterProvider) providerConfig() *KubeconfigProviderConfig {
	raw, ok := p.config.GetProviderConfig(api.ClusterProviderKubeConfig)
	if !ok {
		return nil
	}
	cfg, _ := raw.(*KubeconfigProviderConfig)
	return cfg
}

// serverHostForContext returns the host portion of the kubeconfig
// cluster.server URL for the given context, or empty string if unknown.
func (p *kubeConfigClusterProvider) serverHostForContext(target string) string {
	p.mu.RLock()
	server, ok := p.contextServers[target]
	p.mu.RUnlock()
	if !ok || server == "" {
		return ""
	}
	u, err := url.Parse(server)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// GetTokenExchangeStrategy returns the exchanger name configured for the
// kubeconfig provider, falling back to the top-level sts strategy.
func (p *kubeConfigClusterProvider) GetTokenExchangeStrategy() string {
	if pc := p.providerConfig(); pc != nil && pc.TokenExchangeStrategy != "" {
		return pc.TokenExchangeStrategy
	}
	return p.config.GetStsStrategy()
}

// GetTokenExchangeConfig returns a per-target token exchange config.
//
// Returns nil when:
//   - no [cluster_provider_configs.kubeconfig] section is configured
//     (defers to the global STS path);
//   - the target's cluster.server host matches a SkipExchangeServers glob
//     (the bearer token is forwarded as-is — provided no top-level
//     token_exchange_strategy is set, otherwise the global path will run);
//   - validation of the constructed config fails.
//
// Otherwise the returned config is built from the top-level sts_* settings.
func (p *kubeConfigClusterProvider) GetTokenExchangeConfig(target string) *tokenexchange.TargetTokenExchangeConfig {
	pc := p.providerConfig()
	if pc == nil {
		return nil
	}

	if host := p.serverHostForContext(target); host != "" {
		for _, pattern := range pc.SkipExchangeServers {
			matched, err := filepath.Match(pattern, host)
			if err != nil {
				klog.V(2).Infof("kubeconfig provider: invalid skip_exchange_servers glob %q: %v", pattern, err)
				continue
			}
			if matched {
				klog.V(5).Infof("kubeconfig provider: target %q server %q matched skip_exchange_servers %q, skipping exchange", target, host, pattern)
				return nil
			}
		}
	}

	authStyle := p.config.GetStsAuthStyle()
	if authStyle == "" {
		authStyle = tokenexchange.AuthStyleParams
	}

	cfg := &tokenexchange.TargetTokenExchangeConfig{
		TokenURL:           p.config.GetStsTokenURL(),
		ClientID:           p.config.GetStsClientId(),
		ClientSecret:       p.config.GetStsClientSecret(),
		Audience:           p.config.GetStsAudience(),
		Scopes:             p.config.GetStsScopes(),
		AuthStyle:          authStyle,
		ClientCertFile:     p.config.GetStsClientCertFile(),
		ClientKeyFile:      p.config.GetStsClientKeyFile(),
		FederatedTokenFile: p.config.GetStsFederatedTokenFile(),
		SubjectTokenType:   p.config.GetStsSubjectTokenType(),
		RequestedTokenType: p.config.GetStsRequestedTokenType(),
		CAFile:             p.config.GetCertificateAuthority(),
	}
	if err := cfg.Validate(); err != nil {
		klog.Warningf("kubeconfig provider: token exchange config validation failed for target %q: %v", target, err)
		return nil
	}
	return cfg
}

func (p *kubeConfigClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.kubeconfigWatcher, p.clusterStateWatcher} {
		if !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}
}
