package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/helm"
	"github.com/fsnotify/fsnotify"
	authenticationv1api "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
)

type Manager struct {
	clientCmdConfig        clientcmd.ClientConfig
	accessControlClientSet *AccessControlClientset

	staticConfig         *config.StaticConfig
	CloseWatchKubeConfig CloseWatchKubeConfig
}

var _ helm.Kubernetes = (*Manager)(nil)
var _ Openshift = (*Manager)(nil)

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

	return newManager(config, restConfig, clientCmdConfig)
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

	return newManager(config, restConfig, clientcmd.NewDefaultClientConfig(*clientCmdConfig, nil))
}

func newManager(config *config.StaticConfig, restConfig *rest.Config, clientCmdConfig clientcmd.ClientConfig) (*Manager, error) {
	k8s := &Manager{
		staticConfig:    config,
		clientCmdConfig: clientCmdConfig,
	}
	var err error
	// TODO: Won't work because not all client-go clients use the shared context (e.g. discovery client uses context.TODO())
	//k8s.cfg.Wrap(func(original http.RoundTripper) http.RoundTripper {
	//	return &impersonateRoundTripper{original}
	//})
	k8s.accessControlClientSet, err = NewAccessControlClientset(k8s.staticConfig, restConfig)
	if err != nil {
		return nil, err
	}
	return k8s, nil
}

func (m *Manager) WatchKubeConfig(onKubeConfigChange func() error) {
	if m.clientCmdConfig == nil {
		return
	}
	kubeConfigFiles := m.clientCmdConfig.ConfigAccess().GetLoadingPrecedence()
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
}

func (m *Manager) configuredNamespace() string {
	if ns, _, nsErr := m.clientCmdConfig.Namespace(); nsErr == nil {
		return ns
	}
	return ""
}

func (m *Manager) NamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return m.configuredNamespace()
	}
	return namespace
}

func (m *Manager) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return m.accessControlClientSet.DiscoveryClient(), nil
}

func (m *Manager) ToRESTMapper() (meta.RESTMapper, error) {
	return m.accessControlClientSet.RESTMapper(), nil
}

// ToRESTConfig returns the rest.Config object (genericclioptions.RESTClientGetter)
func (m *Manager) ToRESTConfig() (*rest.Config, error) {
	return m.accessControlClientSet.cfg, nil
}

// ToRawKubeConfigLoader returns the clientcmd.ClientConfig object (genericclioptions.RESTClientGetter)
func (m *Manager) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return m.clientCmdConfig
}

func (m *Manager) VerifyToken(ctx context.Context, token, audience string) (*authenticationv1api.UserInfo, []string, error) {
	tokenReviewClient := m.accessControlClientSet.AuthenticationV1().TokenReviews()
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
		return &Kubernetes{manager: m}, nil
	}
	klog.V(5).Infof("%s header found (Bearer), using provided bearer token", OAuthAuthorizationHeader)
	derivedCfg := &rest.Config{
		Host:          m.accessControlClientSet.cfg.Host,
		APIPath:       m.accessControlClientSet.cfg.APIPath,
		WrapTransport: m.accessControlClientSet.cfg.WrapTransport,
		// Copy only server verification TLS settings (CA bundle and server name)
		TLSClientConfig: rest.TLSClientConfig{
			Insecure:   m.accessControlClientSet.cfg.Insecure,
			ServerName: m.accessControlClientSet.cfg.ServerName,
			CAFile:     m.accessControlClientSet.cfg.CAFile,
			CAData:     m.accessControlClientSet.cfg.CAData,
		},
		BearerToken: strings.TrimPrefix(authorization, "Bearer "),
		// pass custom UserAgent to identify the client
		UserAgent:   CustomUserAgent,
		QPS:         m.accessControlClientSet.cfg.QPS,
		Burst:       m.accessControlClientSet.cfg.Burst,
		Timeout:     m.accessControlClientSet.cfg.Timeout,
		Impersonate: rest.ImpersonationConfig{},
	}
	clientCmdApiConfig, err := m.clientCmdConfig.RawConfig()
	if err != nil {
		if m.staticConfig.RequireOAuth {
			klog.Errorf("failed to get kubeconfig: %v", err)
			return nil, errors.New("failed to get kubeconfig")
		}
		return &Kubernetes{manager: m}, nil
	}
	clientCmdApiConfig.AuthInfos = make(map[string]*clientcmdapi.AuthInfo)
	derived := &Kubernetes{
		manager: &Manager{
			clientCmdConfig: clientcmd.NewDefaultClientConfig(clientCmdApiConfig, nil),
			staticConfig:    m.staticConfig,
		},
	}
	derived.manager.accessControlClientSet, err = NewAccessControlClientset(derived.manager.staticConfig, derivedCfg)
	if err != nil {
		if m.staticConfig.RequireOAuth {
			klog.Errorf("failed to get kubeconfig: %v", err)
			return nil, errors.New("failed to get kubeconfig")
		}
		return &Kubernetes{manager: m}, nil
	}
	return derived, nil
}
