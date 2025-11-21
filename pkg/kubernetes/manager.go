package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/fsnotify/fsnotify"
	authenticationv1api "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
)

type Manager struct {
	accessControlClientset *AccessControlClientset

	staticConfig         *config.StaticConfig
	CloseWatchKubeConfig CloseWatchKubeConfig

	clusterWatcher *clusterStateWatcher
}

// clusterState represents the cached state of the cluster
type clusterState struct {
	apiGroups   []string
	isOpenShift bool
}

// clusterStateWatcher monitors cluster state changes and triggers debounced reloads
type clusterStateWatcher struct {
	manager        *Manager
	pollInterval   time.Duration
	debounceWindow time.Duration
	lastKnownState clusterState
	reloadCallback func() error
	debounceTimer  *time.Timer
	mu             sync.Mutex
	stopCh         chan struct{}
	stoppedCh      chan struct{}
}

var _ Openshift = (*Manager)(nil)

const (
	// DefaultClusterStatePollInterval is the default interval for polling cluster state changes
	DefaultClusterStatePollInterval = 30 * time.Second
	// DefaultClusterStateDebounceWindow is the default debounce window for cluster state changes
	DefaultClusterStateDebounceWindow = 5 * time.Second
)

var (
	ErrorKubeconfigInClusterNotAllowed = errors.New("kubeconfig manager cannot be used in in-cluster deployments")
	ErrorInClusterNotInCluster         = errors.New("in-cluster manager cannot be used outside of a cluster")
)

func NewKubeconfigManager(config *config.StaticConfig, kubeconfigContext string) (*Manager, error) {
	if IsInCluster(config) {
		return nil, ErrorKubeconfigInClusterNotAllowed
	}

	pathOptions := clientcmd.NewDefaultPathOptions()
	if config.KubeConfig != "" {
		pathOptions.LoadingRules.ExplicitPath = config.KubeConfig
	}
	clientCmdConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		&clientcmd.ConfigOverrides{
			ClusterInfo:    clientcmdapi.Cluster{Server: ""},
			CurrentContext: kubeconfigContext,
		})

	restConfig, err := clientCmdConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes rest config from kubeconfig: %v", err)
	}

	return NewManager(config, restConfig, clientCmdConfig)
}

func NewInClusterManager(config *config.StaticConfig) (*Manager, error) {
	if config.KubeConfig != "" {
		return nil, fmt.Errorf("kubeconfig file %s cannot be used with the in-cluster deployments: %v", config.KubeConfig, ErrorKubeconfigInClusterNotAllowed)
	}

	if !IsInCluster(config) {
		return nil, ErrorInClusterNotInCluster
	}

	restConfig, err := InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster kubernetes rest config: %v", err)
	}

	// Create a dummy kubeconfig clientcmdapi.Config for in-cluster config to be used in places where clientcmd.ClientConfig is required
	clientCmdConfig := clientcmdapi.NewConfig()
	clientCmdConfig.Clusters["cluster"] = &clientcmdapi.Cluster{
		Server:                restConfig.Host,
		InsecureSkipTLSVerify: restConfig.Insecure,
	}
	clientCmdConfig.AuthInfos["user"] = &clientcmdapi.AuthInfo{
		Token: restConfig.BearerToken,
	}
	clientCmdConfig.Contexts[inClusterKubeConfigDefaultContext] = &clientcmdapi.Context{
		Cluster:  "cluster",
		AuthInfo: "user",
	}
	clientCmdConfig.CurrentContext = inClusterKubeConfigDefaultContext

	return NewManager(config, restConfig, clientcmd.NewDefaultClientConfig(*clientCmdConfig, nil))
}

func NewManager(config *config.StaticConfig, restConfig *rest.Config, clientCmdConfig clientcmd.ClientConfig) (*Manager, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}
	if restConfig == nil {
		return nil, errors.New("restConfig cannot be nil")
	}
	if clientCmdConfig == nil {
		return nil, errors.New("clientCmdConfig cannot be nil")
	}

	// Apply QPS and Burst from environment variables if set (primarily for testing)
	applyRateLimitFromEnv(restConfig)

	k8s := &Manager{
		staticConfig: config,
	}
	var err error
	// TODO: Won't work because not all client-go clients use the shared context (e.g. discovery client uses context.TODO())
	//k8s.cfg.Wrap(func(original http.RoundTripper) http.RoundTripper {
	//	return &impersonateRoundTripper{original}
	//})
	k8s.accessControlClientset, err = NewAccessControlClientset(k8s.staticConfig, clientCmdConfig, restConfig)
	if err != nil {
		return nil, err
	}
	return k8s, nil
}

