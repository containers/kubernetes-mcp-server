package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
)

const (
	ACMHubTargetParameterName    = "cluster"
	ClusterProviderACM           = "acm"
	ClusterProviderACMKubeConfig = "acm-kubeconfig"
	LocalClusterLabel            = "local-cluster"
)

// ACMProviderConfig holds ACM-specific configuration that users can set in config.toml
type ACMProviderConfig struct {
	// The host for the ACM cluster proxy addon
	// Optional: If not provided, will auto-discover the cluster-proxy-addon-user OCP route
	// If using the acm-kubeconfig strategy, this should be the route hostname for the proxy
	// If using the acm strategy, this should be the service name for the proxy
	ClusterProxyAddonHost string `toml:"cluster_proxy_addon_host,omitempty"`

	// Whether to skip verifying the TLS certs from the cluster proxy
	ClusterProxyAddonSkipTLSVerify bool `toml:"cluster_proxy_addon_skip_tls_verify"`

	// The CA file for the cluster proxy addon
	ClusterProxyAddonCAFile string `toml:"cluster_proxy_addon_ca_file,omitempty"`

	// TokenExchangeStrategy specifies which token exchange protocol to use
	// Valid values: "keycloak-v1", "rfc8693", ""
	// Default: "" (no per-cluster token exchange)
	TokenExchangeStrategy string `toml:"token_exchange_strategy,omitempty"`

	// Clusters holds per-cluster token exchange configuration
	// The key is the cluster name (e.g. "my-managed-cluster")
	Clusters map[string]*tokenexchange.TargetTokenExchangeConfig `toml:"clusters,omitempty"`
}

func (c *ACMProviderConfig) Validate() error {
	var err error = nil
	if !c.ClusterProxyAddonSkipTLSVerify && c.ClusterProxyAddonCAFile == "" {
		err = errors.Join(err, fmt.Errorf("cluster_proxy_addon_ca_file is required if tls verification is not disabled"))
	}

	return err
}

func (c *ACMProviderConfig) ResolveClusterProxyAddonCAFilePath(ctx context.Context) {
	path := config.ConfigDirPathFromContext(ctx)
	c.ClusterProxyAddonCAFile = filepath.Join(path, c.ClusterProxyAddonCAFile)
}

type ACMKubeConfigProviderConfig struct {
	ACMProviderConfig

	// Name of the context in the kubeconfig file to look for acm access credentials in.
	// Should point to the "hub" cluster.
	ContextName string `toml:"context_name,omitempty"`
}

func (c *ACMKubeConfigProviderConfig) Validate() error {
	err := c.ACMProviderConfig.Validate()

	if c.ContextName == "" {
		err = errors.Join(err, fmt.Errorf("context_name is required if acm-kubeconfig strategy is used"))
	}

	return err
}

func parseAcmConfig(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	cfg := &ACMProviderConfig{}
	if err := md.PrimitiveDecode(primitive, cfg); err != nil {
		return nil, err
	}

	cfg.ResolveClusterProxyAddonCAFilePath(ctx)

	return cfg, nil
}

func parseAcmKubeConfigConfig(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	cfg := &ACMKubeConfigProviderConfig{}
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}

	cfg.ResolveClusterProxyAddonCAFilePath(ctx)

	return cfg, nil
}

type acmHubClusterProvider struct {
	hubManager         *Manager // for the main "hub" cluster
	clusterProxyHost   string
	skipTLSVerify      bool
	clusterProxyCAFile string
	watchKubeConfig    bool // whether or not the kubeconfig should be watched for changes

	// config for token exchange
	targetTokenConfigs map[string]*tokenexchange.TargetTokenExchangeConfig
	exchangeStrategy   string

	kubeConfigWatcher *watcher.Kubeconfig
	clusterWatcher    *watcher.ClusterState

	// Context for cancelling the watch goroutine
	watchCtx    context.Context
	watchCancel context.CancelFunc

	// initialResourceVersion is set during init and passed to watchManagedClusters
	initialResourceVersion string

	// mu protects clusterManagers, hubClusterName, and watchStarted
	mu              sync.RWMutex
	clusterManagers map[string]*Manager
	hubClusterName  string
	watchStarted    bool
}

