package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	authenticationv1api "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
)

type Manager struct {
	accessControlClientset *AccessControlClientset

	config api.BaseConfig
}

var _ api.Openshift = (*Manager)(nil)

var (
	ErrorKubeconfigInClusterNotAllowed = errors.New("kubeconfig manager cannot be used in in-cluster deployments")
	ErrorInClusterNotInCluster         = errors.New("in-cluster manager cannot be used outside of a cluster")
)

func NewKubeconfigManager(config api.BaseConfig, kubeconfigContext string) (*Manager, error) {
	if IsInCluster(config) {
		return nil, ErrorKubeconfigInClusterNotAllowed
	}

	pathOptions := clientcmd.NewDefaultPathOptions()
	if config.GetKubeConfigPath() != "" {
		pathOptions.LoadingRules.ExplicitPath = config.GetKubeConfigPath()
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

func NewInClusterManager(config api.BaseConfig) (*Manager, error) {
	if config.GetKubeConfigPath() != "" {
		return nil, fmt.Errorf("kubeconfig file %s cannot be used with the in-cluster deployments: %v", config.GetKubeConfigPath(), ErrorKubeconfigInClusterNotAllowed)
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

func NewManager(config api.BaseConfig, restConfig *rest.Config, clientCmdConfig clientcmd.ClientConfig) (*Manager, error) {
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
		config: config,
	}
	var err error
	// TODO: Won't work because not all client-go clients use the shared context (e.g. discovery client uses context.TODO())
	//k8s.restConfig.Wrap(func(original http.RoundTripper) http.RoundTripper {
	//	return &impersonateRoundTripper{original}
	//})
	k8s.accessControlClientset, err = NewAccessControlClientset(k8s.config, clientCmdConfig, restConfig)
	if err != nil {
		return nil, err
	}
	return k8s, nil
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
		if m.config.IsRequireOAuth() {
			return nil, errors.New("oauth token required")
		}
		return &Kubernetes{m.accessControlClientset}, nil
	}
	klog.V(5).Infof("%s header found (Bearer), using provided bearer token", OAuthAuthorizationHeader)
	derivedCfg := &rest.Config{
		Host:          m.accessControlClientset.RESTConfig().Host,
		APIPath:       m.accessControlClientset.RESTConfig().APIPath,
		WrapTransport: m.accessControlClientset.RESTConfig().WrapTransport,
		// Copy only server verification TLS settings (CA bundle and server name)
		TLSClientConfig: rest.TLSClientConfig{
			Insecure:   m.accessControlClientset.RESTConfig().Insecure,
			ServerName: m.accessControlClientset.RESTConfig().ServerName,
			CAFile:     m.accessControlClientset.RESTConfig().CAFile,
			CAData:     m.accessControlClientset.RESTConfig().CAData,
		},
		BearerToken: strings.TrimPrefix(authorization, "Bearer "),
		// pass custom UserAgent to identify the client
		UserAgent:   CustomUserAgent,
		QPS:         m.accessControlClientset.RESTConfig().QPS,
		Burst:       m.accessControlClientset.RESTConfig().Burst,
		Timeout:     m.accessControlClientset.RESTConfig().Timeout,
		Impersonate: rest.ImpersonationConfig{},
	}
	clientCmdApiConfig, err := m.accessControlClientset.clientCmdConfig.RawConfig()
	if err != nil {
		if m.config.IsRequireOAuth() {
			klog.Errorf("failed to get kubeconfig: %v", err)
			return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
		}
		return &Kubernetes{m.accessControlClientset}, nil
	}
	clientCmdApiConfig.AuthInfos = make(map[string]*clientcmdapi.AuthInfo)
	derived, err := NewAccessControlClientset(m.config, clientcmd.NewDefaultClientConfig(clientCmdApiConfig, nil), derivedCfg)
	if err != nil {
		if m.config.IsRequireOAuth() {
			klog.Errorf("failed to create derived clientset: %v", err)
			return nil, fmt.Errorf("failed to create derived clientset: %w", err)
		}
		return &Kubernetes{m.accessControlClientset}, nil
	}
	return &Kubernetes{derived}, nil
}

// Invalidate invalidates the cached discovery information.
func (m *Manager) Invalidate() {
	m.accessControlClientset.DiscoveryClient().Invalidate()
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
