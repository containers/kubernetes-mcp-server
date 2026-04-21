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
	s.Run("defaults to passthrough", func() {
		cfg := config.Default()
		s.Equal(api.ClusterAuthPassthrough, cfg.ResolveClusterAuthMode())
	})

	s.Run("defaults to passthrough regardless of require_oauth", func() {
		cfg := config.Default()
		cfg.RequireOAuth = true
		s.Equal(api.ClusterAuthPassthrough, cfg.ResolveClusterAuthMode())
	})

	s.Run("returns explicit kubeconfig when set", func() {
		cfg := config.Default()
		cfg.ClusterAuthMode = api.ClusterAuthKubeconfig
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

	s.Run("auto-detect defaults to passthrough", func() {
		cfg := config.Default()
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
