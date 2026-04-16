package kubernetes

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type TokenExchangeRoutingSuite struct {
	suite.Suite
}

func (s *TokenExchangeRoutingSuite) TestResolveClusterAuthMode() {
	s.Run("returns passthrough when OAuth is required", func() {
		cfg := config.Default()
		cfg.RequireOAuth = true
		s.Equal(api.ClusterAuthPassthrough, cfg.ResolveClusterAuthMode())
	})

	s.Run("returns kubeconfig when OAuth is not required", func() {
		cfg := config.Default()
		cfg.RequireOAuth = false
		s.Equal(api.ClusterAuthKubeconfig, cfg.ResolveClusterAuthMode())
	})

	s.Run("returns explicit mode when set", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthKubeconfig
		cfg.RequireOAuth = true
		s.Equal(api.ClusterAuthKubeconfig, cfg.ResolveClusterAuthMode())
	})
}

func (s *TokenExchangeRoutingSuite) TestStsExchangeTokenInContextRouting() {
	s.Run("kubeconfig mode clears OAuth token", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthKubeconfig

		ctx := context.WithValue(context.Background(), OAuthAuthorizationHeader, "Bearer original-token")
		result, err := stsExchangeTokenInContext(ctx, cfg, nil, nil, "original-token", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("", auth)
	})

	s.Run("passthrough mode preserves token", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthPassthrough

		ctx := context.Background()
		result, err := stsExchangeTokenInContext(ctx, cfg, nil, nil, "original-token", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer original-token", auth)
	})

	s.Run("auto-detect defaults to kubeconfig when OAuth not required", func() {
		cfg := config.Default()
		cfg.RequireOAuth = false
		cfg.ClusterAuthMode = "" // auto-detect

		ctx := context.Background()
		result, err := stsExchangeTokenInContext(ctx, cfg, nil, nil, "original-token", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("", auth)
	})

	s.Run("auto-detect defaults to passthrough when OAuth required", func() {
		cfg := config.Default()
		cfg.RequireOAuth = true
		cfg.ClusterAuthMode = "" // auto-detect

		ctx := context.Background()
		result, err := stsExchangeTokenInContext(ctx, cfg, nil, nil, "original-token", nil)
		s.Require().NoError(err)

		auth, _ := result.Value(OAuthAuthorizationHeader).(string)
		s.Equal("Bearer original-token", auth)
	})
}

func TestTokenExchangeRouting(t *testing.T) {
	suite.Run(t, new(TokenExchangeRoutingSuite))
}
