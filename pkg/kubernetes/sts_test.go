package kubernetes

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func TestIsEnabled(t *testing.T) {
	disabledCases := []SecurityTokenService{
		{},
		{Provider: nil},
		{Provider: &oidc.Provider{}},
		{Provider: &oidc.Provider{}, ClientId: "test-client-id", ClientSecret: "test-client-secret"},
		{ClientId: "test-client-id", ClientSecret: "test-client-secret", ExternalAccountAudience: "test-audience"},
		{Provider: &oidc.Provider{}, ClientSecret: "test-client-secret", ExternalAccountAudience: "test-audience"},
	}
	for _, sts := range disabledCases {
		t.Run(fmt.Sprintf("SecurityTokenService{%+v}.IsEnabled() = false", sts), func(t *testing.T) {
			if sts.IsEnabled() {
				t.Errorf("SecurityTokenService{%+v}.IsEnabled() = true; want false", sts)
			}
		})
	}
	enabledCases := []SecurityTokenService{
		{Provider: &oidc.Provider{}, ClientId: "test-client-id", ExternalAccountAudience: "test-audience"},
		{Provider: &oidc.Provider{}, ClientId: "test-client-id", ExternalAccountAudience: "test-audience", ClientSecret: "test-client-secret"},
		{Provider: &oidc.Provider{}, ClientId: "test-client-id", ExternalAccountAudience: "test-audience", ClientSecret: "test-client-secret", ExternalAccountScopes: []string{"test-scope"}},
	}
	for _, sts := range enabledCases {
		t.Run(fmt.Sprintf("SecurityTokenService{%+v}.IsEnabled() = true", sts), func(t *testing.T) {
			if !sts.IsEnabled() {
				t.Errorf("SecurityTokenService{%+v}.IsEnabled() = false; want true", sts)
			}
		})
	}
}

func TestExternalAccountTokenExchange(t *testing.T) {
	mockServer := test.NewMockServer()
	authServer := mockServer.Config().Host
	var tokenExchangeRequest *http.Request
	mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"issuer": "%s",
				"authorization_endpoint": "https://mock-oidc-provider/authorize",
				"token_endpoint": "%s/token"
			}`, authServer, authServer)
			return
		}
		if req.URL.Path == "/token" {
			tokenExchangeRequest = req
			_ = tokenExchangeRequest.ParseForm()
			if tokenExchangeRequest.PostForm.Get("subject_token") != "the-original-access-token" {
				http.Error(w, "Invalid subject_token", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"exchanged-access-token","token_type":"Bearer","expires_in":253402297199}`))
			return
		}
	}))
	t.Cleanup(mockServer.Close)
	provider, err := oidc.NewProvider(t.Context(), authServer)
	if err != nil {
		t.Fatalf("oidc.NewProvider() error = %v; want nil", err)
	}
	// With missing Token Source information
	_, err = (&SecurityTokenService{Provider: provider}).ExternalAccountTokenExchange(t.Context(), &oauth2.Token{})
	t.Run("ExternalAccountTokenExchange with missing token source returns error", func(t *testing.T) {
		if err == nil {
			t.Fatalf("ExternalAccountTokenExchange() error = nil; want error")
		}
		if !strings.Contains(err.Error(), "must be set") {
			t.Errorf("ExternalAccountTokenExchange() error = %v; want missing required field", err)
		}
	})
	// With valid Token Source information
	sts := SecurityTokenService{
		Provider:                provider,
		ClientId:                "test-client-id",
		ClientSecret:            "test-client-secret",
		ExternalAccountAudience: "test-audience",
		ExternalAccountScopes:   []string{"test-scope"},
	}
	// With Invalid token
	_, err = sts.ExternalAccountTokenExchange(t.Context(), &oauth2.Token{
		AccessToken: "invalid-access-token",
		TokenType:   "Bearer",
	})
	t.Run("ExternalAccountTokenExchange with invalid token returns error", func(t *testing.T) {
		if err == nil {
			t.Fatalf("ExternalAccountTokenExchange() error = nil; want error")
		}
		if !strings.Contains(err.Error(), "status code 401: Invalid subject_token") {
			t.Errorf("ExternalAccountTokenExchange() error = %v; want invalid_grant: Invalid subject_token", err)
		}
	})
	// With Valid token
	exchangeToken, err := sts.ExternalAccountTokenExchange(t.Context(), &oauth2.Token{
		AccessToken: "the-original-access-token",
		TokenType:   "Bearer",
	})
	t.Run("ExternalAccountTokenExchange with valid token returns new token", func(t *testing.T) {
		if err != nil {
			t.Errorf("ExternalAccountTokenExchange() error = %v; want nil", err)
		}
		if exchangeToken == nil {
			t.Fatal("ExternalAccountTokenExchange() = nil; want token")
		}
		if exchangeToken.AccessToken != "exchanged-access-token" {
			t.Errorf("exchangeToken.AccessToken = %s; want exchanged-access-token", exchangeToken.AccessToken)
		}
	})
	t.Run("ExternalAccountTokenExchange with valid token sends POST request", func(t *testing.T) {
		if tokenExchangeRequest == nil {
			t.Fatal("tokenExchangeRequest is nil; want request")
		}
		if tokenExchangeRequest.Method != "POST" {
			t.Errorf("tokenExchangeRequest.Method = %s; want POST", tokenExchangeRequest.Method)
		}
	})
	t.Run("ExternalAccountTokenExchange with valid token has correct form data", func(t *testing.T) {
		if tokenExchangeRequest.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("tokenExchangeRequest.Content-Type = %s; want application/x-www-form-urlencoded", tokenExchangeRequest.Header.Get("Content-Type"))
		}
		if tokenExchangeRequest.PostForm.Get("audience") != "test-audience" {
			t.Errorf("tokenExchangeRequest.PostForm[audience] = %s; want test-audience", tokenExchangeRequest.PostForm.Get("audience"))
		}
		if tokenExchangeRequest.PostForm.Get("subject_token_type") != "urn:ietf:params:oauth:token-type:access_token" {
			t.Errorf("tokenExchangeRequest.PostForm[subject_token_type] = %s; want urn:ietf:params:oauth:token-type:access_token", tokenExchangeRequest.PostForm.Get("subject_token_type"))
		}
		if tokenExchangeRequest.PostForm.Get("subject_token") != "the-original-access-token" {
			t.Errorf("tokenExchangeRequest.PostForm[subject_token] = %s; want the-original-access-token", tokenExchangeRequest.PostForm.Get("subject_token"))
		}
		if len(tokenExchangeRequest.PostForm["scope"]) == 0 || tokenExchangeRequest.PostForm["scope"][0] != "test-scope" {
			t.Errorf("tokenExchangeRequest.PostForm[scope] = %v; want [test-scope]", tokenExchangeRequest.PostForm["scope"])
		}
	})
	t.Run("ExternalAccountTokenExchange with valid token sends correct client credentials header", func(t *testing.T) {
		if tokenExchangeRequest.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("test-client-id:test-client-secret")) {
			t.Errorf("tokenExchangeRequest.Header[Authorization] = %s; want Basic base64(test-client-id:test-client-secret)", tokenExchangeRequest.Header.Get("Authorization"))
		}
	})
}
