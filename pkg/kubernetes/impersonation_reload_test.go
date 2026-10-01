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

func (s *ImpersonationReloadSuite) TestReloadNeverFallsBackToBackendIdentity() {
	for _, identity := range []string{"", "alice"} {
		s.Run("proxy identity="+identity, func() {
			server := test.NewMockServer()
			s.T().Cleanup(server.Close)
			server.Handle(test.NewDiscoveryClientHandler())
			previous := config.BaseDefault()
			previous.KubeConfig.SetForTest(server.KubeconfigFile(s.T()))
			previous.ClusterAuthMode.SetForTest(config.ClusterAuthPassthrough)
			state := config.NewConfigState(previous)
			provider, err := kubernetes.NewProvider(s.T().Context(), previous,
				kubernetes.WithTokenExchange(oauth.NewState(nil)),
				kubernetes.WithConfigProvider(state.Load))
			s.Require().NoError(err)
			s.T().Cleanup(provider.Close)
			next := config.BaseDefault()
			next.KubeConfig = previous.KubeConfig
			next.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
			// SIGHUP publishes the HTTP/provider snapshot before existing managers.
			state.Store(next)
			ctx := context.WithValue(s.T().Context(), kubernetes.OAuthAuthorizationHeader, "Bearer frontend-token")
			if identity != "" {
				ctx = kubernetes.WithImpersonationIdentity(ctx, kubernetes.ImpersonationIdentity{UserName: identity})
			}
			client, err := provider.GetDerivedKubernetes(ctx, "")
			s.Error(err, "mixed reload snapshots must reject the request instead of using backend identity")
			s.Nil(client)

			provider.PublishKubernetesConfig(next)
			client, err = provider.GetDerivedKubernetes(ctx, "")
			if identity == "" {
				s.Error(err, "missing identity remains invalid after managers catch up")
				s.Nil(client)
			} else {
				s.Require().NoError(err)
				s.Equal(identity, client.RESTConfig().Impersonate.UserName)
			}
		})
	}
}

func TestImpersonationReload(t *testing.T) {
	suite.Run(t, new(ImpersonationReloadSuite))
}
