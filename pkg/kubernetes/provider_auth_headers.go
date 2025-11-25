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
	return ""
}

func (p *AuthHeadersClusterProvider) WatchTargets(watch func() error) {
}

func (p *AuthHeadersClusterProvider) Close() {
}
