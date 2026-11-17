package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
)

// fakeClock is a manually advanced clock for clientLimiters tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

type ClientLimitersSuite struct {
	suite.Suite
	clock    *fakeClock
	limiters *clientLimiters
}

func (s *ClientLimitersSuite) SetupTest() {
	s.clock = &fakeClock{t: time.Unix(1_000_000, 0)}
	s.limiters = newClientLimiters(s.clock.now)
}

func (s *ClientLimitersSuite) TestBurstThenRefill() {
	for i := 0; i < 3; i++ {
		_, ok := s.limiters.allow("ip:10.0.0.1", 1, 3)
		s.Truef(ok, "request %d within burst must be allowed", i+1)
	}
	wait, ok := s.limiters.allow("ip:10.0.0.1", 1, 3)
	s.False(ok, "request over burst must be refused")
	s.Equal(time.Second, wait, "one token refills after 1s at 1 rps")

	s.clock.advance(time.Second)
	_, ok = s.limiters.allow("ip:10.0.0.1", 1, 3)
	s.True(ok, "a refilled token must be usable")
}

func (s *ClientLimitersSuite) TestRefusedRequestDoesNotUseAToken() {
	_, ok := s.limiters.allow("ip:10.0.0.1", 1, 1)
	s.Require().True(ok)
	for i := 0; i < 5; i++ {
		_, ok = s.limiters.allow("ip:10.0.0.1", 1, 1)
		s.Require().False(ok)
	}
	s.clock.advance(time.Second)
	_, ok = s.limiters.allow("ip:10.0.0.1", 1, 1)
	s.True(ok, "refused requests must not push the next token further out")
}

func (s *ClientLimitersSuite) TestClientsAreIndependent() {
	_, ok := s.limiters.allow("sub:alice", 1, 1)
	s.Require().True(ok)
	_, ok = s.limiters.allow("sub:alice", 1, 1)
	s.Require().False(ok)
	_, ok = s.limiters.allow("sub:bob", 1, 1)
	s.True(ok, "another client keeps its own bucket")
	_, ok = s.limiters.allow("ip:10.0.0.1", 1, 1)
	s.True(ok, "an IP key is separate from subject keys")
}

func (s *ClientLimitersSuite) TestNewSettingsResetBuckets() {
	_, ok := s.limiters.allow("ip:10.0.0.1", 1, 1)
	s.Require().True(ok)
	_, ok = s.limiters.allow("ip:10.0.0.1", 1, 1)
	s.Require().False(ok)
	_, ok = s.limiters.allow("ip:10.0.0.1", 1, 2)
	s.True(ok, "a changed burst starts a new bucket")
	_, ok = s.limiters.allow("ip:10.0.0.1", 1, 2)
	s.True(ok, "the new bucket has the new burst size")
}

func (s *ClientLimitersSuite) TestSweepRemovesIdleClients() {
	_, _ = s.limiters.allow("ip:10.0.0.1", 10, 10)
	_, _ = s.limiters.allow("ip:10.0.0.2", 10, 10)
	s.clock.advance(clientLimiterIdleTTL - time.Minute)
	_, _ = s.limiters.allow("ip:10.0.0.2", 10, 10)
	s.clock.advance(2 * time.Minute)
	_, _ = s.limiters.allow("ip:10.0.0.3", 10, 10)
	s.NotContains(s.limiters.entries, "ip:10.0.0.1", "idle past the TTL")
	s.Contains(s.limiters.entries, "ip:10.0.0.2", "seen within the TTL")
	s.Contains(s.limiters.entries, "ip:10.0.0.3")
}

func (s *ClientLimitersSuite) TestSweepKeepsBucketsThatHaveNotRefilled() {
	// burst 10 at 0.01 rps refills in 1000s, longer than clientLimiterIdleTTL.
	for i := 0; i < 10; i++ {
		_, _ = s.limiters.allow("ip:10.0.0.1", 0.01, 10)
	}
	s.clock.advance(clientLimiterIdleTTL + time.Minute)
	_, _ = s.limiters.allow("ip:10.0.0.2", 0.01, 10)
	s.Contains(s.limiters.entries, "ip:10.0.0.1", "dropping it would hand the client a full bucket early")
	s.clock.advance(1000 * time.Second)
	_, _ = s.limiters.allow("ip:10.0.0.2", 0.01, 10)
	s.NotContains(s.limiters.entries, "ip:10.0.0.1")
}

func TestClientLimiters(t *testing.T) {
	suite.Run(t, new(ClientLimitersSuite))
}