var _ Provider = &acmHubClusterProvider{}

func init() {
	RegisterProvider(ClusterProviderACM, newACMHubClusterProvider)
	RegisterProvider(ClusterProviderACMKubeConfig, newACMKubeConfigClusterProvider)

	config.RegisterProviderConfig(ClusterProviderACM, parseAcmConfig)
	config.RegisterProviderConfig(ClusterProviderACMKubeConfig, parseAcmKubeConfigConfig)
}

// IsACMHub checks if the current cluster is an ACM hub by looking for ACM CRDs
// This is included here instead of in other files so that it doesn't create conflicts
// with upstream changes
func (m *Manager) IsACMHub() bool {
	discoveryClient := m.kubernetes.DiscoveryClient()

	_, apiLists, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		klog.V(3).Infof("failed to discover server resources for ACM detection: %v", err)
		return false
	}

	for _, apiList := range apiLists {
		if apiList.GroupVersion == "cluster.open-cluster-management.io/v1" {
			for _, resource := range apiList.APIResources {
				if resource.Kind == "ManagedCluster" {
					klog.V(2).Info("Detected ACM hub cluster")
					return true
				}
			}
		}
	}

	return false
}

func newACMHubClusterProvider(cfg api.BaseConfig) (Provider, error) {
	m, err := NewInClusterManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster Kubernetes Manager for acm-hub cluster provider strategy: %w", err)
	}

	providerCfg, ok := cfg.GetProviderConfig(ClusterProviderACM)
	if !ok {
		return nil, fmt.Errorf("missing required config for strategy '%s'", ClusterProviderACM)
	}

	return newACMClusterProvider(m, providerCfg.(*ACMProviderConfig), false)
}

func newACMKubeConfigClusterProvider(cfg api.BaseConfig) (Provider, error) {
	providerCfg, ok := cfg.GetProviderConfig(ClusterProviderACMKubeConfig)
	if !ok {
		return nil, fmt.Errorf("missing required config for strategy '%s'", ClusterProviderACMKubeConfig)
	}

	acmKubeConfigProviderCfg := providerCfg.(*ACMKubeConfigProviderConfig)
	baseManager, err := NewKubeconfigManager(cfg, acmKubeConfigProviderCfg.ContextName)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create manager to hub cluster specified by acm_context_name %s: %w",
			acmKubeConfigProviderCfg.ContextName,
			err,
		)
	}

	return newACMClusterProvider(baseManager, &acmKubeConfigProviderCfg.ACMProviderConfig, true)
}

