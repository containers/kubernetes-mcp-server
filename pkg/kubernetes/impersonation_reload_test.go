package kubernetes_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
)

type ImpersonationReloadSuite struct {
	suite.Suite
}

func (s *ImpersonationReloadSuite) TestConfigurationReloadPreservesCallerIdentity() {
	for _, identity := range []string{"", "alice"} {
		s.Run("proxy identity="+identity, func() {
			server := test.NewMockServer()
			s.T().Cleanup(server.Close)
			server.Handle(test.NewDiscoveryClientHandler())
			raw := server.TLSKubeconfig(s.T())
			raw.AuthInfos["fake"].Token = "backend-token"
			previous := config.BaseDefault()
			previous.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), raw))
			previous.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
			state := config.NewConfigState(previous)
			provider, err := kubernetes.NewProvider(s.T().Context(), previous,
				kubernetes.WithTokenExchange(oauth.NewState(nil)),
				kubernetes.WithConfigProvider(state.Load))
			s.Require().NoError(err)
			s.T().Cleanup(provider.Close)
			next := config.BaseDefault()
			next.KubeConfig = previous.KubeConfig
			next.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
			next.ReadOnly.SetForTest(true)
			// SIGHUP publishes the HTTP/provider snapshot before existing managers.
			state.Store(next)
			ctx := context.WithValue(s.T().Context(), kubernetes.OAuthAuthorizationHeader, "Bearer frontend-token")
			if identity != "" {
				ctx = kubernetes.WithImpersonationIdentity(ctx, kubernetes.ImpersonationIdentity{UserName: identity})
			}
			for _, publish := range []bool{false, true} {
				if publish {
					provider.PublishKubernetesConfig(next)
				}
				client, err := provider.GetDerivedKubernetes(ctx, "")
				if identity == "" {
					s.Error(err, "missing identity must never yield a backend client during reload")
					s.Nil(client)
				} else {
					s.Require().NoError(err)
					s.Equal(identity, client.RESTConfig().Impersonate.UserName)
					s.Equal("backend-token", client.RESTConfig().BearerToken)
					s.Equal(publish, client.Config().ReadOnly.Get())
				}
			}
		})
	}
}

func TestImpersonationReload(t *testing.T) {
	suite.Run(t, new(ImpersonationReloadSuite))
}
