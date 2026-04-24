package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"k8s.io/klog/v2"
)

const maxWellKnownResponseSize = 1 << 20 // 1 MB
const oidcConfigCacheTTL = 5 * time.Minute
const oidcConfigFetchTimeout = 30 * time.Second

var allowedResponseHeaders = map[string]bool{
	"Cache-Control": true,
	"Date":          true,
	"Etag":          true,
	"Expires":       true,
	"Last-Modified": true,
	"Pragma":        true,
}

const (
	oauthAuthorizationServerEndpoint = "/.well-known/oauth-authorization-server"
	oauthProtectedResourceEndpoint   = "/.well-known/oauth-protected-resource"
	openIDConfigurationEndpoint      = "/.well-known/openid-configuration"
)

var WellKnownEndpoints = []string{
	oauthAuthorizationServerEndpoint,
	oauthProtectedResourceEndpoint,
	openIDConfigurationEndpoint,
}

// WellKnownMetadataGenerator generates well-known metadata when the upstream
// authorization server doesn't provide certain endpoints.
// This allows supporting OIDC providers that only implement openid-configuration.
type WellKnownMetadataGenerator interface {
	// GenerateAuthorizationServerMetadata generates oauth-authorization-server metadata
	// from the openid-configuration. Returns nil if generation is not possible.
	GenerateAuthorizationServerMetadata(oidcConfig map[string]interface{}) map[string]interface{}

	// GenerateProtectedResourceMetadata generates oauth-protected-resource metadata (RFC 9728)
	// for the MCP server. authorizationServerURL is where OAuth metadata can be fetched.
	GenerateProtectedResourceMetadata(oidcConfig map[string]interface{}, authorizationServerURL string) map[string]interface{}
}

// DefaultMetadataGenerator provides standard metadata generation for OIDC providers
// that only implement openid-configuration (e.g., Entra ID, Auth0, etc.)
type DefaultMetadataGenerator struct{}

// GenerateAuthorizationServerMetadata returns the openid-configuration as-is,
// since it contains the required OAuth 2.0 Authorization Server Metadata fields.
func (g *DefaultMetadataGenerator) GenerateAuthorizationServerMetadata(oidcConfig map[string]interface{}) map[string]interface{} {
	return oidcConfig
}

// GenerateProtectedResourceMetadata generates RFC 9728 compliant metadata
// for the MCP server acting as an OAuth 2.0 protected resource.
func (g *DefaultMetadataGenerator) GenerateProtectedResourceMetadata(oidcConfig map[string]interface{}, authorizationServerURL string) map[string]interface{} {
	metadata := map[string]interface{}{
		"authorization_servers": []string{authorizationServerURL},
	}

	// Copy relevant fields from openid-configuration
	if scopes, ok := oidcConfig["scopes_supported"]; ok {
		metadata["scopes_supported"] = scopes
	}
	metadata["bearer_methods_supported"] = []string{"header"}

	return metadata
}

type WellKnown struct {
	oauthState        *oauth.State
	cfgState          *config.StaticConfigState
	metadataGenerator WellKnownMetadataGenerator
	// Cache for openid-configuration to avoid repeated fetches (TTL: oidcConfigCacheTTL)
	oidcConfigCache     map[string]interface{}
	oidcConfigCacheTime time.Time
	oidcConfigCacheURL  string // tracks which authURL the cache was fetched for
	oidcConfigCacheMu   sync.RWMutex
	oidcConfigFlight    singleflight.Group
}

var _ http.Handler = &WellKnown{}

func WellKnownHandler(cfgState *config.StaticConfigState, oauthState *oauth.State) http.Handler {
	return WellKnownHandlerWithGenerator(cfgState, oauthState, &DefaultMetadataGenerator{})
}

// WellKnownHandlerWithGenerator creates a WellKnown handler with a custom metadata generator.
// This allows customizing how metadata is generated for different OIDC providers.
func WellKnownHandlerWithGenerator(cfgState *config.StaticConfigState, oauthState *oauth.State, generator WellKnownMetadataGenerator) http.Handler {
	if generator == nil {
		generator = &DefaultMetadataGenerator{}
	}
	return &WellKnown{
		oauthState:        oauthState,
		cfgState:          cfgState,
		metadataGenerator: generator,
	}
}

// authorizationURL returns the current authorization URL from the oauth snapshot, trimming trailing slashes.
func (w *WellKnown) authorizationURL() string {
	snap := w.oauthState.Load()
	if snap == nil {
		return ""
	}
	return strings.TrimSuffix(snap.AuthorizationURL, "/")
}

