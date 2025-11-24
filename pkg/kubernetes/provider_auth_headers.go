package kubernetes

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	authenticationv1api "k8s.io/api/authentication/v1"
	"k8s.io/klog/v2"
)

// AuthHeadersClusterProvider implements Provider for authentication via request headers.
// This provider requires users to provide authentication tokens via request headers.
// It uses cluster connection details from configuration but does not use any
// authentication credentials from kubeconfig files.
type AuthHeadersClusterProvider struct {
	staticConfig *config.StaticConfig
}

var _ Provider = &AuthHeadersClusterProvider{}

func init() {
	RegisterProvider(config.ClusterProviderAuthHeaders, newAuthHeadersClusterProvider)
}

// newAuthHeadersClusterProvider creates a provider that requires header-based authentication.
// Users must provide tokens via request headers (server URL, CA cert).
func newAuthHeadersClusterProvider(cfg *config.StaticConfig) (Provider, error) {
	// // Create a base manager using kubeconfig for cluster connection details
	// m, err := NewKubeconfigManager(cfg, "")
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create auth-headers provider: %w", err)
	// }

	// // Create a minimal kubeconfig with only cluster connection info (no auth)
	// rawConfig, err := m.clientCmdConfig.RawConfig()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to read kubeconfig: %w", err)
	// }

	// // Get the current context to extract cluster info
	// currentContext := rawConfig.Contexts[rawConfig.CurrentContext]
	// if currentContext == nil {
	// 	return nil, fmt.Errorf("current context not found in kubeconfig")
	// }

	// cluster := rawConfig.Clusters[currentContext.Cluster]
	// if cluster == nil {
	// 	return nil, fmt.Errorf("cluster %s not found in kubeconfig", currentContext.Cluster)
	// }

	// // Create a REST config with only cluster connection details (no auth)
	// restConfig := &rest.Config{
	// 	Host:    cluster.Server,
	// 	APIPath: m.cfg.APIPath,
	// 	TLSClientConfig: rest.TLSClientConfig{
	// 		Insecure:   cluster.InsecureSkipTLSVerify,
	// 		ServerName: cluster.TLSServerName,
	// 		CAData:     cluster.CertificateAuthorityData,
	// 		CAFile:     cluster.CertificateAuthority,
	// 	},
	// 	UserAgent: rest.DefaultKubernetesUserAgent(),
	// 	QPS:       m.cfg.QPS,
	// 	Burst:     m.cfg.Burst,
	// 	Timeout:   m.cfg.Timeout,
	// }

	// // Create a minimal clientcmd config without any authentication
	// minimalConfig := clientcmdapi.NewConfig()
	// minimalConfig.Clusters["cluster"] = &clientcmdapi.Cluster{
	// 	Server:                   cluster.Server,
	// 	InsecureSkipTLSVerify:    cluster.InsecureSkipTLSVerify,
	// 	CertificateAuthority:     cluster.CertificateAuthority,
	// 	CertificateAuthorityData: cluster.CertificateAuthorityData,
	// 	TLSServerName:            cluster.TLSServerName,
	// }
	// minimalConfig.Contexts["auth-headers-context"] = &clientcmdapi.Context{
	// 	Cluster: "cluster",
	// }
	// minimalConfig.CurrentContext = "auth-headers-context"

	// // Create a new manager with the stripped-down config
	// baseManager, err := newManager(cfg, restConfig, clientcmd.NewDefaultClientConfig(*minimalConfig, nil))
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create base manager for auth-headers provider: %w", err)
	// }

	klog.V(1).Infof("Auth-headers provider initialized - all requests must include valid headers")

	return &AuthHeadersClusterProvider{staticConfig: cfg}, nil
}

func (p *AuthHeadersClusterProvider) IsOpenShift(ctx context.Context) bool {
	return false
}

func (p *AuthHeadersClusterProvider) VerifyToken(ctx context.Context, target, token, audience string) (*authenticationv1api.UserInfo, []string, error) {
	return nil, nil, fmt.Errorf("auth-headers VerifyToken not implemented")
}

func (p *AuthHeadersClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	// Single cluster mode
	return []string{""}, nil
}

