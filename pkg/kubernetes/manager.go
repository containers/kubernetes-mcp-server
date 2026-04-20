package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
)

type Manager struct {
	kubernetes *Kubernetes

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

	resolvedContext, err := resolveKubeconfigContext(pathOptions.LoadingRules, kubeconfigContext)
	if err != nil {
		return nil, err
	}

	clientCmdConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		pathOptions.LoadingRules,
		&clientcmd.ConfigOverrides{
			ClusterInfo:    clientcmdapi.Cluster{Server: ""},
			CurrentContext: resolvedContext,
		})

	restConfig, err := clientCmdConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes rest config from kubeconfig: %w", err)
	}

	return NewManager(config, restConfig, clientCmdConfig)
}

// resolveKubeconfigContext determines which kubeconfig context to use.
// If kubeconfigContext is explicitly set, it is returned as-is.
// If it is empty, the function loads the kubeconfig and:
//   - returns the current-context if set
//   - auto-selects the only available context if there is exactly one
//   - returns a descriptive error if there are zero or multiple contexts
func resolveKubeconfigContext(loadingRules *clientcmd.ClientConfigLoadingRules, kubeconfigContext string) (string, error) {
	if kubeconfigContext != "" {
		return kubeconfigContext, nil
	}

	rawConfig, err := loadingRules.Load()
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	if rawConfig.CurrentContext != "" {
		return rawConfig.CurrentContext, nil
	}

	switch len(rawConfig.Contexts) {
	case 0:
		return "", fmt.Errorf( //nolint:ST1005 // user-facing error with actionable guidance
			"no current-context is set and no contexts are defined in kubeconfig.\n" +
				"Configure a context with 'kubectl config set-context <name>' and 'kubectl config use-context <name>'")
	case 1:
		for name := range rawConfig.Contexts {
			klog.Infof("current-context is not set in kubeconfig, auto-selecting the only available context %q", name)
			return name, nil
		}
	}

	names := make([]string, 0, len(rawConfig.Contexts))
	for name := range rawConfig.Contexts {
		names = append(names, name)
	}
	slices.Sort(names)
	return "", fmt.Errorf( //nolint:ST1005 // user-facing error with actionable guidance
		"current-context is not set in kubeconfig and multiple contexts are available (%s).\n"+
			"Set one with 'kubectl config use-context <context-name>'",
		strings.Join(names, ", "))
}

func NewInClusterManager(config api.BaseConfig) (*Manager, error) {
	if config.GetKubeConfigPath() != "" {
		return nil, fmt.Errorf("kubeconfig file %s cannot be used with the in-cluster deployments: %w", config.GetKubeConfigPath(), ErrorKubeconfigInClusterNotAllowed)
	}

	if !IsInCluster(config) {
		return nil, ErrorInClusterNotInCluster
	}

	restConfig, err := InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster kubernetes rest config: %w", err)
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
	k8s.kubernetes, err = NewKubernetes(k8s.config, clientCmdConfig, restConfig)
	if err != nil {
		return nil, err
	}
	return k8s, nil
}

func (m *Manager) Derived(ctx context.Context) (*Kubernetes, error) {
	authorization, ok := ctx.Value(OAuthAuthorizationHeader).(string)
	hasToken := ok && strings.HasPrefix(authorization, "Bearer ")

	// No token: use kubeconfig credentials, or reject if passthrough requires one.
	// In kubeconfig mode, the token exchange layer clears the auth header before we get here,
	// so this branch handles both "no token sent" and "kubeconfig mode cleared it".
	if !hasToken {
		if m.config.ResolveClusterAuthMode() == api.ClusterAuthPassthrough {
			return nil, errors.New("oauth token required for passthrough auth mode")
		}
		return m.kubernetes, nil
	}

	klog.V(5).Infof("%s header found (Bearer), using provided bearer token", OAuthAuthorizationHeader)
	userAgent := CustomUserAgent
	if ua, ok := ctx.Value(UserAgentHeader).(string); ok && ua != "" {
		userAgent = ua
	}
	derivedCfg := &rest.Config{
		Host:    m.kubernetes.RESTConfig().Host,
		APIPath: m.kubernetes.RESTConfig().APIPath,
		// Copy only server verification TLS settings (CA bundle and server name)
		TLSClientConfig: rest.TLSClientConfig{
			Insecure:   m.kubernetes.RESTConfig().Insecure,
			ServerName: m.kubernetes.RESTConfig().ServerName,
			CAFile:     m.kubernetes.RESTConfig().CAFile,
			CAData:     m.kubernetes.RESTConfig().CAData,
		},
		BearerToken: strings.TrimPrefix(authorization, "Bearer "),
		// pass custom UserAgent to identify the client
		UserAgent:   userAgent,
		QPS:         m.kubernetes.RESTConfig().QPS,
		Burst:       m.kubernetes.RESTConfig().Burst,
		Timeout:     m.kubernetes.RESTConfig().Timeout,
		Impersonate: rest.ImpersonationConfig{},
	}
	clientCmdApiConfig, err := m.kubernetes.clientCmdConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	clientCmdApiConfig.AuthInfos = make(map[string]*clientcmdapi.AuthInfo)
	derived, err := NewKubernetes(m.config, clientcmd.NewDefaultClientConfig(clientCmdApiConfig, nil), derivedCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create derived client: %w", err)
	}
	context.AfterFunc(ctx, derived.close)
	return derived, nil
}

// Close releases HTTP transport resources held by this manager.
func (m *Manager) Close() {
	if m != nil {
		m.kubernetes.close()
	}
}

// Invalidate invalidates the cached discovery information.
func (m *Manager) Invalidate() {
	m.kubernetes.DiscoveryClient().Invalidate()
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
