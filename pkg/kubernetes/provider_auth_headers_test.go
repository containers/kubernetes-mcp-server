package kubernetes

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthHeadersProviderFactory(t *testing.T) {
	t.Run("auth-headers provider initializes without kubeconfig", func(t *testing.T) {
		cfg := &config.StaticConfig{
			ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
		}

		provider, err := newAuthHeadersClusterProvider(cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)
		assert.IsType(t, &AuthHeadersClusterProvider{}, provider)
	})

	t.Run("auth-headers provider initializes with minimal config", func(t *testing.T) {
		cfg := &config.StaticConfig{
			ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
		}

		provider, err := newAuthHeadersClusterProvider(cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)
	})
}

func TestAuthHeadersProviderInterface(t *testing.T) {
	cfg := &config.StaticConfig{
		ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
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

	t.Run("IsOpenShift returns false", func(t *testing.T) {
		assert.False(t, provider.IsOpenShift(context.Background()))
	})

	t.Run("VerifyToken not supported", func(t *testing.T) {
		_, _, err := provider.VerifyToken(context.Background(), "", "token", "audience")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
	})

	t.Run("WatchTargets does nothing", func(t *testing.T) {
		called := false
		provider.WatchTargets(func() error {
			called = true
			return nil
		})
		// WatchTargets should not call the function
		assert.False(t, called)
	})

	t.Run("Close does nothing", func(t *testing.T) {
		// Should not panic
		provider.Close()
	})
}

func TestAuthHeadersProviderGetDerivedKubernetes(t *testing.T) {
	mockServer := test.NewMockServer()
	defer mockServer.Close()

	cfg := &config.StaticConfig{
		ClusterProviderStrategy: config.ClusterProviderAuthHeaders,
	}

	provider, err := newAuthHeadersClusterProvider(cfg)
	require.NoError(t, err)

	// Generate test CA certificate data in valid PEM format
	caCert := []byte(`-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHHCgVZU8BiMA0GCSqGSIb3DQEBBQUAMA0xCzAJBgNVBAYTAlVT
MB4XDTA5MDUxOTE1MTc1N1oXDTEwMDUxOTE1MTc1N1owDTELMAkGA1UEBhMCVVMw
gZ8wDQYJKoZIhvcNAQEBBQADgY0AMIGJAoGBANLJhPHhITqQbPklG3ibCVxwGMRf
p/v4XqhfdQHdcVfHap6NQ5Wok/9X5gK7d1ONlGjn/Ut9Pz4xwqGy3nLxVz1CsE2k
TqQxdqEQBVNvFrAB4OlD9K9wQ3R+0S1wPPQ9yg9i6vF2JlOvD1HFJzIGcz1kLZU2
wj5FqYY5SHmXF2YbAgMBAAEwDQYJKoZIhvcNAQEFBQADgYEAc9NQIv8J/cqV0zBX
c6d5Wm1NJdTxYwG/+xHDaLDK8R3W5Y1e7YwNg7nN8K2GqMh3YYxmDJCLDhGdKDEV
V5qHcKhFCFPxTmKgzVjy8vhR7VqZU4dJhC8sDbE/IkKH7hBo7CLHH/T2Ly9LcDY0
9C2zNtDN3KEzGW3V7/J7IvVBDy0=
-----END CERTIFICATE-----`)
	caCertBase64 := base64.StdEncoding.EncodeToString(caCert)

	t.Run("GetDerivedKubernetes requires auth headers in context", func(t *testing.T) {
		ctx := context.Background()
		_, err := provider.GetDerivedKubernetes(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authHeaders required")
	})

	t.Run("GetDerivedKubernetes works with token authentication", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			Server:                   mockServer.Config().Host,
			CertificateAuthorityData: nil,
			AuthorizationToken:       "test-token",
			InsecureSkipTLSVerify:    true,
		}

		ctx := context.WithValue(context.Background(), AuthHeadersContextKey, authHeaders)
		k, err := provider.GetDerivedKubernetes(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, k)
		assert.NotNil(t, k.manager)
	})

	t.Run("GetDerivedKubernetes accepts client certificate authentication", func(t *testing.T) {
		// Note: We use dummy cert/key data since we can't easily create valid certificates for testing.
		// The actual validation happens when connecting to the cluster, not during manager creation.
		clientCert := []byte("dummy-cert")
		clientKey := []byte("dummy-key")

		authHeaders := &K8sAuthHeaders{
			Server:                   mockServer.Config().Host,
			CertificateAuthorityData: nil,
			ClientCertificateData:    clientCert,
			ClientKeyData:            clientKey,
			InsecureSkipTLSVerify:    true,
			AuthorizationToken:       "", // No token when using client cert
		}

		// This should fail because the certificates are invalid, but we're testing that the provider
		// accepts the auth headers and attempts to create the manager
		ctx := context.WithValue(context.Background(), AuthHeadersContextKey, authHeaders)
		_, err := provider.GetDerivedKubernetes(ctx, "")
		// Expect an error about invalid certificates, which means the provider accepted the headers
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create auth headers cluster manager")
	})

	t.Run("GetDerivedKubernetes works with insecure skip TLS verify", func(t *testing.T) {
		authHeaders := &K8sAuthHeaders{
			Server:                   mockServer.Config().Host,
			CertificateAuthorityData: nil, // Don't provide CA data when skipping TLS verification
			AuthorizationToken:       "test-token",
			InsecureSkipTLSVerify:    true,
		}

		ctx := context.WithValue(context.Background(), AuthHeadersContextKey, authHeaders)
		k, err := provider.GetDerivedKubernetes(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, k)
	})

	t.Run("NewK8sAuthHeadersFromHeaders parses token auth correctly", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   mockServer.Config().Host,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            "Bearer test-token",
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		assert.Equal(t, mockServer.Config().Host, authHeaders.Server)
		assert.Equal(t, caCert, authHeaders.CertificateAuthorityData)
		assert.Equal(t, "Bearer test-token", authHeaders.AuthorizationToken)
		assert.False(t, authHeaders.InsecureSkipTLSVerify)
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("NewK8sAuthHeadersFromHeaders parses cert auth correctly", func(t *testing.T) {
		clientCert := []byte("test-client-cert")
		clientKey := []byte("test-client-key")
		clientCertBase64 := base64.StdEncoding.EncodeToString(clientCert)
		clientKeyBase64 := base64.StdEncoding.EncodeToString(clientKey)

		headers := map[string]any{
			string(CustomServerHeader):                   mockServer.Config().Host,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomClientCertificateDataHeader):    clientCertBase64,
			string(CustomClientKeyDataHeader):            clientKeyBase64,
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		assert.Equal(t, mockServer.Config().Host, authHeaders.Server)
		assert.Equal(t, caCert, authHeaders.CertificateAuthorityData)
		assert.Equal(t, clientCert, authHeaders.ClientCertificateData)
		assert.Equal(t, clientKey, authHeaders.ClientKeyData)
		assert.True(t, authHeaders.IsValid())
	})

	t.Run("NewK8sAuthHeadersFromHeaders requires server header", func(t *testing.T) {
		headers := map[string]any{
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            "Bearer test-token",
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kubernetes-server")
	})

	t.Run("NewK8sAuthHeadersFromHeaders requires CA data header", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):        mockServer.Config().Host,
			string(CustomAuthorizationHeader): "Bearer test-token",
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kubernetes-certificate-authority-data")
	})

	t.Run("NewK8sAuthHeadersFromHeaders requires valid auth method", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   mockServer.Config().Host,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication")
	})

	t.Run("NewK8sAuthHeadersFromHeaders handles insecure skip TLS verify", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   mockServer.Config().Host,
			string(CustomCertificateAuthorityDataHeader): caCertBase64,
			string(CustomAuthorizationHeader):            "Bearer test-token",
			string(CustomInsecureSkipTLSVerifyHeader):    "true",
		}

		authHeaders, err := NewK8sAuthHeadersFromHeaders(headers)
		require.NoError(t, err)
		assert.True(t, authHeaders.InsecureSkipTLSVerify)
	})

	t.Run("NewK8sAuthHeadersFromHeaders handles invalid base64 CA data", func(t *testing.T) {
		headers := map[string]any{
			string(CustomServerHeader):                   mockServer.Config().Host,
			string(CustomCertificateAuthorityDataHeader): "invalid-base64!!!",
			string(CustomAuthorizationHeader):            "Bearer test-token",
		}

		_, err := NewK8sAuthHeadersFromHeaders(headers)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "certificate authority data")
	})
}
