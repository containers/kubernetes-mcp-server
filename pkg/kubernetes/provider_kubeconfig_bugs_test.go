package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/require"
)

// TestPerTargetCacheInvalidatedOnTokenURLRotation verifies that the per-target
// token-exchange cache is invalidated when the OIDC token endpoint rotates.
// TokenURL is included in the cache key, so each distinct endpoint URL produces
// a distinct cache entry rather than reusing a stale config.
func TestPerTargetCacheInvalidatedOnTokenURLRotation(t *testing.T) {
	ctx := context.Background()

	kubeconfigPath := createTestKubeconfigFile(t, "spoke-context")
	configPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[token_exchange]
		strategy = "rfc8693"
		audience = "openshift"

		[token_exchange.client_auth]
		method = "client_secret_basic"
		client_id = "global-client"
		client_secret = "global-secret"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.spoke-context]
		audience = "spoke"
	`)

	cfg, err := config.Read(ctx, configPath, "")
	require.NoError(t, err)

	provider, err := newKubeConfigClusterProvider(ctx, cfg)
	require.NoError(t, err)
	defer provider.Close()

	kcProvider := provider.(*kubeConfigClusterProvider)

	// First request: OIDC discovery returned the old endpoint.
	first, err := kcProvider.GetTargetTokenExchangeConfig("spoke-context", cfg, "https://old-idp.example.com/token")
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, "https://old-idp.example.com/token", first.GetTokenURL())

	// Cache hit: identical key (same URL) returns the same pointer.
	same, err := kcProvider.GetTargetTokenExchangeConfig("spoke-context", cfg, "https://old-idp.example.com/token")
	require.NoError(t, err)
	require.True(t, first == same, "identical cache key must return the same pointer")

	// OIDC provider rotates its token endpoint — next request carries the new URL.
	// Because TokenURL is part of the cache key, this is a cache miss and a new
	// entry is built.
	second, err := kcProvider.GetTargetTokenExchangeConfig("spoke-context", cfg, "https://new-idp.example.com/token")
	require.NoError(t, err)
	require.NotNil(t, second)
	require.True(t, first != second, "rotated URL must not return the same cached pointer")
	require.Equal(t, "https://new-idp.example.com/token", second.GetTokenURL(),
		"per-target config must carry the rotated OIDC token URL")
	// The old config is immutable after build — its URL is unchanged.
	require.Equal(t, "https://old-idp.example.com/token", first.GetTokenURL(),
		"old cached config must retain its original URL")
}

// TestPerTargetClientIDOverrideWithPrivateKeyJWTAuth verifies that a per-target
// client_id override is applied when the global auth style is private_key_jwt
// (assertion). Previously, the &&-guard requiring both client_id AND
// client_secret dropped client_id-only overrides for certificate-based auth.
func TestPerTargetClientIDOverrideWithPrivateKeyJWTAuth(t *testing.T) {
	ctx := context.Background()

	// Create placeholder cert/key files — config validation checks existence but
	// not content; the actual JWT is only built during token exchange.
	certFile := filepath.Join(t.TempDir(), "cert.pem")
	keyFile := filepath.Join(t.TempDir(), "key.pem")
	require.NoError(t, os.WriteFile(certFile, []byte("placeholder"), 0o600))
	require.NoError(t, os.WriteFile(keyFile, []byte("placeholder"), 0o600))

	kubeconfigPath := createTestKubeconfigFile(t, "spoke-context")
	configPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[token_exchange]
		strategy = "rfc8693"
		audience = "openshift"

		[token_exchange.client_auth]
		method = "private_key_jwt"
		client_id = "global-client"
		certificate_file = "`+certFile+`"
		private_key_file = "`+keyFile+`"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.spoke-context]
		audience = "spoke"
		client_id = "spoke-client"
		# private_key_jwt does not use client_secret; client_id is overridable alone
	`)

	cfg, err := config.Read(ctx, configPath, "")
	require.NoError(t, err)

	provider, err := newKubeConfigClusterProvider(ctx, cfg)
	require.NoError(t, err)
	defer provider.Close()

	result, err := provider.(*kubeConfigClusterProvider).GetTargetTokenExchangeConfig("spoke-context", cfg, "")
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "spoke-client", result.ClientID,
		"per-target client_id must override global for auth styles that do not use client_secret")
}

