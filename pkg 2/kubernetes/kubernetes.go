package kubernetes

import (
	"context"
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/fsnotify/fsnotify"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/helm"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type HeaderKey string

const (
	CustomAuthorizationHeader = HeaderKey("kubernetes-authorization")
	OAuthAuthorizationHeader  = HeaderKey("Authorization")

	CustomUserAgent = "kubernetes-mcp-server/bearer-token-auth"
)

type CloseWatchKubeConfig func() error

type Kubernetes struct {
	manager *Manager
}

type Manager struct {
	cfg                     *rest.Config
	clientCmdConfig         clientcmd.ClientConfig
	discoveryClient         discovery.CachedDiscoveryInterface
	accessControlClientSet  *AccessControlClientset
	accessControlRESTMapper *AccessControlRESTMapper
	dynamicClient           *dynamic.DynamicClient

	staticConfig         *config.StaticConfig
	CloseWatchKubeConfig CloseWatchKubeConfig
}

var Scheme = scheme.Scheme
var ParameterCodec = runtime.NewParameterCodec(Scheme)

var _ helm.Kubernetes = &Manager{}

func NewManager(config *config.StaticConfig) (*Manager, error) {
	k8s := &Manager{
		staticConfig: config,
	}
	if err := resolveKubernetesConfigurations(k8s); err != nil {
		return nil, err
	}
	// TODO: Won't work because not all client-go clients use the shared context (e.g. discovery client uses context.TODO())
	//k8s.cfg.Wrap(func(original http.RoundTripper) http.RoundTripper {
	//	return &impersonateRoundTripper{original}
	//})
	var err error
	k8s.accessControlClientSet, err = NewAccessControlClientset(k8s.cfg, k8s.staticConfig)
	if err != nil {
		return nil, err
	}
	k8s.discoveryClient = memory.NewMemCacheClient(k8s.accessControlClientSet.DiscoveryClient())
	k8s.accessControlRESTMapper = NewAccessControlRESTMapper(
		restmapper.NewDeferredDiscoveryRESTMapper(k8s.discoveryClient),
		k8s.staticConfig,
	)
	k8s.dynamicClient, err = dynamic.NewForConfig(k8s.cfg)
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

func (m *Manager) GetAPIServerHost() string {
	if m.cfg == nil {
		return ""
	}
	return m.cfg.Host
}

func (m *Manager) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return m.discoveryClient, nil
}

func (m *Manager) ToRESTMapper() (meta.RESTMapper, error) {
	return m.accessControlRESTMapper, nil
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
		Host:    m.cfg.Host,
		APIPath: m.cfg.APIPath,
		// Copy only server verification TLS settings (CA bundle and server name)
		TLSClientConfig: rest.TLSClientConfig{
			Insecure:   m.cfg.Insecure,
			ServerName: m.cfg.ServerName,
			CAFile:     m.cfg.CAFile,
			CAData:     m.cfg.CAData,
		},
		BearerToken: strings.TrimPrefix(authorization, "Bearer "),
		// pass custom UserAgent to identify the client
		UserAgent:   CustomUserAgent,
		QPS:         m.cfg.QPS,
		Burst:       m.cfg.Burst,
		Timeout:     m.cfg.Timeout,
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
	derived := &Kubernetes{manager: &Manager{
		clientCmdConfig: clientcmd.NewDefaultClientConfig(clientCmdApiConfig, nil),
		cfg:             derivedCfg,
		staticConfig:    m.staticConfig,
	}}
	derived.manager.accessControlClientSet, err = NewAccessControlClientset(derived.manager.cfg, derived.manager.staticConfig)
	if err != nil {
		if m.staticConfig.RequireOAuth {
			klog.Errorf("failed to get kubeconfig: %v", err)
			return nil, errors.New("failed to get kubeconfig")
		}
		return &Kubernetes{manager: m}, nil
	}
	derived.manager.discoveryClient = memory.NewMemCacheClient(derived.manager.accessControlClientSet.DiscoveryClient())
	derived.manager.accessControlRESTMapper = NewAccessControlRESTMapper(
		restmapper.NewDeferredDiscoveryRESTMapper(derived.manager.discoveryClient),
		derived.manager.staticConfig,
	)
	derived.manager.dynamicClient, err = dynamic.NewForConfig(derived.manager.cfg)
	if err != nil {
		if m.staticConfig.RequireOAuth {
			klog.Errorf("failed to initialize dynamic client: %v", err)
			return nil, errors.New("failed to initialize dynamic client")
		}
		return &Kubernetes{manager: m}, nil
	}
	return derived, nil
}

func (k *Kubernetes) NewHelm() *helm.Helm {
	// This is a derived Kubernetes, so it already has the Helm initialized
	return helm.NewHelm(k.manager)
}
