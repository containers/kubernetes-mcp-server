package kcp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
)

// kcpTargetParameterName is the parameter name used to specify
// the workspace when using the kcp cluster provider strategy.
const kcpTargetParameterName = "workspace"

// kcpClusterProvider implements Provider for managing multiple
// kcp workspaces as separate cluster targets.
// It discovers workspaces via the kcp tenancy API and creates
// managers for each workspace on-demand.
type kcpClusterProvider struct {
	mu  sync.RWMutex
	cfg *config.Config
	*kubernetes.ProviderGVKFilter
	baseServerURL       string
	restConfig          *rest.Config
	clientCmdConfig     clientcmd.ClientConfig
	defaultWorkspace    string
	managers            map[string]*kubernetes.Manager
	workspaceWatcher    *WorkspaceWatcher
	clusterStateWatcher *watcher.ClusterState
}

var _ kubernetes.Provider = &kcpClusterProvider{}

func init() {
	kubernetes.RegisterProvider(config.ClusterProviderKcp, newKcpClusterProvider)
}

// newKcpClusterProvider creates a provider that manages multiple kcp workspaces.
// Each workspace is treated as a separate cluster target.
func newKcpClusterProvider(ctx context.Context, cfg *config.Config) (kubernetes.Provider, error) {
	ret := &kcpClusterProvider{cfg: cfg}
	if err := ret.reset(ctx); err != nil {
		return nil, err
	}
	ret.ProviderGVKFilter = kubernetes.NewProviderGVKFilter(ret)
	return ret, nil
}

func (p *kcpClusterProvider) reset(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resetLocked(ctx)
}

func (p *kcpClusterProvider) resetLocked(ctx context.Context) error {
	pathOptions := clientcmd.NewDefaultPathOptions()
	if p.cfg.KubeConfig.Get() != "" {
		pathOptions.LoadingRules.ExplicitPath = p.cfg.KubeConfig.Get()
	}

	clientCmdConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		&clientcmd.ConfigOverrides{})

	rawConfig, err := clientCmdConfig.RawConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	currentContext := rawConfig.Contexts[rawConfig.CurrentContext]
	if currentContext == nil {
		return errors.New("no current context in kubeconfig")
	}

	currentCluster := rawConfig.Clusters[currentContext.Cluster]
	if currentCluster == nil {
		return errors.New("current context's cluster not found in kubeconfig")
	}

	baseServerURL, defaultWorkspace := ParseServerURL(currentCluster.Server)
	if defaultWorkspace == "" {
		return errors.New("failed to parse workspace from kubeconfig cluster URL")
	}

	restConfig, err := clientCmdConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to create rest config: %w", err)
	}

	baseManager, err := kubernetes.NewKubeconfigManager(ctx, p.cfg, rawConfig.CurrentContext)
	if err != nil {
		return fmt.Errorf("failed to create base manager: %w", err)
	}

	workspaceList, err := DiscoverAllWorkspaces(ctx, restConfig, defaultWorkspace)
	if err != nil {
		klogutil.LogWarn(klogutil.FromContext(ctx), "Failed to discover workspaces via API, falling back to kubeconfig", klogutil.Err(err))
		workspaceList, err = workspacesFromKubeconfig(ctx, clientCmdConfig)
		if err != nil {
			baseManager.Close()
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}
	}

	k8s, err := baseManager.Derived(ctx)
	if err != nil {
		baseManager.Close()
		return fmt.Errorf("failed to get kubernetes client: %w", err)
	}

	newManagers := make(map[string]*kubernetes.Manager, len(workspaceList))
	for _, ws := range workspaceList {
		newManagers[ws] = nil
	}
	newManagers[defaultWorkspace] = baseManager

	oldManagers := p.managers
	p.clientCmdConfig = clientCmdConfig
	p.baseServerURL = baseServerURL
	p.defaultWorkspace = defaultWorkspace
	p.restConfig = restConfig
	p.managers = newManagers
	p.Close()
	p.workspaceWatcher = NewWorkspaceWatcher(ctx, k8s.DynamicClient(), defaultWorkspace, p.cfg.WorkspacePollInterval.Get(), p.cfg.WorkspaceDebounceWindow.Get())
	p.clusterStateWatcher = watcher.NewClusterState(ctx, k8s.DiscoveryClient(), p.cfg.ClusterStatePollInterval.Get(), p.cfg.ClusterStateDebounceWindow.Get())
	for _, old := range oldManagers {
		if old != nil {
			old.Close()
		}
	}

	return nil
}

// workspacesFromKubeconfig extracts workspace names from kubeconfig cluster URLs as a fallback.
func workspacesFromKubeconfig(ctx context.Context, clientCmdConfig clientcmd.ClientConfig) ([]string, error) {
	rawConfig, err := clientCmdConfig.RawConfig()
	if err != nil {
		return nil, err
	}

	workspaces := make(map[string]bool)
	for _, cluster := range rawConfig.Clusters {
		if ws := ExtractWorkspaceFromURL(cluster.Server); ws != "" {
			workspaces[ws] = true
		}
	}

	result := make([]string, 0, len(workspaces))
	for ws := range workspaces {
		result = append(result, ws)
	}

	klogutil.FromContext(ctx).V(2).Info("Discovered workspaces from kubeconfig", "num_workspaces", len(result))
	return result, nil
}