// TestPerTargetSubjectIssuerPropagated verifies that cross-realm Keycloak token
// exchange can be configured via per-target subject_issuer, and that the value
// reaches the built config. KubeConfigTargetConfig must carry a subject_issuer
// field or the value is silently dropped during TOML parsing.
func TestPerTargetSubjectIssuerPropagated(t *testing.T) {
	ctx := context.Background()

	kubeconfigPath := createTestKubeconfigFile(t, "spoke-context")
	configPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[token_exchange]
		strategy = "rfc8693"
		audience = "openshift"

		[token_exchange.client_auth]
		method = "client_secret_basic"
		client_id = "global-client"
		client_secret = "global-secret"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.spoke-context]
		audience = "spoke"
		subject_issuer = "external-keycloak"
	`)

	cfg, err := config.Read(ctx, configPath, "")
	require.NoError(t, err)

	provider, err := newKubeConfigClusterProvider(ctx, cfg)
	require.NoError(t, err)
	defer provider.Close()

	result, err := provider.(*kubeConfigClusterProvider).GetTargetTokenExchangeConfig("spoke-context", cfg, "")
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "external-keycloak", result.SubjectIssuer,
		"per-target subject_issuer must be propagated for cross-realm Keycloak exchange")
}

// TestHotReloadRemovingTokenExchangeBlockReturnsError verifies that removing the
// global [token_exchange] block via a SIGHUP (while per-target exchange is still
// configured) causes ExchangeTokenInContext to return an error rather than
// silently passing the unexchanged token to the spoke cluster.
func TestHotReloadRemovingTokenExchangeBlockReturnsError(t *testing.T) {
	ctx := context.Background()

	kubeconfigPath := createTestKubeconfigFile(t, "spoke-context")

	// Initial valid config — per-target exchange is configured alongside [token_exchange].
	validConfigPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[token_exchange]
		strategy = "rfc8693"
		audience = "openshift"

		[token_exchange.client_auth]
		method = "client_secret_basic"
		client_id = "global-client"
		client_secret = "global-secret"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.spoke-context]
		audience = "spoke"
	`)

	cfg, err := config.Read(ctx, validConfigPath, "")
	require.NoError(t, err)

	provider, err := newKubeConfigClusterProvider(ctx, cfg)
	require.NoError(t, err)
	defer provider.Close()

	// Simulate a misconfigured SIGHUP that drops [token_exchange].
	// config.Read succeeds (the cross-config constraint is only validated at
	// provider creation time, not here). The provider is not recreated.
	hotReloadedConfigPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.spoke-context]
		audience = "spoke"
	`)

	hotReloadedCfg, err := config.Read(ctx, hotReloadedConfigPath, "")
	require.NoError(t, err)

	// ExchangeTokenInContext is called with the hot-reloaded config (no
	// [token_exchange]) and a nil globalConfig (OIDC discovery not set up).
	bearerCtx := context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer test-token")
	_, exchangeErr := ExchangeTokenInContext(bearerCtx, hotReloadedCfg, provider, "spoke-context", nil)

	require.Error(t, exchangeErr,
		"removing [token_exchange] while per-target exchange is configured must surface an error")
}

// TestCloseClearsTokenExchangeCache verifies that Close() clears the cached
// per-target token exchange configs.
func TestCloseClearsTokenExchangeCache(t *testing.T) {
	ctx := context.Background()

	kubeconfigPath := createTestKubeconfigFile(t, "test-context")
	configPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[token_exchange]
		strategy = "rfc8693"
		audience = "openshift"

		[token_exchange.client_auth]
		method = "client_secret_basic"
		client_id = "global-client"
		client_secret = "global-secret"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.test-context]
		audience = "target-audience"
	`)

	cfg, err := config.Read(ctx, configPath, "")
	require.NoError(t, err)

	provider, err := newKubeConfigClusterProvider(ctx, cfg)
	require.NoError(t, err)

	kcProvider := provider.(*kubeConfigClusterProvider)

	// Populate cache
	targetConfig, err := kcProvider.GetTargetTokenExchangeConfig("test-context", cfg, "")
	require.NoError(t, err)
	require.NotNil(t, targetConfig)

	kcProvider.targetConfigsMu.Lock()
	require.Len(t, kcProvider.targetConfigs, 1, "Cache should be populated")
	kcProvider.targetConfigsMu.Unlock()

	// Close should clean up the cache
	kcProvider.Close()

	kcProvider.targetConfigsMu.Lock()
	defer kcProvider.targetConfigsMu.Unlock()
	require.Empty(t, kcProvider.targetConfigs, "Close() should clear targetConfigs")
}