func discoverClusterProxyHost(m *Manager, isClusterProviderACMKubeConfig bool) (string, error) {
	ctx := context.Background()

	// With ClusterProviderACMKubeConfig we cannot use a vanilla service, since the mcp server is not in the cluster
	if isClusterProviderACMKubeConfig {
		// Try OpenShift Route in multicluster-engine namespace
		routeGVR := schema.GroupVersionResource{
			Group:    "route.openshift.io",
			Version:  "v1",
			Resource: "routes",
		}

		route, err := m.kubernetes.DynamicClient().Resource(routeGVR).Namespace("multicluster-engine").Get(ctx, "cluster-proxy-addon-user", metav1.GetOptions{})
		if err == nil {
			host, found, err := unstructured.NestedString(route.Object, "spec", "host")
			if err == nil && found && host != "" {
				klog.V(2).Infof("Auto-discovered cluster-proxy route: %s", host)
				return host, nil
			}
		}
	}

	// Fallback: Try to find the service
	svcClient := m.kubernetes.CoreV1().Services("multicluster-engine")
	svc, err := svcClient.Get(ctx, "cluster-proxy-addon-user", metav1.GetOptions{})
	if err == nil && len(svc.Spec.Ports) > 0 {
		port := svc.Spec.Ports[0].Port // default to first port
		for _, p := range svc.Spec.Ports {
			if p.Name == "user-port" { // if we find user port, use this instead
				port = p.Port
			}
		}
		host := fmt.Sprintf("%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, port)
		klog.V(2).Infof("Auto-discovered cluster-proxy service: %s", host)
		return host, nil
	}

	return "", fmt.Errorf("failed to auto-discover cluster-proxy host: route and service not found")
}

func newACMClusterProvider(m *Manager, cfg *ACMProviderConfig, watchKubeConfig bool) (Provider, error) {
	if !m.IsACMHub() {
		return nil, fmt.Errorf("not deployed in an ACM hub cluster")
	}

	// Auto-discover cluster-proxy host if is not provided
	clusterProxyHost := cfg.ClusterProxyAddonHost
	if clusterProxyHost == "" {
		discoveredHost, err := discoverClusterProxyHost(m, watchKubeConfig)
		if err != nil {
			return nil, fmt.Errorf("cluster_proxy_addon_host not provided and auto-discovery failed: %w", err)
		}
		clusterProxyHost = discoveredHost
		klog.V(1).Infof("Using auto-discovered cluster-proxy host: %s", clusterProxyHost)
	}

	// Create cancellable context for the watch goroutine
	watchCtx, watchCancel := context.WithCancel(context.Background())

	provider := &acmHubClusterProvider{
		hubManager:         m,
		clusterManagers:    make(map[string]*Manager),
		targetTokenConfigs: cfg.Clusters,
		exchangeStrategy:   cfg.TokenExchangeStrategy,
		watchKubeConfig:    watchKubeConfig,
		watchCtx:           watchCtx,
		watchCancel:        watchCancel,
		clusterProxyHost:   clusterProxyHost,
		clusterProxyCAFile: cfg.ClusterProxyAddonCAFile,
		skipTLSVerify:      cfg.ClusterProxyAddonSkipTLSVerify,
		kubeConfigWatcher:  watcher.NewKubeconfig(m.kubernetes.clientCmdConfig),
		clusterWatcher:     watcher.NewClusterState(m.kubernetes.DiscoveryClient()),
	}

	ctx := context.Background()
	resourceVersion, err := provider.refreshClusters(ctx)
	if err != nil {
		klog.Warningf("Failed to discover managed clusters: %v", err)
	}
	provider.initialResourceVersion = resourceVersion

	klog.V(2).Infof("ACM hub provider initialized with %d managed clusters", len(provider.clusterManagers))
	return provider, nil
}

func (p *acmHubClusterProvider) IsOpenShift(ctx context.Context) bool {
	return p.hubManager.IsOpenShift(ctx)
}

func (p *acmHubClusterProvider) IsMultiTarget() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.clusterManagers) > 1
}

func (p *acmHubClusterProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	if target == "" || target == p.GetDefaultTarget() {
		return p.hubManager.Derived(ctx)
	}

	manager, err := p.managerForCluster(target)
	if err != nil {
		return nil, err
	}

	return manager.Derived(ctx)
}

func (p *acmHubClusterProvider) GetDefaultTarget() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.hubClusterName == "" {
		return "local-cluster" // fallback if hub not yet discovered
	}
	return p.hubClusterName
}

func (p *acmHubClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	targets := make([]string, 0, len(p.clusterManagers))
	for name := range p.clusterManagers {
		targets = append(targets, name)
	}
	slices.Sort(targets)
	return targets, nil
}

func (p *acmHubClusterProvider) GetTargetParameterName() string {
	return ACMHubTargetParameterName
}

func (p *acmHubClusterProvider) SupportsGVKs(_ []schema.GroupVersionKind) bool {
	return true
}

