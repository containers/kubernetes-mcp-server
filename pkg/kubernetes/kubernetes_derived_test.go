package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/clientcmd"
)

type DerivedTestSuite struct {
	suite.Suite
}

func (s *DerivedTestSuite) TestKubeConfig() {
	// Create a temporary kubeconfig file for testing
	tempDir := s.T().TempDir()
	kubeconfigPath := filepath.Join(tempDir, "config")
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
	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644)
	s.Require().NoError(err, "failed to create kubeconfig file")

	s.Run("with no RequireOAuth (default) config", func() {
		testStaticConfig := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + strings.ReplaceAll(kubeconfigPath, `\`, `\\`) + `"
		`)))
		s.Run("without authorization header returns original clientset", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			derived, err := testManager.Derived(s.T().Context())
			s.Require().NoErrorf(err, "failed to create derived kubernetes: %v", err)

			s.Equal(derived, testManager.kubernetes, "expected original client, got different client")
		})

		s.Run("with invalid authorization header returns original client", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "invalid-token")
			derived, err := testManager.Derived(ctx)
			s.Require().NoErrorf(err, "failed to create derived kubernetes: %v", err)

			s.Equal(derived, testManager.kubernetes, "expected original client, got different client")
		})

		s.Run("with valid bearer token creates derived kubernetes with correct configuration", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "Bearer aiTana-julIA")
			derived, err := testManager.Derived(ctx)
			s.Require().NoErrorf(err, "failed to create derived kubernetes: %v", err)

			s.NotEqual(derived, testManager.kubernetes, "expected new derived client, got original client")
			s.Equal(derived.config, testStaticConfig, "config not properly wired to derived client")

			s.Run("RestConfig is correctly copied and sensitive fields are omitted", func() {
				derivedCfg := derived.RESTConfig()
				s.Require().NotNil(derivedCfg, "derived config is nil")

				originalCfg := testManager.kubernetes.RESTConfig()
				s.Equalf(originalCfg.Host, derivedCfg.Host, "expected Host %s, got %s", originalCfg.Host, derivedCfg.Host)
				s.Equalf(originalCfg.APIPath, derivedCfg.APIPath, "expected APIPath %s, got %s", originalCfg.APIPath, derivedCfg.APIPath)
				s.Equalf(originalCfg.QPS, derivedCfg.QPS, "expected QPS %f, got %f", originalCfg.QPS, derivedCfg.QPS)
				s.Equalf(originalCfg.Burst, derivedCfg.Burst, "expected Burst %d, got %d", originalCfg.Burst, derivedCfg.Burst)
				s.Equalf(originalCfg.Timeout, derivedCfg.Timeout, "expected Timeout %v, got %v", originalCfg.Timeout, derivedCfg.Timeout)

				s.Equalf(originalCfg.Insecure, derivedCfg.Insecure, "expected TLS Insecure %v, got %v", originalCfg.Insecure, derivedCfg.Insecure)
				s.Equalf(originalCfg.ServerName, derivedCfg.ServerName, "expected TLS ServerName %s, got %s", originalCfg.ServerName, derivedCfg.ServerName)
				s.Equalf(originalCfg.CAFile, derivedCfg.CAFile, "expected TLS CAFile %s, got %s", originalCfg.CAFile, derivedCfg.CAFile)
				s.Equalf(string(originalCfg.CAData), string(derivedCfg.CAData), "expected TLS CAData %s, got %s", string(originalCfg.CAData), string(derivedCfg.CAData))

				s.Equalf("aiTana-julIA", derivedCfg.BearerToken, "expected BearerToken %s, got %s", "aiTana-julIA", derivedCfg.BearerToken)
				s.Equalf("kubernetes-mcp-server/bearer-token-auth", derivedCfg.UserAgent, "expected UserAgent \"kubernetes-mcp-server/bearer-token-auth\", got %s", derivedCfg.UserAgent)

				// Verify that sensitive fields are NOT copied to prevent credential leakage
				// The derived config should only use the bearer token from the Authorization header
				// and not inherit any authentication credentials from the original kubeconfig
				s.Emptyf(derivedCfg.CertFile, "expected TLS CertFile to be empty, got %s", derivedCfg.CertFile)
				s.Emptyf(derivedCfg.KeyFile, "expected TLS KeyFile to be empty, got %s", derivedCfg.KeyFile)
				s.Emptyf(len(derivedCfg.CertData), "expected TLS CertData to be empty, got %v", derivedCfg.CertData)
				s.Emptyf(len(derivedCfg.KeyData), "expected TLS KeyData to be empty, got %v", derivedCfg.KeyData)

				s.Emptyf(derivedCfg.Username, "expected Username to be empty, got %s", derivedCfg.Username)
				s.Emptyf(derivedCfg.Password, "expected Password to be empty, got %s", derivedCfg.Password)
				s.Nilf(derivedCfg.AuthProvider, "expected AuthProvider to be nil, got %v", derivedCfg.AuthProvider)
				s.Nilf(derivedCfg.ExecProvider, "expected ExecProvider to be nil, got %v", derivedCfg.ExecProvider)
				s.Emptyf(derivedCfg.BearerTokenFile, "expected BearerTokenFile to be empty, got %s", derivedCfg.BearerTokenFile)
				s.Emptyf(derivedCfg.Impersonate.UserName, "expected Impersonate.UserName to be empty, got %s", derivedCfg.Impersonate.UserName)

				// Verify that the original manager still has the sensitive data
				s.Falsef(originalCfg.Username == "" && originalCfg.Password == "", "original kubeconfig shouldn't be modified")

			})
			s.Run("derived kubernetes has ClientConfig properly wired", func() {
				// Verify that the derived kubernetes has proper ClientConfig initialized
				s.NotNilf(derived.ToRawKubeConfigLoader(), "expected ToRawKubeConfigLoader to be initialized")
				derivedClientCmdApiConfig, err := derived.ToRawKubeConfigLoader().RawConfig()
				s.Require().NoErrorf(err, "failed to get derived clientCmdApiConfig: %v", err)
				s.Equalf("test-context", derivedClientCmdApiConfig.CurrentContext, "expected CurrentContext %s, got %s", "test-context", derivedClientCmdApiConfig.CurrentContext)
				s.Equalf(1, len(derivedClientCmdApiConfig.Clusters), "expected 1 cluster, got %d", len(derivedClientCmdApiConfig.Clusters))
				s.Equalf(1, len(derivedClientCmdApiConfig.Contexts), "expected 1 context, got %d", len(derivedClientCmdApiConfig.Contexts))
				s.Emptyf(derivedClientCmdApiConfig.AuthInfos, "expected 0 authInfos, got %d", len(derivedClientCmdApiConfig.AuthInfos))
			})
			s.Run("derived kubernetes has initialized clients", func() {
				// Verify that the derived kubernetes has proper clients initialized
				s.Equalf(testStaticConfig, derived.config, "config not properly wired to derived client")
				s.NotNilf(derived.RESTConfig(), "expected restConfig to be initialized")
				s.NotNilf(derived.RESTMapper(), "expected RESTMapper to be initialized")
				s.NotNilf(derived.DiscoveryClient(), "expected discoveryClient to be initialized")
				s.NotNilf(derived.DynamicClient(), "expected dynamicClient to be initialized")
				s.NotNilf(derived.MetricsV1beta1Client(), "expected metricsV1beta1Client to be initialized")
			})
		})
	})

	s.Run("with no RequireOAuth (default) and RawConfig error", func() {
		testStaticConfig := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + strings.ReplaceAll(kubeconfigPath, `\`, `\\`) + `"
		`)))

		s.Run("with bearer token but RawConfig fails returns original client", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			// Corrupt the clientCmdConfig by setting it to a config that will fail on RawConfig()
			// We'll do this by creating a config with an invalid file path
			badKubeconfigPath := filepath.Join(s.T().TempDir(), "nonexistent", "config")
			badConfig := test.Must(config.ReadToml([]byte(`
				kubeconfig = "` + strings.ReplaceAll(badKubeconfigPath, `\`, `\\`) + `"
			`)))
			badManager, _ := NewManager(badConfig, testManager.kubernetes.RESTConfig(), testManager.kubernetes.ToRawKubeConfigLoader())
			// Replace the clientCmdConfig with one that will fail
			pathOptions := clientcmd.NewDefaultPathOptions()
			pathOptions.LoadingRules.ExplicitPath = badKubeconfigPath
			badClientCmdConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				pathOptions.LoadingRules,
				&clientcmd.ConfigOverrides{})
			badManager.kubernetes.clientCmdConfig = badClientCmdConfig

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "Bearer aiTana-julIA")
			derived, err := badManager.Derived(ctx)
			s.Require().NoErrorf(err, "expected no error when RequireOAuth=false, got: %v", err)
			s.Equal(derived, badManager.kubernetes, "expected original client when RawConfig fails and RequireOAuth=false")
		})
	})

	s.Run("with RequireOAuth=true and RawConfig error", func() {
		badKubeconfigPath := filepath.Join(s.T().TempDir(), "nonexistent", "config")
		testStaticConfig := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + strings.ReplaceAll(badKubeconfigPath, `\`, `\\`) + `"
			require_oauth = true
		`)))

		s.Run("with bearer token but RawConfig fails returns error", func() {
			// First create a working manager
			workingKubeconfigPath := filepath.Join(s.T().TempDir(), "working-config")
			err := os.WriteFile(workingKubeconfigPath, []byte(kubeconfigContent), 0644)
			s.Require().NoError(err)
			workingConfig := test.Must(config.ReadToml([]byte(`
				kubeconfig = "` + strings.ReplaceAll(workingKubeconfigPath, `\`, `\\`) + `"
			`)))
			testManager, err := NewKubeconfigManager(workingConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			// Now create a bad manager with RequireOAuth=true
			badManager, _ := NewManager(testStaticConfig, testManager.kubernetes.RESTConfig(), testManager.kubernetes.ToRawKubeConfigLoader())
			// Replace the clientCmdConfig with one that will fail
			pathOptions := clientcmd.NewDefaultPathOptions()
			pathOptions.LoadingRules.ExplicitPath = badKubeconfigPath
			badClientCmdConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				pathOptions.LoadingRules,
				&clientcmd.ConfigOverrides{})
			badManager.kubernetes.clientCmdConfig = badClientCmdConfig

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "Bearer aiTana-julIA")
			derived, err := badManager.Derived(ctx)
			s.Require().Error(err, "expected error when RawConfig fails and RequireOAuth=true")
			s.ErrorContains(err, "failed to get kubeconfig", "expected error containing 'failed to get kubeconfig'")
			s.Nil(derived, "expected nil derived kubernetes when RawConfig fails and RequireOAuth=true")
		})
	})

	s.Run("with no RequireOAuth (default) and NewKubernetes error", func() {
		testStaticConfig := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + strings.ReplaceAll(kubeconfigPath, `\`, `\\`) + `"
		`)))

		s.Run("with bearer token but invalid rest config returns original client", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			// Corrupt the rest config to make NewKubernetes fail
			// Setting an invalid Host URL should cause client creation to fail
			testManager.kubernetes.restConfig.Host = "://invalid-url"

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "Bearer aiTana-julIA")
			derived, err := testManager.Derived(ctx)
			s.Require().NoErrorf(err, "expected no error when RequireOAuth=false, got: %v", err)
			s.Equal(derived, testManager.kubernetes, "expected original client when NewKubernetes fails and RequireOAuth=false")
		})
	})

	s.Run("with RequireOAuth=true and NewKubernetes error", func() {
		testStaticConfig := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + strings.ReplaceAll(kubeconfigPath, `\`, `\\`) + `"
			require_oauth = true
		`)))

		s.Run("with bearer token but invalid rest config returns error", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			// Corrupt the rest config to make NewKubernetes fail
			testManager.kubernetes.restConfig.Host = "://invalid-url"

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "Bearer aiTana-julIA")
			derived, err := testManager.Derived(ctx)
			s.Require().Error(err, "expected error when NewKubernetes fails and RequireOAuth=true")
			s.ErrorContains(err, "failed to create derived client", "expected error containing 'failed to create derived client'")
			s.Nil(derived, "expected nil derived kubernetes when NewKubernetes fails and RequireOAuth=true")
		})
	})

	s.Run("with RequireOAuth=true", func() {
		testStaticConfig := test.Must(config.ReadToml([]byte(`
			kubeconfig = "` + strings.ReplaceAll(kubeconfigPath, `\`, `\\`) + `"
			require_oauth = true
		`)))

		s.Run("with no authorization header returns oauth token required error", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			derived, err := testManager.Derived(s.T().Context())
			s.Require().Error(err, "expected error for missing oauth token, got nil")
			s.EqualError(err, "oauth token required", "expected error 'oauth token required', got %s", err.Error())
			s.Nil(derived, "expected nil derived kubernetes when oauth token required")
		})

		s.Run("with invalid authorization header returns oauth token required error", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "invalid-token")
			derived, err := testManager.Derived(ctx)
			s.Require().Error(err, "expected error for invalid oauth token, got nil")
			s.EqualError(err, "oauth token required", "expected error 'oauth token required', got %s", err.Error())
			s.Nil(derived, "expected nil derived kubernetes when oauth token required")
		})

		s.Run("with valid bearer token creates derived kubernetes", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "Bearer aiTana-julIA")
			derived, err := testManager.Derived(ctx)
			s.Require().NoErrorf(err, "failed to create derived kubernetes: %v", err)

			s.NotEqual(derived, testManager.kubernetes, "expected new derived client, got original client")
			s.Equal(derived.config, testStaticConfig, "config not properly wired to derived client")

			derivedCfg := derived.RESTConfig()
			s.Require().NotNil(derivedCfg, "derived config is nil")

			s.Equalf("aiTana-julIA", derivedCfg.BearerToken, "expected BearerToken %s, got %s", "aiTana-julIA", derivedCfg.BearerToken)
		})
	})
}

func TestDerived(t *testing.T) {
	suite.Run(t, new(DerivedTestSuite))
}
