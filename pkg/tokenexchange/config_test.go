package tokenexchange

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type TargetTokenExchangeConfigSuite struct {
	suite.Suite
}

func (s *TargetTokenExchangeConfigSuite) TestHTTPClientCAFile() {
	s.Run("without CAFile uses system roots", func() {
		client, err := (&TargetTokenExchangeConfig{}).HTTPClient()
		s.Require().NoError(err)

		wrapper, ok := client.Transport.(*tlsEnforcingTransport)
		s.Require().True(ok)
		transport, ok := wrapper.Base.(*http.Transport)
		s.Require().True(ok)
		s.Require().NotNil(transport.TLSClientConfig)
		s.Equal(uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
		s.Nil(transport.TLSClientConfig.RootCAs)
	})

	s.Run("trusts configured CA and rejects another CA", func() {
		trustedServer := s.newTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
		defer trustedServer.Close()
		untrustedServer := s.newTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
		defer untrustedServer.Close()

		cfg := &TargetTokenExchangeConfig{
			CAFile: s.writeCAFile(trustedServer.Certificate()),
		}

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)
		resp, err := client.Get(trustedServer.URL)
		s.Require().NoError(err)
		s.Require().NoError(resp.Body.Close())

		_, err = client.Get(untrustedServer.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "certificate signed by unknown authority")
	})

	s.Run("rebuilds TLS roots after CAFile changes", func() {
		oldServer := s.newTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("old"))
		}))
		defer oldServer.Close()
		newServer := s.newTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("new"))
		}))
		defer newServer.Close()

		cfg := &TargetTokenExchangeConfig{
			CAFile: s.writeCAFile(oldServer.Certificate()),
		}

		oldClient, err := cfg.HTTPClient()
		s.Require().NoError(err)
		resp, err := oldClient.Get(oldServer.URL)
		s.Require().NoError(err)
		s.Require().NoError(resp.Body.Close())

		sameClient, err := cfg.HTTPClient()
		s.Require().NoError(err)
		s.Same(oldClient, sameClient, "unchanged CAFile must reuse the memoized client")

		cfg.CAFile = s.writeCAFile(newServer.Certificate())
		newClient, err := cfg.HTTPClient()
		s.Require().NoError(err)
		s.NotSame(oldClient, newClient, "changed CAFile must rebuild the client")
		resp, err = newClient.Get(newServer.URL)
		s.Require().NoError(err)
		s.Require().NoError(resp.Body.Close())
	})

	s.Run("invalid CA file returns parse error", func() {
		path := filepath.Join(s.T().TempDir(), "ca.pem")
		s.Require().NoError(os.WriteFile(path, []byte("not a certificate"), 0o600))

		_, err := (&TargetTokenExchangeConfig{CAFile: path}).HTTPClient()
		s.Require().Error(err)
		s.Contains(err.Error(), "failed to parse CA certificate")
	})

	s.Run("missing CA file returns read error", func() {
		_, err := (&TargetTokenExchangeConfig{CAFile: filepath.Join(s.T().TempDir(), "missing.pem")}).HTTPClient()
		s.Require().Error(err)
		s.Contains(err.Error(), "failed to read CA file")
	})
}

func (s *TargetTokenExchangeConfigSuite) TestSetRequireTLS() {
	s.Run("no enforcement when requireTLS is nil", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		client, err := cfg.HTTPClient()
		s.Require().NoError(err)

		resp, err := client.Get(server.URL)
		s.Require().NoError(err)
		_ = resp.Body.Close()
		s.Equal(http.StatusOK, resp.StatusCode)
	})

	s.Run("blocks HTTP when requireTLS returns true", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{}
		cfg.SetRequireTLS(func() bool { return true })

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)

		_, err = client.Get(server.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})

	s.Run("allows HTTPS when requireTLS returns true", func() {
		server := s.newTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{
			CAFile: s.writeCAFile(server.Certificate()),
		}
		cfg.SetRequireTLS(func() bool { return true })

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)

		resp, err := client.Get(server.URL)
		s.Require().NoError(err)
		_ = resp.Body.Close()
		s.Equal(http.StatusOK, resp.StatusCode)
	})

	s.Run("allows HTTP when requireTLS returns false", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &TargetTokenExchangeConfig{}
		cfg.SetRequireTLS(func() bool { return false })

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)

		resp, err := client.Get(server.URL)
		s.Require().NoError(err)
		_ = resp.Body.Close()
		s.Equal(http.StatusOK, resp.StatusCode)
	})

	s.Run("evaluates requireTLS per request not at build time", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		enforce := true
		cfg := &TargetTokenExchangeConfig{}
		cfg.SetRequireTLS(func() bool { return enforce })

		client, err := cfg.HTTPClient()
		s.Require().NoError(err)

		_, err = client.Get(server.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")

		enforce = false
		resp, err := client.Get(server.URL)
		s.Require().NoError(err)
		_ = resp.Body.Close()
		s.Equal(http.StatusOK, resp.StatusCode)
	})

	s.Run("enforces when the enforcer is set after the client is built", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Build (and memoize) the client with no enforcer installed.
		cfg := &TargetTokenExchangeConfig{}
		client, err := cfg.HTTPClient()
		s.Require().NoError(err)

		resp, err := client.Get(server.URL)
		s.Require().NoError(err)
		_ = resp.Body.Close()

		// Always-wrapping means a later SetRequireTLS still takes effect on the
		// same memoized client, with no rebuild (guards against an unwrapped
		// client being cached before the enforcer is wired).
		cfg.SetRequireTLS(func() bool { return true })
		_, err = client.Get(server.URL)
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled")
	})
}

func (s *TargetTokenExchangeConfigSuite) TestExchangeEnforcement() {
	s.Run("rfc8693 exchanger is blocked with http URL and requireTLS", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func (s *TargetTokenExchangeConfigSuite) newTLSServer(handler http.Handler) *httptest.Server {
	s.T().Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	s.Require().NoError(err)

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{der},
			PrivateKey:  key,
		}},
	}
	server.StartTLS()
	return server
}

func (s *TargetTokenExchangeConfigSuite) writeCAFile(cert *x509.Certificate) string {
	s.T().Helper()

	path := filepath.Join(s.T().TempDir(), "ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	s.Require().NoError(os.WriteFile(path, certPEM, 0o600))
	return path
}

func TestTargetTokenExchangeConfig(t *testing.T) {
	suite.Run(t, new(TargetTokenExchangeConfigSuite))
}
