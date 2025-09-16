package kubernetes

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestManager_Derived(t *testing.T) {
	// Create a temporary kubeconfig file for testing
	tempDir := t.TempDir()
	kubeconfigPath := path.Join(tempDir, "config")
	kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-cluster.example.com
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    username: test-username
    password: test-password
`
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644); err != nil {
		t.Fatalf("failed to create kubeconfig file: %v", err)
	}

	t.Run("without authorization header returns original manager", func(t *testing.T) {
		testStaticConfig := &config.StaticConfig{
			KubeConfig:    kubeconfigPath,
			DisabledTools: []string{"configuration_view"},
			DeniedResources: []config.GroupVersionKind{
				{Group: "apps", Version: "v1", Kind: "Deployment"},
			},
		}

		testManager, err := NewManager(testStaticConfig)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}
		defer testManager.Close()
		ctx := context.Background()
		derived, err := testManager.Derived(ctx)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		if derived.manager != testManager {
			t.Errorf("expected original manager, got different manager")
		}
	})

	t.Run("with invalid authorization header returns original manager", func(t *testing.T) {
		testStaticConfig := &config.StaticConfig{
			KubeConfig:    kubeconfigPath,
			DisabledTools: []string{"configuration_view"},
			DeniedResources: []config.GroupVersionKind{
				{Group: "apps", Version: "v1", Kind: "Deployment"},
			},
		}

		testManager, err := NewManager(testStaticConfig)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}
		defer testManager.Close()
		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "invalid-token")
		derived, err := testManager.Derived(ctx)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		if derived.manager != testManager {
			t.Errorf("expected original manager, got different manager")
		}
	})

	t.Run("with valid bearer token creates derived manager with correct configuration", func(t *testing.T) {
		testStaticConfig := &config.StaticConfig{
			KubeConfig:    kubeconfigPath,
			DisabledTools: []string{"configuration_view"},
			DeniedResources: []config.GroupVersionKind{
				{Group: "apps", Version: "v1", Kind: "Deployment"},
			},
		}

		testManager, err := NewManager(testStaticConfig)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}
		defer testManager.Close()
		testBearerToken := "test-bearer-token-123"
		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer "+testBearerToken)
		derived, err := testManager.Derived(ctx)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		if derived.manager == testManager {
			t.Errorf("expected new derived manager, got original manager")
		}

		if derived.manager.staticConfig != testStaticConfig {
			t.Errorf("staticConfig not properly wired to derived manager")
		}

		derivedCfg := derived.manager.cfg
		if derivedCfg == nil {
			t.Fatalf("derived config is nil")
		}

		originalCfg := testManager.cfg
		if derivedCfg.Host != originalCfg.Host {
			t.Errorf("expected Host %s, got %s", originalCfg.Host, derivedCfg.Host)
		}
		if derivedCfg.APIPath != originalCfg.APIPath {
			t.Errorf("expected APIPath %s, got %s", originalCfg.APIPath, derivedCfg.APIPath)
		}
		if derivedCfg.QPS != originalCfg.QPS {
			t.Errorf("expected QPS %f, got %f", originalCfg.QPS, derivedCfg.QPS)
		}
		if derivedCfg.Burst != originalCfg.Burst {
			t.Errorf("expected Burst %d, got %d", originalCfg.Burst, derivedCfg.Burst)
		}
		if derivedCfg.Timeout != originalCfg.Timeout {
			t.Errorf("expected Timeout %v, got %v", originalCfg.Timeout, derivedCfg.Timeout)
		}

		if derivedCfg.Insecure != originalCfg.Insecure {
			t.Errorf("expected TLS Insecure %v, got %v", originalCfg.Insecure, derivedCfg.Insecure)
		}
		if derivedCfg.ServerName != originalCfg.ServerName {
			t.Errorf("expected TLS ServerName %s, got %s", originalCfg.ServerName, derivedCfg.ServerName)
		}
		if derivedCfg.CAFile != originalCfg.CAFile {
			t.Errorf("expected TLS CAFile %s, got %s", originalCfg.CAFile, derivedCfg.CAFile)
		}
		if string(derivedCfg.CAData) != string(originalCfg.CAData) {
			t.Errorf("expected TLS CAData %s, got %s", string(originalCfg.CAData), string(derivedCfg.CAData))
		}

		if derivedCfg.BearerToken != testBearerToken {
			t.Errorf("expected BearerToken %s, got %s", testBearerToken, derivedCfg.BearerToken)
		}
		if derivedCfg.UserAgent != CustomUserAgent {
			t.Errorf("expected UserAgent %s, got %s", CustomUserAgent, derivedCfg.UserAgent)
		}

		// Verify that sensitive fields are NOT copied to prevent credential leakage
		// The derived config should only use the bearer token from the Authorization header
		// and not inherit any authentication credentials from the original kubeconfig
		if derivedCfg.CertFile != "" {
			t.Errorf("expected TLS CertFile to be empty, got %s", derivedCfg.CertFile)
		}
		if derivedCfg.KeyFile != "" {
			t.Errorf("expected TLS KeyFile to be empty, got %s", derivedCfg.KeyFile)
		}
		if len(derivedCfg.CertData) != 0 {
			t.Errorf("expected TLS CertData to be empty, got %v", derivedCfg.CertData)
		}
		if len(derivedCfg.KeyData) != 0 {
			t.Errorf("expected TLS KeyData to be empty, got %v", derivedCfg.KeyData)
		}

		if derivedCfg.Username != "" {
			t.Errorf("expected Username to be empty, got %s", derivedCfg.Username)
		}
		if derivedCfg.Password != "" {
			t.Errorf("expected Password to be empty, got %s", derivedCfg.Password)
		}
		if derivedCfg.AuthProvider != nil {
			t.Errorf("expected AuthProvider to be nil, got %v", derivedCfg.AuthProvider)
		}
		if derivedCfg.ExecProvider != nil {
			t.Errorf("expected ExecProvider to be nil, got %v", derivedCfg.ExecProvider)
		}
		if derivedCfg.BearerTokenFile != "" {
			t.Errorf("expected BearerTokenFile to be empty, got %s", derivedCfg.BearerTokenFile)
		}
		if derivedCfg.Impersonate.UserName != "" {
			t.Errorf("expected Impersonate.UserName to be empty, got %s", derivedCfg.Impersonate.UserName)
		}

		// Verify that the original manager still has the sensitive data
		if originalCfg.Username == "" && originalCfg.Password == "" {
			t.Logf("original kubeconfig shouldn't be modified")
		}

		// Verify that the derived manager has proper clients initialized
		if derived.manager.accessControlClientSet == nil {
			t.Error("expected accessControlClientSet to be initialized")
		}
		if derived.manager.accessControlClientSet.staticConfig != testStaticConfig {
			t.Errorf("staticConfig not properly wired to derived manager")
		}
		if derived.manager.discoveryClient == nil {
			t.Error("expected discoveryClient to be initialized")
		}
		if derived.manager.accessControlRESTMapper == nil {
			t.Error("expected accessControlRESTMapper to be initialized")
		}
		if derived.manager.accessControlRESTMapper.staticConfig != testStaticConfig {
			t.Errorf("staticConfig not properly wired to derived manager")
		}
		if derived.manager.dynamicClient == nil {
			t.Error("expected dynamicClient to be initialized")
		}
	})

	t.Run("with RequireOAuth=true and no authorization header returns oauth token required error", func(t *testing.T) {
		testStaticConfig := &config.StaticConfig{
			KubeConfig:    kubeconfigPath,
			RequireOAuth:  true,
			DisabledTools: []string{"configuration_view"},
			DeniedResources: []config.GroupVersionKind{
				{Group: "apps", Version: "v1", Kind: "Deployment"},
			},
		}

		testManager, err := NewManager(testStaticConfig)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}
		defer testManager.Close()
		ctx := context.Background()
		derived, err := testManager.Derived(ctx)
		if err == nil {
			t.Fatal("expected error for missing oauth token, got nil")
		}
		if err.Error() != "oauth token required" {
			t.Fatalf("expected error 'oauth token required', got %s", err.Error())
		}
		if derived != nil {
			t.Error("expected nil derived manager when oauth token required")
		}
	})

	t.Run("with RequireOAuth=true and invalid authorization header returns oauth token required error", func(t *testing.T) {
		testStaticConfig := &config.StaticConfig{
			KubeConfig:    kubeconfigPath,
			RequireOAuth:  true,
			DisabledTools: []string{"configuration_view"},
			DeniedResources: []config.GroupVersionKind{
				{Group: "apps", Version: "v1", Kind: "Deployment"},
			},
		}

		testManager, err := NewManager(testStaticConfig)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}
		defer testManager.Close()
		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "invalid-token")
		derived, err := testManager.Derived(ctx)
		if err == nil {
			t.Fatal("expected error for invalid oauth token, got nil")
		}
		if err.Error() != "oauth token required" {
			t.Fatalf("expected error 'oauth token required', got %s", err.Error())
		}
		if derived != nil {
			t.Error("expected nil derived manager when oauth token required")
		}
	})

	t.Run("with RequireOAuth=true and valid bearer token creates derived manager", func(t *testing.T) {
		testStaticConfig := &config.StaticConfig{
			KubeConfig:    kubeconfigPath,
			RequireOAuth:  true,
			DisabledTools: []string{"configuration_view"},
			DeniedResources: []config.GroupVersionKind{
				{Group: "apps", Version: "v1", Kind: "Deployment"},
			},
		}

		testManager, err := NewManager(testStaticConfig)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}
		defer testManager.Close()
		testBearerToken := "test-bearer-token-123"
		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer "+testBearerToken)
		derived, err := testManager.Derived(ctx)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		if derived.manager == testManager {
			t.Error("expected new derived manager, got original manager")
		}

		if derived.manager.staticConfig != testStaticConfig {
			t.Error("staticConfig not properly wired to derived manager")
		}

		derivedCfg := derived.manager.cfg
		if derivedCfg == nil {
			t.Fatal("derived config is nil")
		}

		if derivedCfg.BearerToken != testBearerToken {
			t.Errorf("expected BearerToken %s, got %s", testBearerToken, derivedCfg.BearerToken)
		}
	})
}

func TestKubernetes_WithContext(t *testing.T) {
	// Create a temporary kubeconfig file with multiple contexts for testing
	tempDir := t.TempDir()
	kubeconfigPath := path.Join(tempDir, "config")
	kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://production-cluster.example.com
  name: production-cluster
- cluster:
    server: https://staging-cluster.example.com
  name: staging-cluster
- cluster:
    server: https://development-cluster.example.com
  name: development-cluster
contexts:
- context:
    cluster: production-cluster
    user: prod-user
  name: production
- context:
    cluster: staging-cluster
    user: staging-user
  name: staging
- context:
    cluster: development-cluster
    user: dev-user
  name: development
current-context: production
users:
- name: prod-user
  user:
    username: prod-username
    password: prod-password
- name: staging-user
  user:
    username: staging-username
    password: staging-password
- name: dev-user
  user:
    username: dev-username
    password: dev-password
`
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644), "failed to create kubeconfig file")

	testStaticConfig := &config.StaticConfig{
		KubeConfig: kubeconfigPath,
	}

	testManager, err := NewManager(testStaticConfig)
	require.NoError(t, err, "failed to create manager")
	defer testManager.Close()

	originalK8s := &Kubernetes{manager: testManager}

	t.Run("WithContext with valid context creates new Kubernetes instance", func(t *testing.T) {
		contextK8s, err := originalK8s.WithContext("staging")
		require.NoError(t, err, "WithContext should not return error")

		// Verify new instance is created
		assert.NotSame(t, contextK8s, originalK8s, "expected new Kubernetes instance, got original instance")

		// Verify new manager is created
		assert.NotSame(t, contextK8s.manager, originalK8s.manager, "expected new manager, got original manager")

		// Verify that all necessary clients are properly initialized
		assert.NotNil(t, contextK8s.manager.accessControlClientSet, "expected accessControlClientSet to be initialized")
		assert.NotNil(t, contextK8s.manager.discoveryClient, "expected discoveryClient to be initialized")
		assert.NotNil(t, contextK8s.manager.accessControlRESTMapper, "expected accessControlRESTMapper to be initialized")
		assert.NotNil(t, contextK8s.manager.dynamicClient, "expected dynamicClient to be initialized")

		// Verify static config is preserved
		assert.Same(t, contextK8s.manager.staticConfig, testStaticConfig, "staticConfig not properly wired to context manager")

		// Verify the context-specific configuration
		require.NotNil(t, contextK8s.manager.cfg, "context config should not be nil")
		assert.Equal(t, "https://staging-cluster.example.com", contextK8s.manager.cfg.Host, "Host should point to staging cluster")
	})

	t.Run("WithContext with different context creates different instance", func(t *testing.T) {
		stagingK8s, err := originalK8s.WithContext("staging")
		require.NoError(t, err, "WithContext should not return error for staging")

		devK8s, err := originalK8s.WithContext("development")
		require.NoError(t, err, "WithContext should not return error for development")

		// Verify different instances are created
		assert.NotSame(t, stagingK8s, devK8s, "expected different instances for different contexts")
		assert.NotSame(t, stagingK8s.manager, devK8s.manager, "expected different managers for different contexts")

		// Verify different configurations
		require.NotNil(t, stagingK8s.manager.cfg, "staging config should not be nil")
		require.NotNil(t, devK8s.manager.cfg, "development config should not be nil")

		assert.Equal(t, "https://staging-cluster.example.com", stagingK8s.manager.cfg.Host, "staging Host should point to staging cluster")
		assert.Equal(t, "https://development-cluster.example.com", devK8s.manager.cfg.Host, "development Host should point to development cluster")
	})

	t.Run("WithContext with empty context name returns same manager", func(t *testing.T) {
		contextK8s, err := originalK8s.WithContext("")
		require.NoError(t, err, "WithContext with empty name should not fail")

		// Should return same manager for empty context
		assert.Same(t, contextK8s.manager, originalK8s.manager, "expected same manager for empty context name")
	})

	t.Run("WithContext with nonexistent context returns error", func(t *testing.T) {
		_, err := originalK8s.WithContext("nonexistent-context")
		require.Error(t, err, "expected error for nonexistent context")
		assert.Contains(t, err.Error(), "context", "error should mention context")
	})

	t.Run("WithContext from in-cluster returns same manager", func(t *testing.T) {
		// Create in-cluster config
		inClusterStaticConfig := &config.StaticConfig{}

		// Mock in-cluster configuration (this will fail to create actual clients, but tests the path)
		originalInClusterConfig := InClusterConfig
		InClusterConfig = func() (*rest.Config, error) {
			return &rest.Config{
				Host: "https://kubernetes.default.svc",
			}, nil
		}
		defer func() {
			InClusterConfig = originalInClusterConfig
		}()

		inClusterManager, err := NewManager(inClusterStaticConfig)
		require.NoError(t, err, "failed to create in-cluster manager")
		defer inClusterManager.Close()

		inClusterK8s := &Kubernetes{manager: inClusterManager}

		contextK8s, err := inClusterK8s.WithContext("any-context")
		require.NoError(t, err, "WithContext from in-cluster should not fail")

		// Should return same manager for in-cluster mode
		assert.Same(t, contextK8s.manager, inClusterK8s.manager, "expected same manager for in-cluster mode")
	})
}
