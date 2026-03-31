package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"k8s.io/klog/v2"
)

const maxWellKnownResponseSize = 1 << 20 // 1 MB

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

type WellKnown struct {
	authorizationUrl                 string
	scopesSupported                  []string
	disableDynamicClientRegistration bool
	httpClient                       *http.Client
}

var _ http.Handler = &WellKnown{}

func WellKnownHandler(staticConfig *config.StaticConfig, httpClient *http.Client) http.Handler {
	authorizationUrl := staticConfig.AuthorizationURL
	if authorizationUrl != "" && strings.HasSuffix(authorizationUrl, "/") {
		authorizationUrl = strings.TrimSuffix(authorizationUrl, "/")
	}
	if httpClient == nil {
		// Create a TLS-enforcing client instead of using http.DefaultClient
		httpClient = config.NewTLSEnforcingClient(nil, staticConfig.IsRequireTLS)
	}
	return &WellKnown{
		authorizationUrl:                 authorizationUrl,
		disableDynamicClientRegistration: staticConfig.DisableDynamicClientRegistration,
		scopesSupported:                  staticConfig.OAuthScopes,
		httpClient:                       httpClient,
	}
}

func (w WellKnown) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if w.authorizationUrl == "" {
		http.Error(writer, "Authorization URL is not configured", http.StatusNotFound)
		return
	}
	upstreamURL, err := url.JoinPath(w.authorizationUrl, request.URL.EscapedPath())
	if err != nil || !strings.HasPrefix(upstreamURL, w.authorizationUrl+"/") {
		http.Error(writer, "Invalid well-known path", http.StatusBadRequest)
		return
	}
	req, err := http.NewRequest(request.Method, upstreamURL, nil)
	if err != nil {
		klog.V(1).Infof("Well-known proxy failed to create request for %s: %v", request.URL.Path, err)
		http.Error(writer, "Failed to create upstream request", http.StatusInternalServerError)
		return
	}
	resp, err := w.httpClient.Do(req.WithContext(request.Context()))
	if err != nil {
		klog.V(1).Infof("Well-known proxy request failed for %s: %v", request.URL.Path, err)
		http.Error(writer, "Failed to fetch upstream well-known metadata", http.StatusInternalServerError)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	var resourceMetadata map[string]interface{}
	err = json.NewDecoder(io.LimitReader(resp.Body, maxWellKnownResponseSize)).Decode(&resourceMetadata)
	if err != nil {
		klog.V(1).Infof("Well-known proxy failed to decode response for %s: %v", request.URL.Path, err)
		http.Error(writer, "Failed to read upstream response", http.StatusInternalServerError)
		return
	}
	if w.disableDynamicClientRegistration {
		delete(resourceMetadata, "registration_endpoint")
		resourceMetadata["require_request_uri_registration"] = false
	}
	if len(w.scopesSupported) > 0 {
		resourceMetadata["scopes_supported"] = w.scopesSupported
	}
	body, err := json.Marshal(resourceMetadata)
	if err != nil {
		klog.V(1).Infof("Well-known proxy failed to marshal response for %s: %v", request.URL.Path, err)
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return
	}
	for key, values := range resp.Header {
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
	writer.WriteHeader(resp.StatusCode)
	_, _ = writer.Write(body)
}

func withCORSHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