func cfgStateWithClientRateLimit(rps float64, burst int, trustProxy bool) *config.ConfigState {
	return config.NewConfigState(func() *config.Config {
		c := config.New()
		c.HTTP.ClientRateLimitRPS.SetForTest(rps)
		c.HTTP.ClientRateLimitBurst.SetForTest(burst)
		c.TrustProxyHeaders.SetForTest(trustProxy)
		return c
	}())
}

type ClientRateLimitMiddlewareSuite struct {
	suite.Suite
}

func (s *ClientRateLimitMiddlewareSuite) serve(h http.Handler, method, path string, mutate func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.RemoteAddr = "192.0.2.10:40000"
	if mutate != nil {
		req = mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func (s *ClientRateLimitMiddlewareSuite) TestLimitsMCPPosts() {
	h := ClientRateLimitMiddleware(cfgStateWithClientRateLimit(0.5, 2, false))(okHandler())
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)
	rec := s.serve(h, http.MethodPost, mcpEndpoint, nil)
	s.Equal(http.StatusTooManyRequests, rec.Code)
	s.Equal("2", rec.Header().Get("Retry-After"), "one token at 0.5 rps takes 2s")
}

func (s *ClientRateLimitMiddlewareSuite) TestDisabledByDefault() {
	h := ClientRateLimitMiddleware(config.NewConfigState(config.New()))(okHandler())
	for i := 0; i < 50; i++ {
		s.Require().Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)
	}
}

func (s *ClientRateLimitMiddlewareSuite) TestDefaultBurst() {
	h := ClientRateLimitMiddleware(cfgStateWithClientRateLimit(0.001, 0, false))(okHandler())
	for i := 0; i < config.DefaultRateLimitBurst; i++ {
		s.Require().Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)
	}
	s.Equal(http.StatusTooManyRequests, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)
}

func (s *ClientRateLimitMiddlewareSuite) TestOnlyMCPPostsCount() {
	h := ClientRateLimitMiddleware(cfgStateWithClientRateLimit(0.001, 1, false))(okHandler())
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, mcpEndpoint},
		{http.MethodDelete, mcpEndpoint},
		{http.MethodGet, healthEndpoint},
		{http.MethodGet, metricsEndpoint},
		{http.MethodGet, "/.well-known/oauth-protected-resource"},
		{http.MethodPost, "/other"},
	} {
		for i := 0; i < 3; i++ {
			s.Equalf(http.StatusOK, s.serve(h, tc.method, tc.path, nil).Code, "%s %s", tc.method, tc.path)
		}
	}
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code, "the MCP bucket is still full")
}

func (s *ClientRateLimitMiddlewareSuite) TestKeysOnVerifiedSubject() {
	h := ClientRateLimitMiddleware(cfgStateWithClientRateLimit(0.001, 1, false))(okHandler())
	as := func(subject, remote string) func(*http.Request) *http.Request {
		return func(r *http.Request) *http.Request {
			r.RemoteAddr = remote
			return r.WithContext(withVerifiedSubject(r.Context(), subject))
		}
	}
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, as("alice", "192.0.2.10:1")).Code)
	s.Equal(http.StatusTooManyRequests, s.serve(h, http.MethodPost, mcpEndpoint, as("alice", "198.51.100.7:2")).Code,
		"the same subject is one client from any address")
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, as("bob", "192.0.2.10:3")).Code,
		"another subject behind the same address is another client")
}

func (s *ClientRateLimitMiddlewareSuite) TestKeysOnClientIP() {
	s.Run("without trust_proxy_headers, X-Forwarded-For cannot pick a new bucket", func() {
		h := ClientRateLimitMiddleware(cfgStateWithClientRateLimit(0.001, 1, false))(okHandler())
		s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)
		rec := s.serve(h, http.MethodPost, mcpEndpoint, func(r *http.Request) *http.Request {
			r.Header.Set("X-Forwarded-For", "203.0.113.99")
			return r
		})
		s.Equal(http.StatusTooManyRequests, rec.Code)
	})
	s.Run("with trust_proxy_headers, clients behind one proxy are told apart", func() {
		h := ClientRateLimitMiddleware(cfgStateWithClientRateLimit(0.001, 1, true))(okHandler())
		from := func(ip string) func(*http.Request) *http.Request {
			return func(r *http.Request) *http.Request {
				r.Header.Set("X-Forwarded-For", ip+", 10.0.0.1")
				return r
			}
		}
		s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, from("203.0.113.1")).Code)
		s.Equal(http.StatusTooManyRequests, s.serve(h, http.MethodPost, mcpEndpoint, from("203.0.113.1")).Code)
		s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, from("203.0.113.2")).Code)
	})
}