func (p *AuthHeadersClusterProvider) GetTargetParameterName() string {
	return ""
}

func (p *AuthHeadersClusterProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	// authHeaders, ok := ctx.Value(AuthHeadersContextKey).(*K8sAuthHeaders)
	// if !ok {
	// 	return nil, errors.New("authHeaders required")
	// }

	// decodedCA, err := authHeaders.GetDecodedCertificateAuthorityData()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to decode certificate authority data: %w", err)
	// }

	// restConfig := &rest.Config{
	// 	Host:        authHeaders.ClusterURL,
	// 	BearerToken: authHeaders.AuthorizationToken,
	// 	TLSClientConfig: rest.TLSClientConfig{
	// 		Insecure: false,
	// 		CAData:   decodedCA,
	// 	},
	// }

	// _ := clientcmd.NewDefaultClientConfig(*restConfig, nil)

	// // Create a REST config with only cluster connection details (no auth)
	// restConfig := &rest.Config{
	// 	Host:    cluster.Server,
	// 	APIPath: m.cfg.APIPath,
	// 	TLSClientConfig: rest.TLSClientConfig{
	// 		Insecure:   cluster.InsecureSkipTLSVerify,
	// 		ServerName: cluster.TLSServerName,
	// 		CAData:     cluster.CertificateAuthorityData,
	// 		CAFile:     cluster.CertificateAuthority,
	// 	},
	// 	UserAgent: rest.DefaultKubernetesUserAgent(),
	// 	QPS:       m.cfg.QPS,
	// 	Burst:     m.cfg.Burst,
	// 	Timeout:   m.cfg.Timeout,
	// }

	// // Create a minimal clientcmd config without any authentication
	// minimalConfig := clientcmdapi.NewConfig()
	// minimalConfig.Clusters["cluster"] = &clientcmdapi.Cluster{
	// 	Server:                   cluster.Server,
	// 	InsecureSkipTLSVerify:    cluster.InsecureSkipTLSVerify,
	// 	CertificateAuthority:     cluster.CertificateAuthority,
	// 	CertificateAuthorityData: cluster.CertificateAuthorityData,
	// 	TLSServerName:            cluster.TLSServerName,
	// }
	// minimalConfig.Contexts["auth-headers-context"] = &clientcmdapi.Context{
	// 	Cluster: "cluster",
	// }
	// minimalConfig.CurrentContext = "auth-headers-context"

	// derivedCfg := &rest.Config{
	// 	Host:    authHeaders.ClusterURL,
	// 	APIPath: m.cfg.APIPath,
	// 	// Copy only server verification TLS settings (CA bundle and server name)
	// 	TLSClientConfig: rest.TLSClientConfig{
	// 		Insecure:   m.cfg.Insecure,
	// 		ServerName: m.cfg.ServerName,
	// 		CAFile:     m.cfg.CAFile,
	// 		CAData:     m.cfg.CAData,
	// 	},
	// 	BearerToken: strings.TrimPrefix(authorization, "Bearer "),
	// 	// pass custom UserAgent to identify the client
	// 	UserAgent:   CustomUserAgent,
	// 	QPS:         m.cfg.QPS,
	// 	Burst:       m.cfg.Burst,
	// 	Timeout:     m.cfg.Timeout,
	// 	Impersonate: rest.ImpersonationConfig{},
	// }

	// type Manager struct {
	// 	cfg                     *rest.Config
	// 	clientCmdConfig         clientcmd.ClientConfig
	// 	discoveryClient         discovery.CachedDiscoveryInterface
	// 	accessControlClientSet  *AccessControlClientset
	// 	accessControlRESTMapper *AccessControlRESTMapper
	// 	dynamicClient           *dynamic.DynamicClient

	// 	staticConfig         *config.StaticConfig
	// 	CloseWatchKubeConfig CloseWatchKubeConfig
	// }

	// k := &Kubernetes{
	// 	manager: p.baseManager,
	// }

	return nil, nil
}

func (p *AuthHeadersClusterProvider) GetDefaultTarget() string {
	return ""
}

func (p *AuthHeadersClusterProvider) WatchTargets(watch func() error) {
}

func (p *AuthHeadersClusterProvider) Close() {
}
