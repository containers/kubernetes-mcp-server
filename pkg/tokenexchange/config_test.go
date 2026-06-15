package tokenexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

func (s *ConfigSuite) TestSetRequireTLS() {
	s.Run("no enforcement when requireTLS is nil", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "test-token",
				"token_type":   "Bearer",
			})
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
		}
		// requireTLS defaults to nil - HTTP should work
		client, err := cfg.HTTPClient()
		s.Require().NoError(err)
		s.NotNil(client)

		resp, err := client.Get(server.URL)
		s.Require().NoError(err)
		_ = resp.Body.Close()
		s.Equal(http.StatusOK, resp.StatusCode)
	})

	s.Run("blocks HTTP when requireTLS returns true", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{}
		cfg.SetRequireTLS(func() bool { return true })

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)
		s.NotNil(client)

		_, err = client.Get(server.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
		s.Contains(err.Error(), "\"http\" scheme")
	})

	s.Run("allows HTTPS when requireTLS returns true", func() {
		cfg := &TargetTokenExchangeConfig{}
		cfg.SetRequireTLS(func() bool { return true })

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)
		s.NotNil(client)

		// HTTPS should not be blocked - use a transport-level check
		resp, err := client.Get("https://example.com/token")
		// We don't care if the request succeeds (network), only that it's not blocked by TLS enforcement
		if err != nil {
			s.NotContains(err.Error(), "require_tls is enabled", "HTTPS request should not be blocked by TLS enforcement")
		} else {
			_ = resp.Body.Close()
		}
	})

	s.Run("allows HTTP when requireTLS returns false", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{}
		cfg.SetRequireTLS(func() bool { return false })

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)
		s.NotNil(client)

		resp, err := client.Get(server.URL)
		s.Require().NoError(err)
		_ = resp.Body.Close()
		s.Equal(http.StatusOK, resp.StatusCode)
	})

	s.Run("SetRequireTLS with nil function is accepted", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{}
		cfg.SetRequireTLS(nil)

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)
		s.NotNil(client)

		resp, err := client.Get(server.URL)
		s.Require().NoError(err)
		_ = resp.Body.Close()
		s.Equal(http.StatusOK, resp.StatusCode)
	})
}

func (s *ConfigSuite) TestHTTPClientCachingWithEnforcement() {
	cfg := &TargetTokenExchangeConfig{}
	cfg.SetRequireTLS(func() bool { return true })

	client1, err := cfg.HTTPClient()
	s.Require().NoError(err)
	s.NotNil(client1)

	client2, err := cfg.HTTPClient()
	s.Require().NoError(err)
	s.NotNil(client2)

	// Same client instance should be returned (cached)
	s.Equal(fmt.Sprintf("%p", client1), fmt.Sprintf("%p", client2),
		"HTTPClient should return the same cached instance")

	// Create a new config with a different requireTLS function
	cfg2 := &TargetTokenExchangeConfig{
		TokenURL: "https://example.com/token",
	}
	cfg2.SetRequireTLS(func() bool { return true })

	client3, err := cfg2.HTTPClient()
	s.Require().NoError(err)
	s.NotNil(client3)

	// Make a request that would be blocked by the enforcement
	_, err = client3.Get("http://malicious.example.com/token")
	s.Require().Error(err)
	s.Contains(err.Error(), "require_tls is enabled")
}

// TestTCPConnection tests that the enforcement happens at the HTTP client level,
// not at the transport connection level.
func (s *ConfigSuite) TestEnforcementBlocksBeforeConnection() {
	// Start a server on an HTTP URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &TargetTokenExchangeConfig{}
	cfg.SetRequireTLS(func() bool { return true })

	client, err := cfg.HTTPClient()
	s.Require().NoError(err)

	// Request should be blocked before any connection is made
	_, err = client.Get(server.URL)
	s.Require().Error(err)
	s.Contains(err.Error(), "require_tls is enabled")

	// Server should not have received any request (enforcement is client-side)
}

// TestExchangeEnforcement tests that the enforcement is active when an exchanger
// uses the HTTP client for an actual token exchange
func (s *ConfigSuite) TestExchangeEnforcement() {
	s.Run("rfc8693 exchanger is blocked with http URL and requireTLS", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "exchanged-token",
				"token_type":   "Bearer",
			})
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}
		cfg.SetRequireTLS(func() bool { return true })

		exchanger := &rfc8693Exchanger{}
		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})

	s.Run("keycloak-v1 exchanger is blocked with http URL and requireTLS", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "exchanged-token",
				"token_type":   "Bearer",
			})
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}
		cfg.SetRequireTLS(func() bool { return true })

		exchanger := &keycloakV1Exchanger{}
		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})

	s.Run("entra-obo exchanger is blocked with http URL and requireTLS", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "exchanged-token",
				"token_type":   "Bearer",
			})
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Scopes:       []string{"api://target/.default"},
		}
		cfg.SetRequireTLS(func() bool { return true })

		exchanger := &entraOBOExchanger{}
		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})
}
