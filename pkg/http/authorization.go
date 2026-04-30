package http

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
)

// write401 sends a 401/Unauthorized response with WWW-Authenticate header.
func write401(w http.ResponseWriter, wwwAuthenticateHeader, errorType, message string) {
	w.Header().Set("WWW-Authenticate", wwwAuthenticateHeader+fmt.Sprintf(`, error="%s"`, errorType))
	http.Error(w, message, http.StatusUnauthorized)
}

// AuthorizationMiddleware validates the OAuth flow for protected resources.
//
// The flow is skipped for unprotected resources, such as health checks and well-known endpoints.
//
//	There are several auth scenarios supported by this middleware:
//
//	 1. requireOAuth is false:
//
//	    - The OAuth flow is skipped, and the server is effectively unprotected.
//	    - The request is passed to the next handler without any validation.
//
//	    see TestAuthorizationRequireOAuthFalse
//
//	 2. requireOAuth is set to true, server is protected:
//
//	    2.1. Raw Token Validation (oidcProvider is nil, SkipJWTVerification is true):
//	         - Requires skip_jwt_verification=true; otherwise the request is rejected with 500.
//	         - The token is validated offline for basic sanity checks (expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//	         - No cryptographic signature verification is performed.
//
//	         see TestAuthorizationRawToken
//
//	    2.2. OIDC Provider Validation (oidcProvider is not nil):
//	         - The token is validated offline for basic sanity checks (audience and expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//	         - The token is then validated against the OIDC Provider.
//
//	         see TestAuthorizationOidcToken
func AuthorizationMiddleware(cfgState *config.StaticConfigState, oauthState *oauth.State) func(http.Handler) http.Handler {
	var skipJWTWarningOnce sync.Once
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for infrastructure endpoints (health, metrics) and well-known endpoints
			// Use prefix matching per endpoint to handle sub-paths like /.well-known/oauth-protected-resource/sse
			requestPath := r.URL.EscapedPath()
			isWellKnown := !strings.Contains(requestPath, "..") && slices.ContainsFunc(WellKnownEndpoints, func(ep string) bool {
				return requestPath == ep || strings.HasPrefix(requestPath, ep+"/")
			})
			if slices.Contains(infraPaths, r.URL.Path) || isWellKnown {
				next.ServeHTTP(w, r)
				return
			}
			// Load the latest config snapshot on every request so that
			// SIGHUP-reloaded auth settings take effect immediately.
			staticConfig := cfgState.Load()
			if !staticConfig.RequireOAuth {
				// Always extract the Authorization header so it can be forwarded
				// to the cluster, even without OAuth validation.
				if authHeader := r.Header.Get("Authorization"); authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
					ctx := context.WithValue(r.Context(), internalk8s.OAuthAuthorizationHeader, authHeader)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			wwwAuthenticateHeader := "Bearer realm=\"Kubernetes MCP Server\""
			if staticConfig.OAuthAudience != "" {
				wwwAuthenticateHeader += fmt.Sprintf(`, audience="%s"`, staticConfig.OAuthAudience)
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				klog.V(1).Infof("Authentication failed - missing or invalid bearer token: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
				write401(w, wwwAuthenticateHeader, "missing_token", "Unauthorized: Bearer token required")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Opaque token support (e.g., OpenShift OAuth tokens: "sha256~...")
			// When accept_opaque_tokens is enabled and the token is not a JWT,
			// pass it through without validation — the downstream Kubernetes API
			// server will validate it directly.
			if !isJWT(token) {
				if staticConfig.AcceptOpaqueTokens {
					klog.V(2).Infof("Accepting opaque (non-JWT) bearer token for passthrough: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
					ctx := context.WithValue(r.Context(), internalk8s.OAuthAuthorizationHeader, authHeader)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				klog.V(1).Infof("Authentication failed - opaque token rejected (accept_opaque_tokens not enabled): %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
				write401(w, wwwAuthenticateHeader, "invalid_token", "Unauthorized: Token format not supported")
				return
			}

			claims, err := ParseJWTClaims(token)
			if err == nil && claims == nil {
				// Impossible case, but just in case
				err = fmt.Errorf("failed to parse JWT claims from token")
			}
			// Offline validation
			if err == nil {
				err = claims.ValidateOffline(staticConfig.OAuthAudience)
			}
			// Online OIDC provider validation
			if err == nil {
				snapshot := oauthState.Load()
				if snapshot == nil || snapshot.OIDCProvider == nil {
					// Provider was configured (authorization_url set) but is unavailable — reject
					if staticConfig.AuthorizationURL != "" {
						klog.V(1).Infof("Authentication rejected - OIDC provider unavailable: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
						write401(w, wwwAuthenticateHeader, "temporarily_unavailable", "OIDC provider is not available")
						return
					}
					// No provider configured - require explicit opt-in via skip_jwt_verification
					if !staticConfig.SkipJWTVerification {
						// When accept_opaque_tokens is enabled, the server may legitimately
						// start without an OIDC provider. JWTs arriving in this config are
						// a client error (wrong token type), not a server misconfiguration.
						if staticConfig.AcceptOpaqueTokens {
							klog.V(1).Infof("Authentication rejected - JWT received but only opaque tokens are supported in this configuration: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
							write401(w, wwwAuthenticateHeader, "invalid_token", "Unauthorized: Invalid token")
							return
						}
						klog.V(1).Infof("Authentication rejected - JWT verification not configured: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
						http.Error(w, "JWT verification not configured - set authorization_url or skip_jwt_verification", http.StatusInternalServerError)
						return
					}
					skipJWTWarningOnce.Do(func() {
						klog.Warningf("JWT accepted without signature verification - set authorization_url for secure validation")
					})
				} else {
					err = claims.ValidateWithProvider(r.Context(), staticConfig.OAuthAudience, snapshot.OIDCProvider)
				}
			}
			if err != nil {
				klog.V(1).Infof("Authentication failed - JWT validation error: %s %s from %s, error: %v", r.Method, r.URL.Path, r.RemoteAddr, err)
				write401(w, wwwAuthenticateHeader, "invalid_token", "Unauthorized: Invalid token")
				return
			}

			// Store the validated Authorization header in context for MCP handlers
			// This is necessary because SSE transport doesn't propagate HTTP headers to MCP requests
			ctx := context.WithValue(r.Context(), internalk8s.OAuthAuthorizationHeader, authHeader)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

var allSignatureAlgorithms = []jose.SignatureAlgorithm{
	jose.EdDSA,
	jose.HS256,
	jose.HS384,
	jose.HS512,
	jose.RS256,
	jose.RS384,
	jose.RS512,
	jose.ES256,
	jose.ES384,
	jose.ES512,
	jose.PS256,
	jose.PS384,
	jose.PS512,
}

type JWTClaims struct {
	jwt.Claims
	Token string `json:"-"`
	Scope string `json:"scope,omitempty"`
}

func (c *JWTClaims) GetScopes() []string {
	if c.Scope == "" {
		return nil
	}
	return strings.Fields(c.Scope)
}

// ValidateOffline Checks if the JWT claims are valid and if the audience matches the expected one.
func (c *JWTClaims) ValidateOffline(audience string) error {
	expected := jwt.Expected{}
	if audience != "" {
		expected.AnyAudience = jwt.Audience{audience}
	}
	if err := c.Validate(expected); err != nil {
		return fmt.Errorf("JWT token validation error: %w", err)
	}
	return nil
}

// ValidateWithProvider validates the JWT claims against the OIDC provider.
func (c *JWTClaims) ValidateWithProvider(ctx context.Context, audience string, provider *oidc.Provider) error {
	if provider != nil {
		verifier := provider.Verifier(&oidc.Config{
			ClientID: audience,
		})
		_, err := verifier.Verify(ctx, c.Token)
		if err != nil {
			return fmt.Errorf("OIDC token validation error: %w", err)
		}
	}
	return nil
}

// isJWT checks if a token looks like a JWT (three dot-separated base64 parts).
func isJWT(token string) bool {
	return strings.Count(token, ".") == 2
}

func ParseJWTClaims(token string) (*JWTClaims, error) {
	tkn, err := jwt.ParseSigned(token, allSignatureAlgorithms)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT token: %w", err)
	}
	claims := &JWTClaims{}
	err = tkn.UnsafeClaimsWithoutVerification(claims)
	claims.Token = token
	return claims, err
}
