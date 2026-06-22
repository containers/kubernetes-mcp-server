package tokenexchange

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
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

		transport, ok := client.Transport.(*http.Transport)
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
