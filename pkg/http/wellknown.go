package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

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
		httpClient = http.DefaultClient
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
	req, err := http.NewRequest(request.Method, w.authorizationUrl+request.URL.EscapedPath(), nil)
	if err != nil {
		http.Error(writer, "Failed to create request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := w.httpClient.Do(req.WithContext(request.Context()))
	if err != nil {
		http.Error(writer, "Failed to perform request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	var resourceMetadata map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&resourceMetadata)
	if err != nil {
		http.Error(writer, "Failed to read response body: "+err.Error(), http.StatusInternalServerError)
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
		http.Error(writer, "Failed to marshal response body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for key, values := range resp.Header {
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