// managerForWorkspace returns or creates a Manager for the specified workspace.
// callerLock indicates whether the caller (true) or this func (false) is responsible for synchronization.
func (p *kcpClusterProvider) managerForWorkspace(ctx context.Context, workspace string, callerLock bool) (*kubernetes.Manager, error) {
	if !callerLock {
		p.mu.RLock()
	}
	m, ok := p.managers[workspace]
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
		m, ok := p.managers[workspace]
		if ok && m != nil {
			return m, nil
		}
	}

	if _, exists := p.managers[workspace]; !exists {
		return nil, fmt.Errorf("workspace %s not found: %w", workspace, kubernetes.ErrUnknownTarget)
	}

	// Create REST config for this workspace
	workspaceRestConfig := rest.CopyConfig(p.restConfig)
	workspaceRestConfig.Host = ConstructWorkspaceURL(p.baseServerURL, workspace)

	// Get raw config for context creation
	rawConfig, err := p.clientCmdConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Find or create context for this workspace
	contextName := p.findOrCreateWorkspaceContext(&rawConfig, workspace)

	clientCmdConfig := clientcmd.NewDefaultClientConfig(rawConfig,
		&clientcmd.ConfigOverrides{CurrentContext: contextName})

	m, err = kubernetes.NewManager(ctx, p.cfg, workspaceRestConfig, clientCmdConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create manager for workspace %s: %w", workspace, err)
	}

	p.managers[workspace] = m
	return m, nil
}

// findOrCreateWorkspaceContext finds an existing context for a workspace or creates a virtual one.
// Runs under lock.
func (p *kcpClusterProvider) findOrCreateWorkspaceContext(
	rawConfig *clientcmdapi.Config,
	workspace string,
) string {
	// Look for existing context pointing to this workspace
	for ctxName, ctx := range rawConfig.Contexts {
		cluster := rawConfig.Clusters[ctx.Cluster]
		if ExtractWorkspaceFromURL(cluster.Server) == workspace {
			return ctxName
		}
	}

	// Create new virtual context entry (in-memory only, not persisted)
	contextName := fmt.Sprintf("kcp-%s", workspace)
	clusterName := fmt.Sprintf("kcp-cluster-%s", workspace)

	// Get current context's cluster for copying TLS settings
	currentContext := rawConfig.Contexts[rawConfig.CurrentContext]
	currentCluster := rawConfig.Clusters[currentContext.Cluster]

	rawConfig.Clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                   ConstructWorkspaceURL(p.baseServerURL, workspace),
		CertificateAuthorityData: currentCluster.CertificateAuthorityData,
		CertificateAuthority:     currentCluster.CertificateAuthority,
		InsecureSkipTLSVerify:    currentCluster.InsecureSkipTLSVerify,
	}

	rawConfig.Contexts[contextName] = &clientcmdapi.Context{
		Cluster:  clusterName,
		AuthInfo: currentContext.AuthInfo,
	}

	return contextName
}

func (p *kcpClusterProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg.EnableTargetCompatibilityToolFilters.Get()
}

func (p *kcpClusterProvider) IsMultiTarget() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.managers) > 1
}

func (p *kcpClusterProvider) getTargetsUnsync() ([]string, error) {
	workspaces := make([]string, 0, len(p.managers))
	for ws := range p.managers {
		workspaces = append(workspaces, ws)
	}
	sort.Strings(workspaces)
	return workspaces, nil
}

func (p *kcpClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.getTargetsUnsync()
}

func (p *kcpClusterProvider) GetTargetManagers(ctx context.Context) ([]*kubernetes.Manager, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	contextNames, err := p.getTargetsUnsync()
	if err != nil {
		return nil, err
	}
	mgrs := make([]*kubernetes.Manager, 0, len(p.managers))
	for _, cn := range contextNames {
		mgr, err := p.managerForWorkspace(ctx, cn, true)
		if err != nil {
			return nil, err
		}
		mgrs = append(mgrs, mgr)
	}
	return mgrs, nil
}

func (p *kcpClusterProvider) GetTargetParameterName() string {
	return kcpTargetParameterName
}

func (p *kcpClusterProvider) GetDerivedKubernetes(ctx context.Context, workspace string) (*kubernetes.Kubernetes, error) {
	p.mu.RLock()
	if workspace == "" {
		workspace = p.defaultWorkspace
	}
	m, ok := p.managers[workspace]
	if ok && m != nil {
		k8s, err := m.Derived(ctx)
		p.mu.RUnlock()
		return k8s, err
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	m, err := p.managerForWorkspace(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	return m.Derived(ctx)
}

func (p *kcpClusterProvider) GetDefaultTarget() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.defaultWorkspace
}

func (p *kcpClusterProvider) ReloadConfig(_ context.Context, cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg
	return nil
}

func (p *kcpClusterProvider) PublishKubernetesConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, m := range p.managers {
		if m != nil {
			m.SetConfig(cfg)
		}
	}
}

func (p *kcpClusterProvider) WatchTargets(ctx context.Context, reload kubernetes.McpReloader) {
	reloadWithReset := func() error {
		return reload.Run(func() error {
			if err := p.reset(ctx); err != nil {
				return err
			}
			p.WatchTargets(ctx, reload)
			return reload.ApplyToolsets()
		})
	}
	p.workspaceWatcher.Watch(ctx, reloadWithReset)
	p.clusterStateWatcher.Watch(ctx, reload.ClusterStateCallback())
}

func (p *kcpClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.workspaceWatcher, p.clusterStateWatcher} {
		if w != nil && !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}
}