func (m *Manager) WatchKubeConfig(onKubeConfigChange func() error) {
	kubeConfigFiles := m.accessControlClientset.ToRawKubeConfigLoader().ConfigAccess().GetLoadingPrecedence()
	if len(kubeConfigFiles) == 0 {
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	for _, file := range kubeConfigFiles {
		_ = watcher.Add(file)
	}
	go func() {
		for {
			select {
			case _, ok := <-watcher.Events:
				if !ok {
					return
				}
				_ = onKubeConfigChange()
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	if m.CloseWatchKubeConfig != nil {
		_ = m.CloseWatchKubeConfig()
	}
	m.CloseWatchKubeConfig = watcher.Close
}

func (m *Manager) Close() {
	if m.CloseWatchKubeConfig != nil {
		_ = m.CloseWatchKubeConfig()
	}
	if m.clusterWatcher != nil {
		m.clusterWatcher.stop()
	}
}

func (m *Manager) VerifyToken(ctx context.Context, token, audience string) (*authenticationv1api.UserInfo, []string, error) {
	tokenReviewClient := m.accessControlClientset.AuthenticationV1().TokenReviews()
	tokenReview := &authenticationv1api.TokenReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "authentication.k8s.io/v1",
			Kind:       "TokenReview",
		},
		Spec: authenticationv1api.TokenReviewSpec{
			Token:     token,
			Audiences: []string{audience},
		},
	}

	result, err := tokenReviewClient.Create(ctx, tokenReview, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create token review: %v", err)
	}

	if !result.Status.Authenticated {
		if result.Status.Error != "" {
			return nil, nil, fmt.Errorf("token authentication failed: %s", result.Status.Error)
		}
		return nil, nil, fmt.Errorf("token authentication failed")
	}

	return &result.Status.User, result.Status.Audiences, nil
}

func (m *Manager) Derived(ctx context.Context) (*Kubernetes, error) {
	authorization, ok := ctx.Value(OAuthAuthorizationHeader).(string)
	if !ok || !strings.HasPrefix(authorization, "Bearer ") {
		if m.staticConfig.RequireOAuth {
			return nil, errors.New("oauth token required")
		}
		return &Kubernetes{m.accessControlClientset}, nil
	}
	klog.V(5).Infof("%s header found (Bearer), using provided bearer token", OAuthAuthorizationHeader)
	derivedCfg := &rest.Config{
		Host:          m.accessControlClientset.cfg.Host,
		APIPath:       m.accessControlClientset.cfg.APIPath,
		WrapTransport: m.accessControlClientset.cfg.WrapTransport,
		// Copy only server verification TLS settings (CA bundle and server name)
		TLSClientConfig: rest.TLSClientConfig{
			Insecure:   m.accessControlClientset.cfg.Insecure,
			ServerName: m.accessControlClientset.cfg.ServerName,
			CAFile:     m.accessControlClientset.cfg.CAFile,
			CAData:     m.accessControlClientset.cfg.CAData,
		},
		BearerToken: strings.TrimPrefix(authorization, "Bearer "),
		// pass custom UserAgent to identify the client
		UserAgent:   CustomUserAgent,
		QPS:         m.accessControlClientset.cfg.QPS,
		Burst:       m.accessControlClientset.cfg.Burst,
		Timeout:     m.accessControlClientset.cfg.Timeout,
		Impersonate: rest.ImpersonationConfig{},
	}
	clientCmdApiConfig, err := m.accessControlClientset.clientCmdConfig.RawConfig()
	if err != nil {
		if m.staticConfig.RequireOAuth {
			klog.Errorf("failed to get kubeconfig: %v", err)
			return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
		}
		return &Kubernetes{m.accessControlClientset}, nil
	}
	clientCmdApiConfig.AuthInfos = make(map[string]*clientcmdapi.AuthInfo)
	derived, err := NewAccessControlClientset(m.staticConfig, clientcmd.NewDefaultClientConfig(clientCmdApiConfig, nil), derivedCfg)
	if err != nil {
		if m.staticConfig.RequireOAuth {
			klog.Errorf("failed to create derived clientset: %v", err)
			return nil, fmt.Errorf("failed to create derived clientset: %w", err)
		}
		return &Kubernetes{m.accessControlClientset}, nil
	}
	return &Kubernetes{derived}, nil
}

// applyRateLimitFromEnv applies QPS and Burst rate limits from environment variables if set.
// This is primarily useful for tests to avoid client-side rate limiting.
// Environment variables:
//   - KUBE_CLIENT_QPS: Sets the QPS (queries per second) limit
//   - KUBE_CLIENT_BURST: Sets the burst limit
func applyRateLimitFromEnv(cfg *rest.Config) {
	if qpsStr := os.Getenv("KUBE_CLIENT_QPS"); qpsStr != "" {
		if qps, err := strconv.ParseFloat(qpsStr, 32); err == nil {
			cfg.QPS = float32(qps)
		}
	}
	if burstStr := os.Getenv("KUBE_CLIENT_BURST"); burstStr != "" {
		if burst, err := strconv.Atoi(burstStr); err == nil {
			cfg.Burst = burst
		}
	}
}

// WatchClusterState starts a background watcher that periodically polls for cluster state changes
// and triggers a debounced reload when changes are detected.
func (m *Manager) WatchClusterState(pollInterval, debounceWindow time.Duration, onClusterStateChange func() error) {
	if m.clusterWatcher != nil {
		m.clusterWatcher.stop()
	}

	watcher := &clusterStateWatcher{
		manager:        m,
		pollInterval:   pollInterval,
		debounceWindow: debounceWindow,
		reloadCallback: onClusterStateChange,
		stopCh:         make(chan struct{}),
		stoppedCh:      make(chan struct{}),
	}

	captureState := func() clusterState {
		state := clusterState{apiGroups: []string{}}
		if groups, err := m.accessControlClientset.DiscoveryClient().ServerGroups(); err == nil {
			for _, group := range groups.Groups {
				state.apiGroups = append(state.apiGroups, group.Name)
			}
			sort.Strings(state.apiGroups)
		}
		state.isOpenShift = m.IsOpenShift(context.Background())
		return state
	}
	watcher.lastKnownState = captureState()

	m.clusterWatcher = watcher

	// Start background monitoring
	go func() {
		defer close(watcher.stoppedCh)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		klog.V(2).Infof("Started cluster state watcher (poll interval: %v, debounce: %v)", pollInterval, debounceWindow)

		for {
			select {
			case <-watcher.stopCh:
				klog.V(2).Info("Stopping cluster state watcher")
				return
			case <-ticker.C:
				// Invalidate discovery cache to get fresh API groups
				m.accessControlClientset.DiscoveryClient().Invalidate()

				watcher.mu.Lock()
				current := captureState()
				klog.V(3).Infof("Polled cluster state: %d API groups, OpenShift=%v", len(current.apiGroups), current.isOpenShift)

				changed := current.isOpenShift != watcher.lastKnownState.isOpenShift ||
					len(current.apiGroups) != len(watcher.lastKnownState.apiGroups)

				if !changed {
					for i := range current.apiGroups {
						if current.apiGroups[i] != watcher.lastKnownState.apiGroups[i] {
							changed = true
							break
						}
					}
				}

				if changed {
					klog.V(2).Info("Cluster state changed, scheduling debounced reload")
					if watcher.debounceTimer != nil {
						watcher.debounceTimer.Stop()
					}
					watcher.debounceTimer = time.AfterFunc(debounceWindow, func() {
						klog.V(2).Info("Debounce window expired, triggering reload")
						if err := onClusterStateChange(); err != nil {
							klog.Errorf("Failed to reload: %v", err)
						} else {
							watcher.mu.Lock()
							watcher.lastKnownState = captureState()
							watcher.mu.Unlock()
							klog.V(2).Info("Reload completed")
						}
					})
				}
				watcher.mu.Unlock()
			}
		}
	}()
}

// stop stops the cluster state watcher
func (w *clusterStateWatcher) stop() {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	if w.stopCh == nil || w.stoppedCh == nil {
		return
	}

	select {
	case <-w.stopCh:
		// Already closed or stopped
		return
	default:
		close(w.stopCh)
		<-w.stoppedCh
	}
}