// wellKnownHTTPClient returns the current HTTP client from the oauth snapshot,
// falling back to a TLS-enforcing client if none is available.
func (w *WellKnown) wellKnownHTTPClient() *http.Client {
	snap := w.oauthState.Load()
	if snap != nil && snap.HTTPClient != nil {
		return snap.HTTPClient
	}
	return config.NewTLSEnforcingClient(nil, w.cfgState.Load().IsRequireTLS)
}

func (w *WellKnown) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	authURL := w.authorizationURL()
	if authURL == "" {
		http.Error(writer, "Authorization URL is not configured", http.StatusNotFound)
		return
	}

	requestPath := request.URL.EscapedPath()

	// Validate the URL path to prevent path traversal
	upstreamURL, err := url.JoinPath(authURL, requestPath)
	if err != nil || !strings.HasPrefix(upstreamURL, authURL+"/") {
		http.Error(writer, "Invalid well-known path", http.StatusBadRequest)
		return
	}

	// Try direct proxy first (works for Keycloak and other providers that support all endpoints)
	resourceMetadata, respHeaders, err := w.fetchWellKnownEndpoint(request, upstreamURL)
	if err != nil {
		klog.V(1).Infof("Well-known proxy failed to fetch %s: %v", requestPath, err)
		http.Error(writer, "Failed to fetch well-known metadata", http.StatusInternalServerError)
		return
	}

	// If direct fetch returned nil (404), generate metadata using the configured generator.
	// This provides fallback support for OIDC providers that only implement openid-configuration.
	// Use prefix matching to handle paths like /.well-known/oauth-protected-resource/sse
	if resourceMetadata == nil {
		switch {
		case strings.HasPrefix(requestPath, oauthAuthorizationServerEndpoint):
			resourceMetadata, err = w.generateAuthorizationServerMetadata(request)
			if err != nil {
				klog.V(1).Infof("Well-known proxy failed to generate authorization server metadata: %v", err)
				http.Error(writer, "Failed to generate well-known metadata", http.StatusInternalServerError)
				return
			}
			respHeaders = nil
		case strings.HasPrefix(requestPath, oauthProtectedResourceEndpoint):
			resourceMetadata, err = w.generateProtectedResourceMetadata(request)
			if err != nil {
				klog.V(1).Infof("Well-known proxy failed to generate protected resource metadata: %v", err)
				http.Error(writer, "Failed to generate well-known metadata", http.StatusInternalServerError)
				return
			}
			respHeaders = nil
		}
		if resourceMetadata == nil {
			http.Error(writer, "Failed to fetch well-known metadata", http.StatusNotFound)
			return
		}
	}

	w.applyConfigOverrides(resourceMetadata)

	body, err := json.Marshal(resourceMetadata)
	if err != nil {
		klog.V(1).Infof("Well-known proxy failed to marshal response for %s: %v", request.URL.Path, err)
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Copy allowed headers from backend response if available
	for key, values := range respHeaders {
		if !allowedResponseHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		for _, value := range values {
			writer.Header().Add(key, value)
		}
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	withCORSHeaders(writer)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(body)
}

// fetchWellKnownEndpoint creates a new request from the incoming request's method and context,
// then fetches the well-known endpoint. Returns nil metadata if the endpoint returns 404.
func (w *WellKnown) fetchWellKnownEndpoint(request *http.Request, url string) (map[string]interface{}, http.Header, error) {
	req, err := http.NewRequestWithContext(request.Context(), request.Method, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}
	return w.fetchWellKnownEndpointFromRequest(req)
}

// fetchWellKnownEndpointFromRequest performs the HTTP fetch using a pre-built request.
// Returns nil metadata if the endpoint returns 404 (to allow fallback).
func (w *WellKnown) fetchWellKnownEndpointFromRequest(req *http.Request) (map[string]interface{}, http.Header, error) {
	resp, err := w.wellKnownHTTPClient().Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Return nil for 404 to trigger fallback
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	var resourceMetadata map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxWellKnownResponseSize)).Decode(&resourceMetadata); err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return resourceMetadata, resp.Header, nil
}

