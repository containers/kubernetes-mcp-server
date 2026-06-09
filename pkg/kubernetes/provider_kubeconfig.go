package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
)

// KubeConfigTargetParameterName is the parameter name used to specify
// the kubeconfig context when using the kubeconfig cluster provider strategy.
const KubeConfigTargetParameterName = "context"

// kubeConfigClusterProvider implements Provider for managing multiple
// Kubernetes clusters using different contexts from a kubeconfig file.
// It lazily initializes managers for each context as they are requested.
type kubeConfigClusterProvider struct {
	mu sync.RWMutex
	api.BaseConfig
	*ProviderGVKFilter
	defaultContext      string
	managers            map[string]*Manager
	kubeconfigWatcher   *watcher.Kubeconfig
	clusterStateWatcher *watcher.ClusterState
}

var _ Provider = &kubeConfigClusterProvider{}

func init() {
	RegisterProvider(api.ClusterProviderKubeConfig, newKubeConfigClusterProvider)
}

// newKubeConfigClusterProvider creates a provider that manages multiple clusters
// via kubeconfig contexts.
// Internally, it leverages a KubeconfigManager for each context, initializing them
// lazily when requested.
func newKubeConfigClusterProvider(ctx context.Context, cfg api.BaseConfig) (Provider, error) {
	ret := &kubeConfigClusterProvider{BaseConfig: cfg}
	if err := ret.reset(ctx); err != nil {
		return nil, err
	}
	ret.ProviderGVKFilter = NewProviderGVKFilter(ret)
	return ret, nil
}

func (p *kubeConfigClusterProvider) reset(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	m, err := NewKubeconfigManager(ctx, p, "")
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

	p.Close()
	p.kubeconfigWatcher = watcher.NewKubeconfig(ctx, m.kubernetes.clientCmdConfig)
	p.clusterStateWatcher = watcher.NewClusterState(ctx, m.kubernetes.DiscoveryClient())
	p.defaultContext = defaultContext

	return nil
}

// managerForWorkspace returns or creates a Manager for the specified kubeContext.
// callerLock indicates whether the caller (true) or this func (false) is responsible for synchronization.
func (p *kubeConfigClusterProvider) managerForContext(ctx context.Context, kubeContext string, callerLock bool) (*Manager, error) {
	if !callerLock {
		p.mu.RLock()
	}
	m, ok := p.managers[kubeContext]
	if !callerLock {
		p.mu.RUnlock()
	}
	if ok && m != nil {
		return m, nil
	}

	if !callerLock {
		p.mu.Lock()
		defer p.mu.Unlock()
		// Recheck in case it was introduced since RUnlock
		m, ok = p.managers[kubeContext]
		if ok && m != nil {
			return m, nil
		}
	}

	baseManager := p.managers[p.defaultContext]

	m, err := NewKubeconfigManager(ctx, baseManager.config, kubeContext)
	if err != nil {
		return nil, err
	}

	p.managers[kubeContext] = m

	return m, nil
}

func (p *kubeConfigClusterProvider) IsMultiTarget() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.managers) > 1
}

func (p *kubeConfigClusterProvider) getTargetsUnsync() ([]string, error) {
	contextNames := make([]string, 0, len(p.managers))
	for contextName := range p.managers {
		contextNames = append(contextNames, contextName)
	}

	return contextNames, nil
}

func (p *kubeConfigClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.getTargetsUnsync()
}

func (p *kubeConfigClusterProvider) GetTargetManagers(ctx context.Context) ([]*Manager, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	contextNames, err := p.getTargetsUnsync()
	if err != nil {
		return nil, err
	}
	managers := make([]*Manager, 0, len(contextNames))
	for _, cn := range contextNames {
		mgr, err := p.managerForContext(ctx, cn, true)
		if err != nil {
			return nil, err
		}
		managers = append(managers, mgr)
	}

	return managers, nil
}

func (p *kubeConfigClusterProvider) GetTargetParameterName() string {
	return KubeConfigTargetParameterName
}

func (p *kubeConfigClusterProvider) GetDerivedKubernetes(ctx context.Context, context string) (*Kubernetes, error) {
	m, err := p.managerForContext(ctx, context, false)
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

func (p *kubeConfigClusterProvider) WatchTargets(ctx context.Context, reload McpReload) {
	reloadWithReset := func() error {
		if err := p.reset(ctx); err != nil {
			return err
		}
		p.WatchTargets(ctx, reload)
		return reload()
	}
	p.kubeconfigWatcher.Watch(ctx, reloadWithReset)
	p.clusterStateWatcher.Watch(ctx, reload)
}

func (p *kubeConfigClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.kubeconfigWatcher, p.clusterStateWatcher} {
		if !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}
}
