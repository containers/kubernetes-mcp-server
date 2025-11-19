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
		s.Run("without authorization header returns original manager", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)
			s.T().Cleanup(testManager.Close)

			derived, err := testManager.Derived(s.T().Context())
			s.Require().NoErrorf(err, "failed to create derived manager: %v", err)

			s.Equal(derived.manager, testManager, "expected original manager, got different manager")
		})

		s.Run("with invalid authorization header returns original manager", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)
			s.T().Cleanup(testManager.Close)

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "invalid-token")
			derived, err := testManager.Derived(ctx)
			s.Require().NoErrorf(err, "failed to create derived manager: %v", err)

			s.Equal(derived.manager, testManager, "expected original manager, got different manager")
		})

		s.Run("with valid bearer token creates derived manager with correct configuration", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)
			s.T().Cleanup(testManager.Close)

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "Bearer aiTana-julIA")
			derived, err := testManager.Derived(ctx)
			s.Require().NoErrorf(err, "failed to create derived manager: %v", err)

			s.NotEqual(derived.manager, testManager, "expected new derived manager, got original manager")
			s.Equal(derived.manager.staticConfig, testStaticConfig, "staticConfig not properly wired to derived manager")

			s.Run("RestConfig is correctly copied and sensitive fields are omitted", func() {
				derivedCfg := derived.manager.cfg
				s.Require().NotNil(derivedCfg, "derived config is nil")

				originalCfg := testManager.cfg
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
			s.Run("derived manager has initialized clients", func() {
				// Verify that the derived manager has proper clients initialized
				s.NotNilf(derived.manager.accessControlClientSet, "expected accessControlClientSet to be initialized")
				s.Equalf(testStaticConfig, derived.manager.accessControlClientSet.staticConfig, "staticConfig not properly wired to derived manager")
				s.NotNilf(derived.manager.discoveryClient, "expected discoveryClient to be initialized")
				s.NotNilf(derived.manager.restMapper, "expected accessControlRESTMapper to be initialized")
				//s.Equalf(testStaticConfig, derived.manager.re.staticConfig, "staticConfig not properly wired to derived manager")
				s.NotNilf(derived.manager.dynamicClient, "expected dynamicClient to be initialized")
			})
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
			s.T().Cleanup(testManager.Close)

			derived, err := testManager.Derived(s.T().Context())
			s.Require().Error(err, "expected error for missing oauth token, got nil")
			s.EqualError(err, "oauth token required", "expected error 'oauth token required', got %s", err.Error())
			s.Nil(derived, "expected nil derived manager when oauth token required")
		})

		s.Run("with invalid authorization header returns oauth token required error", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)
			s.T().Cleanup(testManager.Close)

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "invalid-token")
			derived, err := testManager.Derived(ctx)
			s.Require().Error(err, "expected error for invalid oauth token, got nil")
			s.EqualError(err, "oauth token required", "expected error 'oauth token required', got %s", err.Error())
			s.Nil(derived, "expected nil derived manager when oauth token required")
		})

		s.Run("with valid bearer token creates derived manager", func() {
			testManager, err := NewKubeconfigManager(testStaticConfig, "")
			s.Require().NoErrorf(err, "failed to create test manager: %v", err)
			s.T().Cleanup(testManager.Close)

			ctx := context.WithValue(s.T().Context(), HeaderKey("Authorization"), "Bearer aiTana-julIA")
			derived, err := testManager.Derived(ctx)
			s.Require().NoErrorf(err, "failed to create derived manager: %v", err)

			s.NotEqual(derived.manager, testManager, "expected new derived manager, got original manager")
			s.Equal(derived.manager.staticConfig, testStaticConfig, "staticConfig not properly wired to derived manager")

			derivedCfg := derived.manager.cfg
			s.Require().NotNil(derivedCfg, "derived config is nil")

			s.Equalf("aiTana-julIA", derivedCfg.BearerToken, "expected BearerToken %s, got %s", "aiTana-julIA", derivedCfg.BearerToken)
		})
	})
}

func TestDerived(t *testing.T) {
	suite.Run(t, new(DerivedTestSuite))
}