// fetchOpenIDConfiguration fetches and caches the openid-configuration from the authorization server.
// Uses singleflight to deduplicate concurrent fetches for the same authorization URL.
func (w *WellKnown) fetchOpenIDConfiguration(request *http.Request) (map[string]interface{}, error) {
	authURL := w.authorizationURL()

	// Check cache first (with TTL and URL match — invalidate if authorization URL changed)
	w.oidcConfigCacheMu.RLock()
	if w.oidcConfigCache != nil && w.oidcConfigCacheURL == authURL && time.Since(w.oidcConfigCacheTime) < oidcConfigCacheTTL {
		result := copyMap(w.oidcConfigCache)
		w.oidcConfigCacheMu.RUnlock()
		return result, nil
	}
	w.oidcConfigCacheMu.RUnlock()

	// singleflight deduplicates concurrent cache misses for the same authURL.
	// We use context.WithoutCancel so that if the winning request is cancelled,
	// the in-flight fetch still completes for all waiters.
	val, err, _ := w.oidcConfigFlight.Do(authURL, func() (any, error) {
		// Decouple from the caller's request lifecycle and apply our own timeout
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(request.Context()), oidcConfigFetchTimeout)
		defer cancel()

		fetchReq, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, authURL+openIDConfigurationEndpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create openid-configuration request: %w", err)
		}

		oidcConfig, _, err := w.fetchWellKnownEndpointFromRequest(fetchReq)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch openid-configuration: %w", err)
		}
		if oidcConfig == nil {
			return nil, nil
		}

		w.oidcConfigCacheMu.Lock()
		w.oidcConfigCache = copyMap(oidcConfig)
		w.oidcConfigCacheTime = time.Now()
		w.oidcConfigCacheURL = authURL
		w.oidcConfigCacheMu.Unlock()

		return oidcConfig, nil
	})
	if err != nil {
		return nil, err
	}
	if val == nil {
		return nil, nil
	}
	return copyMap(val.(map[string]any)), nil
}

// generateAuthorizationServerMetadata generates oauth-authorization-server metadata
// using the configured metadata generator and the fetched openid-configuration.
func (w *WellKnown) generateAuthorizationServerMetadata(request *http.Request) (map[string]interface{}, error) {
	oidcConfig, err := w.fetchOpenIDConfiguration(request)
	if err != nil {
		return nil, err
	}
	if oidcConfig == nil {
		return nil, nil
	}
	return w.metadataGenerator.GenerateAuthorizationServerMetadata(oidcConfig), nil
}

// generateProtectedResourceMetadata generates oauth-protected-resource metadata (RFC 9728)
// using the configured metadata generator.
func (w *WellKnown) generateProtectedResourceMetadata(request *http.Request) (map[string]interface{}, error) {
	oidcConfig, err := w.fetchOpenIDConfiguration(request)
	if err != nil {
		return nil, err
	}
	if oidcConfig == nil {
		return nil, nil
	}

	// MCP server URL - where OAuth metadata can be fetched
	mcpServerURL := w.buildResourceURL(request)
	return w.metadataGenerator.GenerateProtectedResourceMetadata(oidcConfig, mcpServerURL), nil
}

// buildResourceURL constructs the canonical resource URL for the MCP server.
// Uses server_url from config when set. Falls back to X-Forwarded-* headers only
// when trust_proxy_headers is explicitly enabled. Otherwise uses request.Host directly.
func (w *WellKnown) buildResourceURL(request *http.Request) string {
	cfg := w.cfgState.Load()
	if cfg.ServerURL != "" {
		return strings.TrimSuffix(cfg.ServerURL, "/")
	}
	scheme := "https"
	host := request.Host
	if cfg.TrustProxyHeaders {
		if request.TLS == nil && !strings.HasPrefix(request.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "http"
		}
		if fwdHost := request.Header.Get("X-Forwarded-Host"); fwdHost != "" {
			host = fwdHost
		}
	} else {
		if request.TLS == nil {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

// applyConfigOverrides applies server configuration overrides to the metadata.
func (w *WellKnown) applyConfigOverrides(resourceMetadata map[string]interface{}) {
	snap := w.oauthState.Load()
	if snap != nil && snap.DisableDynamicClientRegistration {
		delete(resourceMetadata, "registration_endpoint")
		resourceMetadata["require_request_uri_registration"] = false
	}
	if snap != nil && len(snap.OAuthScopes) > 0 {
		resourceMetadata["scopes_supported"] = snap.OAuthScopes
	}
}

// copyMap creates a shallow copy so the cached original is not mutated by
// applyConfigOverrides, which only modifies top-level keys.
func copyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func withCORSHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
