package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
)

// AuthorizationMiddlewareReloadSuite verifies that config changes published via
// *config.ConfigState.Store take effect on the NEXT request served by an
// already-constructed AuthorizationMiddleware — i.e. the middleware re-reads
// the config snapshot per request instead of capturing it at wiring time.
// Regression guard for issue #1106 / PR #1105.
type AuthorizationMiddlewareReloadSuite struct {
	suite.Suite
}

func TestAuthorizationMiddlewareReload(t *testing.T) {
	suite.Run(t, new(AuthorizationMiddlewareReloadSuite))
}

func (s *AuthorizationMiddlewareReloadSuite) TestRequireOAuthFlipObservedPerRequest() {
	s.Run("unauthenticated request: 200 when require_oauth=false, 401 after cfgState.Store flips it to true", func() {
		cfgState := config.NewConfigState(func() *config.Config {
			c := config.New()
			c.RequireOAuth.SetForTest(false)
			return c
		}())
		oauthState := oauth.NewState(&oauth.Snapshot{})

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		handler := AuthorizationMiddleware(cfgState, oauthState)(next)

		// Pre-reload: require_oauth=false → unauthenticated request passes.
		req1 := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		rr1 := httptest.NewRecorder()
		handler.ServeHTTP(rr1, req1)
		s.Equal(http.StatusOK, rr1.Code, "pre-reload: require_oauth=false must allow unauthenticated requests")

		// Simulate a SIGHUP reload flipping require_oauth to true.
		cfgState.Store(func() *config.Config {
			c := config.New()
			c.RequireOAuth.

				// Post-reload: same unauthenticated request must now be rejected.
				SetForTest(true)
			return c
		}())

		req2 := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		rr2 := httptest.NewRecorder()
		handler.ServeHTTP(rr2, req2)
		s.Equal(http.StatusUnauthorized, rr2.Code,
			"post-reload: require_oauth=true must reject unauthenticated requests")
		s.Contains(rr2.Header().Get("WWW-Authenticate"), `realm="Kubernetes MCP Server"`,
			"401 response must carry WWW-Authenticate challenge")
	})
}
