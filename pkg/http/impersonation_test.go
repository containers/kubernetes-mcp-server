package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type ImpersonationMiddlewareSuite struct{ suite.Suite }

func TestImpersonationMiddleware(t *testing.T) {
	suite.Run(t, new(ImpersonationMiddlewareSuite))
}

func (s *ImpersonationMiddlewareSuite) TestProxyBoundary() {
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"trusted proxy", func(*http.Request) {}, http.StatusNoContent},
		{"trusted IPv6 proxy", func(r *http.Request) { r.RemoteAddr = "[::1]:1234" }, http.StatusNoContent},
		{"IPv6 outside exact proxy CIDR", func(r *http.Request) { r.RemoteAddr = "[::2]:1234" }, http.StatusUnauthorized},
		{"IPv4-mapped proxy", func(r *http.Request) { r.RemoteAddr = "[::ffff:127.0.0.1]:1234" }, http.StatusNoContent},
		{"untrusted peer", func(r *http.Request) { r.RemoteAddr = "192.0.2.1:1234" }, http.StatusUnauthorized},
		{"forwarded address cannot grant trust", func(r *http.Request) {
			r.RemoteAddr = "192.0.2.1:1234"
			r.Header.Set("X-Forwarded-For", "127.0.0.1")
			r.Header.Set("X-Real-IP", "127.0.0.1")
		}, http.StatusUnauthorized},
		{"invalid peer", func(r *http.Request) { r.RemoteAddr = "not-an-address" }, http.StatusUnauthorized},
		{"missing bearer", func(r *http.Request) { r.Header.Del("Authorization") }, http.StatusUnauthorized},
		{"custom bearer is insufficient", func(r *http.Request) {
			r.Header.Del("Authorization")
			r.Header.Set("Kubernetes-Authorization", "Bearer frontend")
		}, http.StatusUnauthorized},
		{"missing user", func(r *http.Request) { r.Header.Del("Impersonate-User") }, http.StatusUnauthorized},
		{"blank user", func(r *http.Request) { r.Header.Set("Impersonate-User", " ") }, http.StatusUnauthorized},
		{"multiple users", func(r *http.Request) { r.Header.Add("Impersonate-User", "bob") }, http.StatusUnauthorized},
		{"blank group", func(r *http.Request) { r.Header.Add("Impersonate-Group", " ") }, http.StatusUnauthorized},
		{"unsupported uid", func(r *http.Request) { r.Header.Set("Impersonate-Uid", "123") }, http.StatusUnauthorized},
		{"unsupported extra", func(r *http.Request) { r.Header.Set("Impersonate-Extra-Scope", "admin") }, http.StatusUnauthorized},
	} {
		s.Run(tc.name, func() {
			cfg := config.BaseDefault()
			cfg.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
			cfg.ImpersonationTrustedProxies.SetForTest([]string{"127.0.0.1/32", "::1/128"})
			called := false
			handler := ImpersonationMiddleware(config.NewConfigState(cfg))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				ti := auth.TokenInfoFromContext(r.Context())
				s.Require().NotNil(ti)
				s.Equal("alice", ti.UserID)
				s.Equal(kubernetes.ImpersonationIdentity{UserName: "alice", Groups: []string{"developers", "readers"}}, ti.Extra[kubernetes.ImpersonationIdentityTokenInfoKey])
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			req.RemoteAddr = "127.0.0.1:1234"
			req.Header.Set("Authorization", "Bearer frontend")
			req.Header.Set("Impersonate-User", "alice")
			req.Header["Impersonate-Group"] = []string{"developers", "readers"}
			tc.mutate(req)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			s.Equal(tc.status, rr.Code)
			s.Equal(tc.status == http.StatusNoContent, called)
		})
	}
}

func (s *ImpersonationMiddlewareSuite) TestDisabledAndHealth() {
	for _, tc := range []struct{ name, mode, path string }{
		{"existing mode", config.ClusterAuthPassthrough, "/mcp"},
		{"health", config.ClusterAuthImpersonation, "/healthz"},
	} {
		s.Run(tc.name, func() {
			cfg := config.BaseDefault()
			cfg.ClusterAuthMode.SetForTest(tc.mode)
			handler := ImpersonationMiddleware(config.NewConfigState(cfg))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
			s.Equal(http.StatusNoContent, rr.Code)
		})
	}
}

func (s *ImpersonationMiddlewareSuite) TestIPv4MappedProxyCIDR() {
	for _, tc := range []struct {
		cidr   string
		peer   string
		status int
	}{
		{"::ffff:127.0.0.1/128", "[::ffff:127.0.0.1]:1234", http.StatusNoContent},
		{"::ffff:127.0.0.1/128", "127.0.0.1:1234", http.StatusNoContent},
		{"::ffff:127.0.0.1/128", "[::ffff:127.0.0.2]:1234", http.StatusUnauthorized},
		{"::ffff:127.0.0.1/128", "127.0.0.2:1234", http.StatusUnauthorized},
		{"::ffff:127.0.0.0/120", "127.0.0.255:1234", http.StatusNoContent},
		{"::ffff:127.0.0.0/120", "127.0.1.1:1234", http.StatusUnauthorized},
		{"::ffff:127.0.0.0/120", "[::1]:1234", http.StatusUnauthorized},
	} {
		s.Run(tc.cidr+"/"+tc.peer, func() {
			cfg := config.BaseDefault()
			cfg.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
			cfg.ImpersonationTrustedProxies.SetForTest([]string{tc.cidr})
			handler := ImpersonationMiddleware(config.NewConfigState(cfg))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			req.RemoteAddr = tc.peer
			req.Header.Set("Authorization", "Bearer frontend")
			req.Header.Set("Impersonate-User", "alice")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			s.Equal(tc.status, rr.Code)
		})
	}
}
