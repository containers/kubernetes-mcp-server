package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
	authenticationapiv1 "k8s.io/api/authentication/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
)

type KubernetesApiTokenVerifier interface {
	// KubernetesApiVerifyToken TODO: clarify proper implementation
	KubernetesApiVerifyToken(ctx context.Context, token, audience string) (*authenticationapiv1.UserInfo, []string, error)
}

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
//	         - If ValidateToken is set, the token is then used against the Kubernetes API Server for TokenReview.
//
//	         see TestAuthorizationRawToken
//
//	    2.2. OIDC Provider Validation (oidcProvider is not nil):
//	         - The token is validated offline for basic sanity checks (audience and expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//	         - The token is then validated against the OIDC Provider.
//	         - If ValidateToken is set, the token is then used against the Kubernetes API Server for TokenReview.
//
//	         see TestAuthorizationOidcToken
//
//	    2.3. OIDC Token Exchange (oidcProvider is not nil, StsClientId and StsAudience are set):
//	         - The token is validated offline for basic sanity checks (audience and expiration).
//	         - If OAuthAudience is set, the token is validated against the audience.
//	         - The token is then validated against the OIDC Provider.
//	         - If the token is valid, an external account token exchange is performed using
//	           the OIDC Provider to obtain a new token with the specified audience and scopes.
//	         - If ValidateToken is set, the exchanged token is then used against the Kubernetes API Server for TokenReview.
//
//	         see TestAuthorizationOidcTokenExchange
func AuthorizationMiddleware(staticConfig *config.StaticConfig, oidcProvider *oidc.Provider, verifier KubernetesApiTokenVerifier) func(http.Handler) http.Handler {
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
			// Scopes propagation, they are likely to be used for authorization.
			if err == nil {
				scopes := claims.GetScopes()
				klog.V(2).Infof("JWT token validated - Scopes: %v", scopes)
				r = r.WithContext(context.WithValue(r.Context(), mcp.TokenScopesContextKey, scopes))
			}
			// Token exchange with OIDC provider
			sts := NewFromConfig(staticConfig, oidcProvider)
			// TODO: Maybe the token had already been exchanged, if it has the right audience and scopes, we can skip this step.
			if err == nil && sts.IsEnabled() {
				var exchangedToken *oauth2.Token
				// If the token is valid, we can exchange it for a new token with the specified audience and scopes.
				exchangedToken, err = sts.ExternalAccountTokenExchange(r.Context(), &oauth2.Token{
					AccessToken: claims.Token,
					TokenType:   "Bearer",
				})
				if err == nil {
					// Replace the original token with the exchanged token
					token = exchangedToken.AccessToken
					claims, err = ParseJWTClaims(token)
					r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token)) // TODO: Implement test to verify, THIS IS A CRITICAL PART
				}
			}
			// Kubernetes API Server TokenReview validation
			if err == nil && staticConfig.ValidateToken {
				err = claims.ValidateWithKubernetesApi(r.Context(), staticConfig.OAuthAudience, verifier)
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
		return fmt.Errorf("JWT token validation error: %v", err)
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
			return fmt.Errorf("OIDC token validation error: %v", err)
		}
	}
	return nil
}

func (c *JWTClaims) ValidateWithKubernetesApi(ctx context.Context, audience string, verifier KubernetesApiTokenVerifier) error {
	if verifier != nil {
		_, _, err := verifier.KubernetesApiVerifyToken(ctx, c.Token, audience)
		if err != nil {
			return fmt.Errorf("kubernetes API token validation error: %v", err)
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