func (p *acmHubClusterProvider) WatchTargets(reload McpReload) {
	if p.watchKubeConfig {
		p.kubeConfigWatcher.Watch(reload)
	}

	p.clusterWatcher.Watch(reload)

	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.watchStarted {
		p.watchStarted = true
		go p.watchManagedClusters(p.initialResourceVersion, reload)
	}
}

func (p *acmHubClusterProvider) Close() {
	// Cancel the watch goroutine first
	if p.watchCancel != nil {
		p.watchCancel()
	}

	p.mu.Lock()
	p.watchStarted = false
	p.mu.Unlock()

	p.clusterWatcher.Close()
	p.kubeConfigWatcher.Close()
}

// GetTokenExchangeConfig returns the token exchange configuration for the specified target.
// Returns nil if no per-target exchange is configured
func (p *acmHubClusterProvider) GetTokenExchangeConfig(target string) *tokenexchange.TargetTokenExchangeConfig {
	defaultTarget := p.GetDefaultTarget()
	if target == "" {
		target = defaultTarget
	}

	cfg, ok := p.targetTokenConfigs[target]
	if !ok {
		return nil
	}

	// Auto-detect subject token type if not explicitly configured
	if cfg.SubjectTokenType == "" {
		cfg.SubjectTokenType = p.detectSubjectTokenType(target, cfg)
	}

	return cfg
}

// detectSubjectTokenType determines whether to use same-realm (access_token) or cross-realm (jwt)
// token exchange based on comparing token URLs.
//   - If target's token_url matches the hub cluster's token_url → same-realm (access_token)
//   - If token_url differs → cross-realm (jwt)
//   - Fallback: If hub config unavailable, compare target name with hub name
func (p *acmHubClusterProvider) detectSubjectTokenType(target string, targetCfg *tokenexchange.TargetTokenExchangeConfig) string {
	defaultTarget := p.GetDefaultTarget()

	hubCfg, hubHasConfig := p.targetTokenConfigs[defaultTarget]

	if hubHasConfig && hubCfg.TokenURL != "" && targetCfg.TokenURL != "" {
		if targetCfg.TokenURL == hubCfg.TokenURL {
			return tokenexchange.TokenTypeAccessToken
		}
		return tokenexchange.TokenTypeJWT
	}

	if target == defaultTarget {
		return tokenexchange.TokenTypeAccessToken
	}
	return tokenexchange.TokenTypeJWT
}

// GetTokenExchangeStrategy returns the token exchange strategy to use (e.g. "keycloak-v1" or "rfc8693").
func (p *acmHubClusterProvider) GetTokenExchangeStrategy() string {
	return p.exchangeStrategy
}

func (p *acmHubClusterProvider) watchManagedClusters(resourceVersion string, onTargetsChanged func() error) {
	gvr := schema.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}

	const (
		initialDelay = 1 * time.Second
		maxDelay     = 5 * time.Minute
		backoffRate  = 2.0
	)

	delay := initialDelay

	for {
		select {
		case <-p.watchCtx.Done():
			klog.V(2).Info("Watch goroutine cancelled, exiting")
			return
		default:
		}

		watchInterface, err := p.hubManager.kubernetes.DynamicClient().Resource(gvr).Watch(
			p.watchCtx,
			metav1.ListOptions{
				ResourceVersion: resourceVersion,
			},
		)
		if err != nil {
			klog.Errorf("Failed to start watch on managed clusters: %v", err)
			klog.V(2).Infof("Waiting %v before retrying watch", delay)
			time.Sleep(delay)
			delay = min(time.Duration(float64(delay)*backoffRate), maxDelay)
			continue
		}

		delay = initialDelay
		klog.V(2).Info("Started watching managed clusters for changes")

		for event := range watchInterface.ResultChan() {
			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			clusterName := obj.GetName()
			resourceVersion = obj.GetResourceVersion()

			switch event.Type {
			case watch.Added:
				klog.V(3).Infof("Managed cluster added: %s", clusterName)
				p.addCluster(clusterName)
				// Check if this is the hub cluster
				labels := obj.GetLabels()
				if labels[LocalClusterLabel] == "true" {
					p.setHubClusterName(clusterName)
				}
				if err := onTargetsChanged(); err != nil {
					klog.Warningf("Error in onTargetsChanged callback: %v", err)
				}
			case watch.Deleted:
				klog.V(3).Infof("Managed cluster deleted: %s", clusterName)
				p.removeCluster(clusterName)
				if err := onTargetsChanged(); err != nil {
					klog.Warningf("Error in onTargetsChanged callback: %v", err)
				}
			case watch.Modified:
				klog.V(3).Infof("Managed cluster modified: %s", clusterName)
			}
		}

		watchInterface.Stop()
		klog.Warning("Managed clusters watch closed, restarting...")
	}
}

