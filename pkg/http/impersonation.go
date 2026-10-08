package http

import (
	"context"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// ImpersonationMiddleware accepts identities only from explicitly trusted peers.
// The proxy must authenticate every request and replace client-supplied headers.
func ImpersonationMiddleware(cfgState *config.ConfigState) func(http.Handler) http.Handler {
	trustedProxies := slices.Clone(cfgState.Load().ImpersonationTrustedProxies.Get())
	return func(next http.Handler) http.Handler {
		verified := auth.RequireBearerToken(func(_ context.Context, _ string, r *http.Request) (*auth.TokenInfo, error) {
			cfg := cfgState.Load()
			if cfg.ClusterAuthMode.Get() != config.ClusterAuthImpersonation || !isTrustedImpersonationProxy(r.RemoteAddr, trustedProxies) {
				return nil, auth.ErrInvalidToken
			}
			if len(r.Header.Values("Authorization")) != 1 || len(r.Header.Values("Impersonate-User")) != 1 {
				return nil, auth.ErrInvalidToken
			}
			for header := range r.Header {
				lower := strings.ToLower(header)
				if lower == "impersonate-uid" || strings.HasPrefix(lower, "impersonate-extra-") {
					return nil, auth.ErrInvalidToken
				}
			}
			identity := kubernetes.ImpersonationIdentity{
				UserName: r.Header.Get("Impersonate-User"),
				Groups:   slices.Clone(r.Header.Values("Impersonate-Group")),
			}
			if err := identity.Validate(); err != nil {
				return nil, auth.ErrInvalidToken
			}
			return &auth.TokenInfo{
				UserID: identity.UserName,
				Extra:  map[string]any{kubernetes.ImpersonationIdentityTokenInfoKey: identity},
			}, nil
		}, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := cfgState.Load()
			if cfg.ClusterAuthMode.Get() != config.ClusterAuthImpersonation ||
				slices.Contains(infraPaths(cfg.MetricsPort.Get() != ""), r.URL.Path) || isWellKnownPath(r.URL.EscapedPath()) {
				next.ServeHTTP(w, r)
				return
			}
			// SDK TokenInfo binds stateful sessions to the current proxy-authenticated user.
			verified.ServeHTTP(w, r)
		})
	}
}

func isTrustedImpersonationProxy(remoteAddr string, cidrs []string) bool {
	peer, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return false
	}
	addr := peer.Addr().Unmap()
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		// Mapped IPv4 prefixes reserve 96 bits before the IPv4 network bits.
		if prefix.Addr().Is4In6() && prefix.Bits() >= 96 {
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
