package kubernetes

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	authenticationv1api "k8s.io/api/authentication/v1"
)

// KubeConfigTargetParameterName is the parameter name used to specify
// the kubeconfig context when using the kubeconfig cluster provider strategy.
const KubeConfigTargetParameterName = "context"

// kubeConfigClusterProvider implements Provider for managing multiple
// Kubernetes clusters using different contexts from a kubeconfig file.
// It lazily initializes managers for each context as they are requested.
type kubeConfigClusterProvider struct {
	defaultContext string
	managers       map[string]*Manager
}

var _ Provider = &kubeConfigClusterProvider{}

func init() {
	RegisterProvider(config.ClusterProviderKubeConfig, newKubeConfigClusterProvider)
}

// newKubeConfigClusterProvider creates a provider that manages multiple clusters
// via kubeconfig contexts. Returns an error if the manager is in-cluster mode.
func newKubeConfigClusterProvider(m *Manager, cfg *config.StaticConfig) (Provider, error) {
	// Handle in-cluster mode
	if IsInCluster(cfg) {
		return nil, fmt.Errorf("kubeconfig ClusterProviderStrategy is invalid for in-cluster deployments")
	}

	rawConfig, err := m.clientCmdConfig.RawConfig()
	if err != nil {
		return nil, err
	}

	allClusterManagers := map[string]*Manager{
		rawConfig.CurrentContext: m, // we already initialized a manager for the default context, let's use it
	}

	for name := range rawConfig.Contexts {
		if name == rawConfig.CurrentContext {
			continue // already initialized this, don't want to set it to nil
		}

		allClusterManagers[name] = nil
	}

	return &kubeConfigClusterProvider{
		defaultContext: rawConfig.CurrentContext,
		managers:       allClusterManagers,
	}, nil
}

func (p *kubeConfigClusterProvider) managerForContext(context string) (*Manager, error) {
	m, ok := p.managers[context]
	if ok && m != nil {
		return m, nil
	}

	baseManager := p.managers[p.defaultContext]

	m, err := NewManager(baseManager.staticConfig, context)
	if err != nil {
		return nil, err
	}

	p.managers[context] = m

	return m, nil
}

func (p *kubeConfigClusterProvider) IsOpenShift(ctx context.Context) bool {
	return p.managers[p.defaultContext].IsOpenShift(ctx)
}

func (p *kubeConfigClusterProvider) VerifyToken(ctx context.Context, context, token, audience string) (*authenticationv1api.UserInfo, []string, error) {
	m, err := p.managerForContext(context)
	if err != nil {
		return nil, nil, err
	}
	return m.VerifyToken(ctx, token, audience)
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

func (p *kubeConfigClusterProvider) WatchTargets(onKubeConfigChanged func() error) {
	m := p.managers[p.defaultContext]

	m.WatchKubeConfig(onKubeConfigChanged)
}

func (p *kubeConfigClusterProvider) Close() {
	m := p.managers[p.defaultContext]

	m.Close()
}