func (p *acmHubClusterProvider) refreshClusters(ctx context.Context) (string, error) {
	dynamicClient := p.hubManager.kubernetes.DynamicClient()

	gvr := schema.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}

	result, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list cluster managers: %w", err)
	}

	for _, item := range result.Items {
		name := item.GetName()
		if name != "" {
			p.addCluster(name)
		}

		labels := item.GetLabels()
		if labels[LocalClusterLabel] == "true" {
			p.setHubClusterName(name)
		}
	}

	resourceVersion := result.GetResourceVersion()
	clusters, _ := p.GetTargets(ctx)
	klog.V(3).Infof("discovered %d managed clusters: %v (resourceVersion: %s)", len(clusters), clusters, resourceVersion)

	return resourceVersion, nil
}

func (p *acmHubClusterProvider) setHubClusterName(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.hubClusterName = name
}

func (p *acmHubClusterProvider) addCluster(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.clusterManagers[name]; !exists {
		p.clusterManagers[name] = nil
	}
}

func (p *acmHubClusterProvider) removeCluster(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.clusterManagers, name)
}

func (p *acmHubClusterProvider) managerForCluster(cluster string) (*Manager, error) {
	p.mu.RLock()
	manager, exists := p.clusterManagers[cluster]
	p.mu.RUnlock()

	if exists && manager != nil {
		return manager, nil
	}

	if !exists {
		return nil, fmt.Errorf("cluster %s not found", cluster)
	}

	proxyConfig := rest.CopyConfig(p.hubManager.kubernetes.restConfig)
	proxyHost := fmt.Sprintf("https://%s/%s", p.clusterProxyHost, cluster)
	proxyConfig.Host = proxyHost

	if p.skipTLSVerify {
		proxyConfig.TLSClientConfig = rest.TLSClientConfig{
			Insecure: true,
		}
	} else {
		proxyConfig.TLSClientConfig = rest.TLSClientConfig{
			CAFile: p.clusterProxyCAFile,
		}
	}

	// Configure TCP keep-alive to prevent idle connection closure
	proxyConfig.Dial = (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext

	hubRawConfig, err := p.hubManager.kubernetes.clientCmdConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get hub kubeconfig: %w", err)
	}

	proxyRawConfig := hubRawConfig.DeepCopy()

	for _, clusterConfig := range proxyRawConfig.Clusters {
		clusterConfig.Server = proxyHost
		if p.skipTLSVerify {
			clusterConfig.InsecureSkipTLSVerify = true
			clusterConfig.CertificateAuthority = ""
			clusterConfig.CertificateAuthorityData = nil
		} else {
			clusterConfig.CertificateAuthority = p.clusterProxyCAFile
			clusterConfig.CertificateAuthorityData = nil
			clusterConfig.InsecureSkipTLSVerify = false
		}
	}

	proxyClientCmdConfig := clientcmd.NewDefaultClientConfig(*proxyRawConfig, nil)
	newManager, err := NewManager(p.hubManager.config, proxyConfig, proxyClientCmdConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create manager for cluster %s: %w", cluster, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if existingManager := p.clusterManagers[cluster]; existingManager != nil {
		return existingManager, nil
	}

	p.clusterManagers[cluster] = newManager
	return newManager, nil
}
