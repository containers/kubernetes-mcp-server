package kubernetes

import (
	"errors"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestKubernetes_IsInCluster(t *testing.T) {
	t.Run("with explicit kubeconfig", func(t *testing.T) {
		m := Manager{
			staticConfig: &config.StaticConfig{
				KubeConfig: "kubeconfig",
			},
		}
		if m.IsInCluster() {
			t.Errorf("expected not in cluster, got in cluster")
		}
	})
	t.Run("with empty kubeconfig and in cluster", func(t *testing.T) {
		originalFunction := InClusterConfig
		InClusterConfig = func() (*rest.Config, error) {
			return &rest.Config{}, nil
		}
		defer func() {
			InClusterConfig = originalFunction
		}()
		m := Manager{
			staticConfig: &config.StaticConfig{
				KubeConfig: "",
			},
		}
		if !m.IsInCluster() {
			t.Errorf("expected in cluster, got not in cluster")
		}
	})
	t.Run("with empty kubeconfig and not in cluster (empty)", func(t *testing.T) {
		originalFunction := InClusterConfig
		InClusterConfig = func() (*rest.Config, error) {
			return nil, nil
		}
		defer func() {
			InClusterConfig = originalFunction
		}()
		m := Manager{
			staticConfig: &config.StaticConfig{
				KubeConfig: "",
			},
		}
		if m.IsInCluster() {
			t.Errorf("expected not in cluster, got in cluster")
		}
	})
	t.Run("with empty kubeconfig and not in cluster (error)", func(t *testing.T) {
		originalFunction := InClusterConfig
		InClusterConfig = func() (*rest.Config, error) {
			return nil, errors.New("error")
		}
		defer func() {
			InClusterConfig = originalFunction
		}()
		m := Manager{
			staticConfig: &config.StaticConfig{
				KubeConfig: "",
			},
		}
		if m.IsInCluster() {
			t.Errorf("expected not in cluster, got in cluster")
		}
	})
}

func TestKubernetes_ResolveKubernetesConfigurations_Explicit(t *testing.T) {
	t.Run("with missing file", func(t *testing.T) {
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			t.Skip("Skipping test on non-linux platforms")
		}
		tempDir := t.TempDir()
		m := Manager{staticConfig: &config.StaticConfig{
			KubeConfig: path.Join(tempDir, "config"),
		}}
		err := resolveKubernetesConfigurations(&m)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected file not found error, got %v", err)
		}
		if !strings.HasSuffix(err.Error(), ": no such file or directory") {
			t.Errorf("expected file not found error, got %v", err)
		}
	})
	t.Run("with empty file", func(t *testing.T) {
		tempDir := t.TempDir()
		kubeconfigPath := path.Join(tempDir, "config")
		if err := os.WriteFile(kubeconfigPath, []byte(""), 0644); err != nil {
			t.Fatalf("failed to create kubeconfig file: %v", err)
		}
		m := Manager{staticConfig: &config.StaticConfig{
			KubeConfig: kubeconfigPath,
		}}
		err := resolveKubernetesConfigurations(&m)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no configuration has been provided") {
			t.Errorf("expected no kubeconfig error, got %v", err)
		}
	})
	t.Run("with valid file", func(t *testing.T) {
		tempDir := t.TempDir()
		kubeconfigPath := path.Join(tempDir, "config")
		kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://example.com
  name: example-cluster
contexts:
- context:
    cluster: example-cluster
    user: example-user
  name: example-context
current-context: example-context
users:
- name: example-user
  user:
    token: example-token
`
		if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644); err != nil {
			t.Fatalf("failed to create kubeconfig file: %v", err)
		}
		m := Manager{staticConfig: &config.StaticConfig{
			KubeConfig: kubeconfigPath,
		}}
		err := resolveKubernetesConfigurations(&m)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if m.cfg == nil {
			t.Errorf("expected non-nil config, got nil")
		}
		if m.cfg.Host != "https://example.com" {
			t.Errorf("expected host https://example.com, got %s", m.cfg.Host)
		}
	})
}

func TestKubernetes_ContextsList(t *testing.T) {
	t.Run("with multiple contexts returns correct context map", func(t *testing.T) {
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
current-context: staging
users:
- name: prod-user
  user:
    token: prod-token
- name: staging-user
  user:
    token: staging-token
- name: dev-user
  user:
    token: dev-token
`
		require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644), "failed to create kubeconfig file")

		testStaticConfig := &config.StaticConfig{
			KubeConfig: kubeconfigPath,
		}

		testManager, err := NewManager(testStaticConfig)
		require.NoError(t, err, "failed to create manager")
		defer testManager.Close()

		k8s := &Kubernetes{manager: testManager}

		contexts, currentContext, err := k8s.ContextsList()
		require.NoError(t, err, "ContextsList should not return error")

		// Verify contexts map
		expectedContexts := map[string]string{
			"production":  "https://production-cluster.example.com",
			"staging":     "https://staging-cluster.example.com",
			"development": "https://development-cluster.example.com",
		}
		assert.Equal(t, expectedContexts, contexts, "contexts should match expected map")

		// Verify current context
		assert.Equal(t, "staging", currentContext, "current context should be staging")
	})

	t.Run("with single context returns single context", func(t *testing.T) {
		tempDir := t.TempDir()
		kubeconfigPath := path.Join(tempDir, "config")
		kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://single-cluster.example.com
  name: single-cluster
contexts:
- context:
    cluster: single-cluster
    user: single-user
  name: single-context
current-context: single-context
users:
- name: single-user
  user:
    token: single-token
`
		require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644), "failed to create kubeconfig file")

		testStaticConfig := &config.StaticConfig{
			KubeConfig: kubeconfigPath,
		}

		testManager, err := NewManager(testStaticConfig)
		require.NoError(t, err, "failed to create manager")
		defer testManager.Close()

		k8s := &Kubernetes{manager: testManager}

		contexts, currentContext, err := k8s.ContextsList()
		require.NoError(t, err, "ContextsList should not return error")

		expectedContexts := map[string]string{
			"single-context": "https://single-cluster.example.com",
		}
		assert.Equal(t, expectedContexts, contexts, "contexts should match expected single context")
		assert.Equal(t, "single-context", currentContext, "current context should be single-context")
	})

	t.Run("with context referencing missing cluster marks as unknown", func(t *testing.T) {
		tempDir := t.TempDir()
		kubeconfigPath := path.Join(tempDir, "config")
		kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://existing-cluster.example.com
  name: existing-cluster
contexts:
- context:
    cluster: existing-cluster
    user: valid-user
  name: valid-context
- context:
    cluster: missing-cluster
    user: invalid-user
  name: invalid-context
current-context: valid-context
users:
- name: valid-user
  user:
    token: valid-token
- name: invalid-user
  user:
    token: invalid-token
`
		require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644), "failed to create kubeconfig file")

		testStaticConfig := &config.StaticConfig{
			KubeConfig: kubeconfigPath,
		}

		testManager, err := NewManager(testStaticConfig)
		require.NoError(t, err, "failed to create manager")
		defer testManager.Close()

		k8s := &Kubernetes{manager: testManager}

		contexts, currentContext, err := k8s.ContextsList()
		require.NoError(t, err, "ContextsList should not return error")

		expectedContexts := map[string]string{
			"valid-context":   "https://existing-cluster.example.com",
			"invalid-context": "unknown",
		}
		assert.Equal(t, expectedContexts, contexts, "contexts should include unknown for missing cluster")
		assert.Equal(t, "valid-context", currentContext, "current context should be valid-context")
	})

	t.Run("in-cluster returns single cluster context", func(t *testing.T) {
		// Mock in-cluster configuration
		originalInClusterConfig := InClusterConfig
		InClusterConfig = func() (*rest.Config, error) {
			return &rest.Config{
				Host: "https://kubernetes.default.svc",
			}, nil
		}
		defer func() {
			InClusterConfig = originalInClusterConfig
		}()

		inClusterStaticConfig := &config.StaticConfig{}
		inClusterManager, err := NewManager(inClusterStaticConfig)
		require.NoError(t, err, "failed to create in-cluster manager")
		defer inClusterManager.Close()

		k8s := &Kubernetes{manager: inClusterManager}

		contexts, currentContext, err := k8s.ContextsList()
		require.NoError(t, err, "ContextsList should not return error for in-cluster")

		expectedContexts := map[string]string{
			"cluster": "https://kubernetes.default.svc",
		}
		assert.Equal(t, expectedContexts, contexts, "in-cluster should return single cluster context")
		assert.Equal(t, "cluster", currentContext, "current context should be cluster")
	})

	t.Run("with nonexistent kubeconfig file returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		kubeconfigPath := path.Join(tempDir, "nonexistent-config")

		testStaticConfig := &config.StaticConfig{
			KubeConfig: kubeconfigPath,
		}

		// This should fail during Manager creation due to missing file
		_, err := NewManager(testStaticConfig)
		assert.Error(t, err, "NewManager should return error for nonexistent kubeconfig")
		assert.Contains(t, err.Error(), "no such file or directory", "error should mention missing file")
	})
}
