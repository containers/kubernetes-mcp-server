package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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
	config              api.BaseConfig
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
func newKubeConfigClusterProvider(cfg api.BaseConfig) (Provider, error) {
	ret := &kubeConfigClusterProvider{config: cfg}
	if err := ret.reset(); err != nil {
		return nil, err
	}
	return ret, nil
}

func (p *kubeConfigClusterProvider) reset() error {
	m, err := NewKubeconfigManager(p.config, "")
	if err != nil {
		if errors.Is(err, ErrorKubeconfigInClusterNotAllowed) {
			return fmt.Errorf("kubeconfig ClusterProviderStrategy is invalid for in-cluster deployments: %w", err)
		}
		return err
	}

	rawConfig, err := m.kubernetes.clientCmdConfig.RawConfig()
	if err != nil {
		return err
	}

	p.managers = map[string]*Manager{
		rawConfig.CurrentContext: m, // we already initialized a manager for the default context, let's use it
	}

	for name := range rawConfig.Contexts {
		if name == rawConfig.CurrentContext {
			continue // already initialized this, don't want to set it to nil
		}
		p.managers[name] = nil
	}

	p.Close()
	p.kubeconfigWatcher = watcher.NewKubeconfig(m.kubernetes.clientCmdConfig)
	p.clusterStateWatcher = watcher.NewClusterState(m.kubernetes.DiscoveryClient())
	p.defaultContext = rawConfig.CurrentContext

	return nil
}

func (p *kubeConfigClusterProvider) managerForContext(context string) (*Manager, error) {
	m, ok := p.managers[context]
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
	return p.managers[p.defaultContext].IsOpenShift(ctx)
}

func (p *kubeConfigClusterProvider) GetTargets(_ context.Context) ([]string, error) {
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

func (p *kubeConfigClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.kubeconfigWatcher, p.clusterStateWatcher} {
		if !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}
}