func (s *ClientRateLimitMiddlewareSuite) TestReload() {
	cfgState := cfgStateWithClientRateLimit(0, 0, false)
	h := ClientRateLimitMiddleware(cfgState)(okHandler())
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)

	cfgState.Store(cfgStateWithClientRateLimit(0.001, 1, false).Load())
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code)
	s.Equal(http.StatusTooManyRequests, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code, "enabled by reload")

	cfgState.Store(cfgStateWithClientRateLimit(0, 0, false).Load())
	s.Equal(http.StatusOK, s.serve(h, http.MethodPost, mcpEndpoint, nil).Code, "disabled by reload")
}

func TestClientRateLimitMiddleware(t *testing.T) {
	suite.Run(t, new(ClientRateLimitMiddlewareSuite))
}

func TestRetryAfterSeconds(t *testing.T) {
	for _, tc := range []struct {
		wait time.Duration
		want int
	}{
		{0, 1}, {time.Millisecond, 1}, {time.Second, 1}, {1001 * time.Millisecond, 2}, {90 * time.Second, 90},
	} {
		if got := retryAfterSeconds(tc.wait); got != tc.want {
			t.Errorf("retryAfterSeconds(%v) = %d, want %d", tc.wait, got, tc.want)
		}
	}
}

// TestAuthorizationMiddlewareStoresVerifiedSubject checks the subject only
// reaches the rate limiter after the token was verified.
func TestAuthorizationMiddlewareStoresVerifiedSubject(t *testing.T) {
	oidcTestServer := NewOidcTestServer(t)
	t.Cleanup(oidcTestServer.Close)
	token := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256, `{
		"iss": "`+oidcTestServer.URL+`",
		"sub": "alice",
		"exp": `+strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)+`,
		"aud": "mcp-server"
	}`)
	newCfg := func(skipJWT bool) *config.ConfigState {
		c := config.New()
		c.RequireOAuth.SetForTest(true)
		c.OAuthAudience.SetForTest("mcp-server")
		c.SkipJWTVerification.SetForTest(skipJWT)
		return config.NewConfigState(c)
	}
	run := func(cfgState *config.ConfigState, snapshot *oauth.Snapshot) (int, string) {
		var seen string
		h := AuthorizationMiddleware(cfgState, oauth.NewState(snapshot))(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = verifiedSubject(r.Context())
		}))
		req := httptest.NewRequest(http.MethodPost, mcpEndpoint, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, seen
	}

	t.Run("verified token: subject is stored", func(t *testing.T) {
		code, seen := run(newCfg(false), &oauth.Snapshot{OIDCProvider: oidcTestServer.Provider})
		if code != http.StatusOK || seen != "alice" {
			t.Fatalf("got code %d subject %q, want 200 \"alice\"", code, seen)
		}
	})
	t.Run("passthrough token (skip_jwt_verification): no subject, so the client IP is used", func(t *testing.T) {
		code, seen := run(newCfg(true), &oauth.Snapshot{})
		if code != http.StatusOK || seen != "" {
			t.Fatalf("got code %d subject %q, want 200 and no subject", code, seen)
		}
	})
}

// TestClientRateLimitStateless runs the real server in stateless mode, where
// every request has an empty MCP session ID and the per-session limiter
// (http.rate_limit_rps) does not apply, and checks the per-client limit does.
func TestClientRateLimitStateless(t *testing.T) {
	c := config.New()
	c.Stateless.SetForTest(true)
	c.HTTP.RateLimitRPS.SetForTest(0.001)
	c.HTTP.RateLimitBurst.SetForTest(1)
	c.HTTP.ClientRateLimitRPS.SetForTest(0.001)
	c.HTTP.ClientRateLimitBurst.SetForTest(2)
	testCaseWithContext(t, &httpContext{Config: c}, func(ctx *httpContext) {
		post := func(id int) *http.Response {
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/mcp", ctx.HttpAddress),
				strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ping"}`, id)))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			_ = resp.Body.Close()
			return resp
		}
		for i := 1; i <= 2; i++ {
			if code := post(i).StatusCode; code == http.StatusTooManyRequests {
				t.Fatalf("request %d within the burst got 429", i)
			}
		}
		resp := post(3)
		if resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("third request: got %d, want 429", resp.StatusCode)
		}
		if resp.Header.Get("Retry-After") == "" {
			t.Errorf("429 without Retry-After")
		}
	})
}
