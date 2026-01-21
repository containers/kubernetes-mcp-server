package kcp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
)

// kcpTargetParameterName is the parameter name used to specify
// the workspace when using the kcp cluster provider strategy.
const kcpTargetParameterName = "workspace"

// kcpClusterProvider implements Provider for managing multiple
// kcp workspaces as separate cluster targets.
// It discovers workspaces via the kcp tenancy API and creates
// managers for each workspace on-demand.
type kcpClusterProvider struct {
	config              api.BaseConfig
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
	kubernetes.RegisterProvider(api.ClusterProviderKcp, newKcpClusterProvider)
}

// newKcpClusterProvider creates a provider that manages multiple kcp workspaces.
// Each workspace is treated as a separate cluster target.
func newKcpClusterProvider(cfg api.BaseConfig) (kubernetes.Provider, error) {
	ret := &kcpClusterProvider{config: cfg}
	if err := ret.reset(); err != nil {
		return nil, err
	}
	return ret, nil
}

func (p *kcpClusterProvider) reset() error {
	// Load kubeconfig
	pathOptions := clientcmd.NewDefaultPathOptions()
	if p.config.GetKubeConfigPath() != "" {
		pathOptions.LoadingRules.ExplicitPath = p.config.GetKubeConfigPath()
	}

	p.clientCmdConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		&clientcmd.ConfigOverrides{})

	rawConfig, err := p.clientCmdConfig.RawConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Get current context
	currentContext := rawConfig.Contexts[rawConfig.CurrentContext]
	if currentContext == nil {
		return errors.New("no current context in kubeconfig")
	}

	currentCluster := rawConfig.Clusters[currentContext.Cluster]
	if currentCluster == nil {
		return errors.New("current context's cluster not found in kubeconfig")
	}

	// Parse kcp server URL to extract base URL and workspace
	p.baseServerURL, p.defaultWorkspace = ParseServerURL(currentCluster.Server)
	if p.defaultWorkspace == "" {
		return errors.New("failed to parse workspace from kubeconfig cluster URL")
	}

	// Create REST config
	p.restConfig, err = p.clientCmdConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to create rest config: %w", err)
	}

	// Create base manager for workspace discovery
	baseManager, err := kubernetes.NewKubeconfigManager(p.config, rawConfig.CurrentContext)
	if err != nil {
		return fmt.Errorf("failed to create base manager: %w", err)
	}

	// Discover workspaces
	workspaceList, err := p.discoverWorkspaces(baseManager)
	if err != nil {
		klog.Warningf("Failed to discover workspaces via API, falling back to kubeconfig: %v", err)
		workspaceList, err = p.workspacesFromKubeconfig()
		if err != nil {
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}
	}

	// Initialize workspace managers (lazily, set to nil first)
	p.managers = make(map[string]*kubernetes.Manager, len(workspaceList))
	for _, ws := range workspaceList {
		p.managers[ws] = nil
	}
	// Store the base manager for the default workspace
	p.managers[p.defaultWorkspace] = baseManager

	// Setup watchers
	p.Close()
	k8s, err := baseManager.Derived(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get kubernetes client: %w", err)
	}
	p.workspaceWatcher = NewWorkspaceWatcher(k8s.DynamicClient(), p.defaultWorkspace)
	p.clusterStateWatcher = watcher.NewClusterState(k8s.DiscoveryClient())

	return nil
}

// discoverWorkspaces queries the kcp tenancy API to discover available workspaces.
// It recursively discovers nested workspaces in the workspace hierarchy.
func (p *kcpClusterProvider) discoverWorkspaces(_ *kubernetes.Manager) ([]string, error) {
	return DiscoverAllWorkspaces(context.TODO(), p.restConfig, p.defaultWorkspace)
}

// workspacesFromKubeconfig extracts workspace names from kubeconfig cluster URLs as a fallback.
func (p *kcpClusterProvider) workspacesFromKubeconfig() ([]string, error) {
	rawConfig, err := p.clientCmdConfig.RawConfig()
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

	klog.V(2).Infof("Discovered %d workspaces from kubeconfig", len(result))
	return result, nil
}

// managerForWorkspace returns or creates a Manager for the specified workspace.
func (p *kcpClusterProvider) managerForWorkspace(workspace string) (*kubernetes.Manager, error) {
	m, ok := p.managers[workspace]
	if ok && m != nil {
		return m, nil
	}

	if _, exists := p.managers[workspace]; !exists {
		return nil, fmt.Errorf("workspace %s not found", workspace)
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

	m, err = kubernetes.NewManager(p.config, workspaceRestConfig, clientCmdConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create manager for workspace %s: %w", workspace, err)
	}

	p.managers[workspace] = m
	return m, nil
}

// findOrCreateWorkspaceContext finds an existing context for a workspace or creates a virtual one.
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

func (p *kcpClusterProvider) IsOpenShift(ctx context.Context) bool {
	// Use default workspace manager
	if m, ok := p.managers[p.defaultWorkspace]; ok && m != nil {
		return m.IsOpenShift(ctx)
	}
	return false
}

func (p *kcpClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	workspaces := make([]string, 0, len(p.managers))
	for ws := range p.managers {
		workspaces = append(workspaces, ws)
	}
	sort.Strings(workspaces)
	return workspaces, nil
}

func (p *kcpClusterProvider) GetTargetParameterName() string {
	return kcpTargetParameterName
}

func (p *kcpClusterProvider) GetDerivedKubernetes(ctx context.Context, workspace string) (*kubernetes.Kubernetes, error) {
	if workspace == "" {
		workspace = p.defaultWorkspace
	}

	m, err := p.managerForWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	return m.Derived(ctx)
}

func (p *kcpClusterProvider) GetDefaultTarget() string {
	return p.defaultWorkspace
}

func (p *kcpClusterProvider) WatchTargets(reload kubernetes.McpReload) {
	reloadWithReset := func() error {
		if err := p.reset(); err != nil {
			return err
		}
		p.WatchTargets(reload)
		return reload()
	}

	p.workspaceWatcher.Watch(reloadWithReset)
	p.clusterStateWatcher.Watch(reload)
}

func (p *kcpClusterProvider) Close() {
	for _, w := range []watcher.Watcher{p.workspaceWatcher, p.clusterStateWatcher} {
		if w != nil && !reflect.ValueOf(w).IsNil() {
			w.Close()
		}
	}
}
