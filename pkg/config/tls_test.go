package config

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type TLSSuite struct {
	suite.Suite
}

func (s *TLSSuite) TestValidateURLRequiresTLS() {
	s.Run("returns nil for empty URL", func() {
		err := ValidateURLRequiresTLS("", "test_url")
		s.NoError(err)
	})

	s.Run("returns nil for HTTPS URL", func() {
		err := ValidateURLRequiresTLS("https://example.com/path", "test_url")
		s.NoError(err)
	})

	s.Run("returns nil for WSS URL", func() {
		err := ValidateURLRequiresTLS("wss://example.com/path", "test_url")
		s.NoError(err)
	})

	s.Run("returns error for HTTP URL", func() {
		err := ValidateURLRequiresTLS("http://example.com/path", "test_url")
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled but test_url uses \"http\" scheme (secure scheme required)")
	})

	s.Run("returns error for non-HTTPS scheme", func() {
		err := ValidateURLRequiresTLS("ftp://example.com/path", "test_url")
		s.Require().Error(err)
		s.Contains(err.Error(), "uses \"ftp\" scheme (secure scheme required)")
	})

	s.Run("includes field name in error message", func() {
		err := ValidateURLRequiresTLS("http://example.com", "my_custom_field")
		s.Require().Error(err)
		s.Contains(err.Error(), "my_custom_field")
	})

	s.Run("returns error for invalid URL", func() {
		err := ValidateURLRequiresTLS("://invalid", "test_url")
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid test_url")
	})
}

func (s *TLSSuite) TestTLSEnforcingTransport() {
	s.Run("allows HTTPS requests when require_tls is true", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		transport := NewTLSEnforcingTransport(server.Client().Transport, func() bool { return true })
		client := &http.Client{Transport: transport}

		resp, err := client.Get(server.URL)
		s.NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	})

	s.Run("blocks HTTP requests when require_tls is true", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		transport := NewTLSEnforcingTransport(http.DefaultTransport, func() bool { return true })
		client := &http.Client{Transport: transport}

		_, err := client.Get(server.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
		s.Contains(err.Error(), "secure scheme required")
	})

	s.Run("allows HTTP requests when require_tls is false", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		transport := NewTLSEnforcingTransport(http.DefaultTransport, func() bool { return false })
		client := &http.Client{Transport: transport}

		resp, err := client.Get(server.URL)
		s.NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	})

	s.Run("checks require_tls dynamically per request", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		requireTLS := false
		transport := NewTLSEnforcingTransport(http.DefaultTransport, func() bool { return requireTLS })
		client := &http.Client{Transport: transport}

		// First request with require_tls=false should succeed
		resp, err := client.Get(server.URL)
		s.NoError(err)
		_ = resp.Body.Close()

		// Change to require_tls=true, same client should now block
		requireTLS = true
		_, err = client.Get(server.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "secure scheme required")
	})

	s.Run("uses DefaultTransport when base is nil", func() {
		transport := NewTLSEnforcingTransport(nil, func() bool { return false })
		s.NotNil(transport)
		enforcing := transport.(*TLSEnforcingTransport)
		s.Equal(http.DefaultTransport, enforcing.Base)
	})
}

func (s *TLSSuite) TestNewTLSEnforcingClient() {
	s.Run("wraps existing client transport", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		baseClient := &http.Client{}
		client := NewTLSEnforcingClient(baseClient, func() bool { return true })

		_, err := client.Get(server.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "secure scheme required")
	})

	s.Run("handles nil base client", func() {
		client := NewTLSEnforcingClient(nil, func() bool { return false })
		s.NotNil(client)
		s.NotNil(client.Transport)
	})
}

func TestTLS(t *testing.T) {
	suite.Run(t, new(TLSSuite))
}
