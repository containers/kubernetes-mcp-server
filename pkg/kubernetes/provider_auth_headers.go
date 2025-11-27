package kubernetes

import (
	"context"
	"errors"
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
// Users must provide tokens via request headers (server URL, Token or client certificate and key).
func newAuthHeadersClusterProvider(cfg *config.StaticConfig) (Provider, error) {
	klog.V(1).Infof("Auth-headers provider initialized - all requests must include valid k8s auth headers")

	return &AuthHeadersClusterProvider{staticConfig: cfg}, nil
}

func (p *AuthHeadersClusterProvider) IsOpenShift(ctx context.Context) bool {
	klog.V(1).Infof("IsOpenShift not supported for auth-headers provider. Returning false.")
	return false
}

func (p *AuthHeadersClusterProvider) VerifyToken(ctx context.Context, target, token, audience string) (*authenticationv1api.UserInfo, []string, error) {
	return nil, nil, fmt.Errorf("VerifyToken not supported for auth-headers provider")
}

func (p *AuthHeadersClusterProvider) GetTargets(_ context.Context) ([]string, error) {
	klog.V(1).Infof("GetTargets not supported for auth-headers provider. Returning empty list.")
	return []string{""}, nil
}

func (p *AuthHeadersClusterProvider) GetTargetParameterName() string {
	klog.V(1).Infof("GetTargetParameterName not supported for auth-headers provider. Returning empty name.")
	return ""
}

func (p *AuthHeadersClusterProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	authHeaders, ok := ctx.Value(AuthHeadersContextKey).(*K8sAuthHeaders)
	if !ok {
		return nil, errors.New("authHeaders required")
	}

	manager, err := NewAuthHeadersClusterManager(authHeaders, p.staticConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth headers cluster manager: %w", err)
	}

	return &Kubernetes{manager: manager}, nil
}

func (p *AuthHeadersClusterProvider) GetDefaultTarget() string {
	klog.V(1).Infof("GetDefaultTarget not supported for auth-headers provider. Returning empty name.")
	return ""
}

func (p *AuthHeadersClusterProvider) WatchTargets(watch func() error) {
	klog.V(1).Infof("WatchTargets not supported for auth-headers provider. Ignoring watch function.")
}

func (p *AuthHeadersClusterProvider) Close() {
}