// TestCacheInvalidatedOnCredentialRotation verifies that when the cache key
// changes (e.g. SIGHUP rotates client_secret), the cached config is rebuilt so
// the new credentials are used instead of the stale ones.
func TestCacheInvalidatedOnCredentialRotation(t *testing.T) {
	ctx := context.Background()

	kubeconfigPath := createTestKubeconfigFile(t, "test-context")
	configPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[token_exchange]
		strategy = "rfc8693"
		audience = "openshift"

		[token_exchange.client_auth]
		method = "client_secret_basic"
		client_id = "global-client"
		client_secret = "global-secret-v1"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.test-context]
		audience = "target-audience"
	`)

	cfg, err := config.Read(ctx, configPath, "")
	require.NoError(t, err)

	provider, err := newKubeConfigClusterProvider(ctx, cfg)
	require.NoError(t, err)
	defer provider.Close()

	kcProvider := provider.(*kubeConfigClusterProvider)

	oldConfig, err := kcProvider.GetTargetTokenExchangeConfig("test-context", cfg, "")
	require.NoError(t, err)
	require.NotNil(t, oldConfig)
	require.Equal(t, "global-secret-v1", oldConfig.ClientSecret)

	// Simulate SIGHUP credential rotation
	rotatedConfigPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[token_exchange]
		strategy = "rfc8693"
		audience = "openshift"

		[token_exchange.client_auth]
		method = "client_secret_basic"
		client_id = "global-client"
		client_secret = "global-secret-v2-rotated"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.test-context]
		audience = "target-audience"
	`)

	rotatedCfg, err := config.Read(ctx, rotatedConfigPath, "")
	require.NoError(t, err)

	newConfig, err := kcProvider.GetTargetTokenExchangeConfig("test-context", rotatedCfg, "")
	require.NoError(t, err)
	require.NotNil(t, newConfig)
	require.Equal(t, "global-secret-v2-rotated", newConfig.ClientSecret)

	// Cache must reflect the rotated credentials
	kcProvider.targetConfigsMu.Lock()
	cached, ok := kcProvider.targetConfigs["test-context"]
	require.True(t, ok)
	require.Equal(t, "global-secret-v2-rotated", cached.config.ClientSecret)
	kcProvider.targetConfigsMu.Unlock()
}

// TestPerTargetPartialCredentialsFallBackToGlobal verifies that a partial
// per-target client credential override (client_id without client_secret) falls
// back to the global credentials for secret-based auth styles, to avoid
// mismatched pairs.
func TestPerTargetPartialCredentialsFallBackToGlobal(t *testing.T) {
	ctx := context.Background()

	kubeconfigPath := createTestKubeconfigFile(t, "spoke-context")
	configPath := writeTestConfig(t, `
		cluster_provider_strategy = "kubeconfig"
		kubeconfig = "`+kubeconfigPath+`"
		cluster_auth_mode = "passthrough"

		[token_exchange]
		strategy = "rfc8693"
		audience = "openshift"

		[token_exchange.client_auth]
		method = "client_secret_basic"
		client_id = "global-client"
		client_secret = "global-secret"

		[cluster_provider_configs.kubeconfig]
		strategy = "rfc8693"

		[cluster_provider_configs.kubeconfig.targets.spoke-context]
		audience = "spoke"
		client_id = "spoke-client"
		# client_secret intentionally NOT set - should fall back to global
	`)

	cfg, err := config.Read(ctx, configPath, "")
	require.NoError(t, err)

	provider, err := newKubeConfigClusterProvider(ctx, cfg)
	require.NoError(t, err)

	kcProvider := provider.(*kubeConfigClusterProvider)
	result, err := kcProvider.GetTargetTokenExchangeConfig("spoke-context", cfg, "")
	require.NoError(t, err)
	require.NotNil(t, result)

	// For client_secret_basic, both client_id and client_secret must be provided
	// together to avoid mismatched pairs. When only client_id is set per-target,
	// both credentials fall back to global.
	require.Equal(t, "global-client", result.ClientID,
		"Partial client credential override should fall back to global")
	require.Equal(t, "global-secret", result.ClientSecret,
		"Should use matching client_secret from global config")
}

// Helper to create a minimal kubeconfig file for testing
func createTestKubeconfigFile(t testing.TB, contextName string) string {
	t.Helper()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "kubeconfig")

	kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: ` + contextName + `
current-context: ` + contextName + `
users:
- name: test-user
  user:
    token: test-token
`

	err := os.WriteFile(tmpFile, []byte(kubeconfigContent), 0600)
	require.NoError(t, err)

	return tmpFile
}

// Helper to write a test config file
func writeTestConfig(t testing.TB, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	err := os.WriteFile(configPath, []byte(content), 0600)
	require.NoError(t, err)

	return configPath
}
