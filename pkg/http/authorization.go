package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
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
//	    2.1. Raw Token Validation (oidcProvider is nil):
//	         - The token is validated offline for basic sanity checks (expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//
//	         see TestAuthorizationRawToken
//
//	    2.2. OIDC Provider Validation (oidcProvider is not nil):
//	         - The token is validated offline for basic sanity checks (audience and expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//	         - The token is then validated against the OIDC Provider.
//
//	         see TestAuthorizationOidcToken
func AuthorizationMiddleware(staticConfig *config.StaticConfig, oidcProvider *oidc.Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == healthEndpoint || slices.Contains(WellKnownEndpoints, r.URL.EscapedPath()) {
				next.ServeHTTP(w, r)
				return
			}
			if !staticConfig.RequireOAuth {
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
				err = claims.ValidateWithProvider(r.Context(), staticConfig.OAuthAudience, oidcProvider)
			}
			if err != nil {
				klog.V(1).Infof("Authentication failed - JWT validation error: %s %s from %s, error: %v", r.Method, r.URL.Path, r.RemoteAddr, err)
				write401(w, wwwAuthenticateHeader, "invalid_token", "Unauthorized: Invalid token")
				return
			}

			next.ServeHTTP(w, r)
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
