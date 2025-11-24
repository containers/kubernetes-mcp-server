package kubernetes

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestAuthHeadersProviderFactory(t *testing.T) {
	t.Run("auth-headers provider requires kubeconfig for cluster connection info", func(t *testing.T) {
		cfg := &config.StaticConfig{
			KubeConfig:              "",
			ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
		}
		_, err := newAuthHeadersClusterProvider(cfg)
		require.Error(t, err)
	})

	t.Run("auth-headers provider initializes with valid kubeconfig", func(t *testing.T) {
		mockServer := test.NewMockServer()
		defer mockServer.Close()

		cfg := &config.StaticConfig{
			KubeConfig:              mockServer.KubeconfigFile(t),
			ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
			RequireOAuth:            true,
		}

		provider, err := newAuthHeadersClusterProvider(cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)
		assert.IsType(t, &AuthHeadersClusterProvider{}, provider)
	})

	t.Run("auth-headers provider automatically enables RequireOAuth", func(t *testing.T) {
		mockServer := test.NewMockServer()
		defer mockServer.Close()

		cfg := &config.StaticConfig{
			KubeConfig:              mockServer.KubeconfigFile(t),
			ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
			RequireOAuth:            false, // Will be forced to true
		}

		provider, err := newAuthHeadersClusterProvider(cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)
		assert.True(t, cfg.RequireOAuth, "RequireOAuth should be forced to true")
	})

	t.Run("auth-headers provider rejects in-cluster config", func(t *testing.T) {
		cfg := &config.StaticConfig{
			ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
		}

		// Temporarily mock IsInCluster to return true
		originalInClusterConfig := InClusterConfig
		InClusterConfig = func() (*rest.Config, error) {
			return &rest.Config{Host: "https://kubernetes.default.svc"}, nil
		}
		defer func() { InClusterConfig = originalInClusterConfig }()

		_, err := newAuthHeadersClusterProvider(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be used in in-cluster deployments")
	})
}

func TestAuthHeadersProviderInterface(t *testing.T) {
	mockServer := test.NewMockServer()
	defer mockServer.Close()

	cfg := &config.StaticConfig{
		KubeConfig:              mockServer.KubeconfigFile(t),
		ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
		RequireOAuth:            true,
	}

	provider, err := newAuthHeadersClusterProvider(cfg)
	require.NoError(t, err)

	t.Run("GetTargets returns single empty target", func(t *testing.T) {
		targets, err := provider.GetTargets(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{""}, targets)
	})

	t.Run("GetTargetParameterName returns empty string", func(t *testing.T) {
		assert.Equal(t, "", provider.GetTargetParameterName())
	})

	t.Run("GetDefaultTarget returns empty string", func(t *testing.T) {
		assert.Equal(t, "", provider.GetDefaultTarget())
	})

	t.Run("GetDerivedKubernetes requires token in context", func(t *testing.T) {
		ctx := context.Background()
		_, err := provider.GetDerivedKubernetes(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("GetDerivedKubernetes works with valid bearer token", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer test-token")
		k, err := provider.GetDerivedKubernetes(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, k)
	})

	t.Run("GetDerivedKubernetes rejects non-empty target", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer test-token")
		_, err := provider.GetDerivedKubernetes(ctx, "some-cluster")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not support multiple targets")
	})

	t.Run("VerifyToken rejects non-empty target", func(t *testing.T) {
		_, _, err := provider.VerifyToken(context.Background(), "some-cluster", "token", "audience")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not support multiple targets")
	})
}
