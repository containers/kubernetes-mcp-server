package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/suite"
)

type TLSUtilSuite struct {
	suite.Suite
}

func TestTLSUtil(t *testing.T) {
	suite.Run(t, new(TLSUtilSuite))
}

func (s *TLSUtilSuite) TestParseTLSVersion() {
	s.Run("returns TLS 1.2 for empty string", func() {
		v, err := ParseTLSVersion("")
		s.Require().NoError(err)
		s.Equal(tls.VersionTLS12, int(v))
	})

	s.Run("parses TLS 1.0", func() {
		v, err := ParseTLSVersion("1.0")
		s.Require().NoError(err)
		s.Equal(tls.VersionTLS10, int(v))
	})

	s.Run("parses TLS 1.1", func() {
		v, err := ParseTLSVersion("1.1")
		s.Require().NoError(err)
		s.Equal(tls.VersionTLS11, int(v))
	})

	s.Run("parses TLS 1.2", func() {
		v, err := ParseTLSVersion("1.2")
		s.Require().NoError(err)
		s.Equal(tls.VersionTLS12, int(v))
	})

	s.Run("parses TLS 1.3", func() {
		v, err := ParseTLSVersion("1.3")
		s.Require().NoError(err)
		s.Equal(tls.VersionTLS13, int(v))
	})

	s.Run("trims whitespace", func() {
		v, err := ParseTLSVersion("  1.3  ")
		s.Require().NoError(err)
		s.Equal(tls.VersionTLS13, int(v))
	})

	s.Run("returns error for invalid version", func() {
		_, err := ParseTLSVersion("1.4")
		s.Error(err)
		s.Contains(err.Error(), "invalid TLS version")
	})

	s.Run("returns error for non-numeric version", func() {
		_, err := ParseTLSVersion("abc")
		s.Error(err)
		s.Contains(err.Error(), "invalid TLS version")
	})
}

func (s *TLSUtilSuite) TestParseTLSCipherSuites() {
	s.Run("returns nil for empty slice", func() {
		result, err := ParseTLSCipherSuites(nil)
		s.Require().NoError(err)
		s.Nil(result)
	})

	s.Run("returns nil for empty strings", func() {
		result, err := ParseTLSCipherSuites([]string{"", "  "})
		s.Require().NoError(err)
		s.Nil(result)
	})

	s.Run("parses valid cipher suite", func() {
		result, err := ParseTLSCipherSuites([]string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"})
		s.Require().NoError(err)
		s.Len(result, 1)
	})

	s.Run("parses multiple cipher suites", func() {
		result, err := ParseTLSCipherSuites([]string{
			"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		})
		s.Require().NoError(err)
		s.Len(result, 2)
	})

	s.Run("trims whitespace from suite names", func() {
		result, err := ParseTLSCipherSuites([]string{"  TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256  "})
		s.Require().NoError(err)
		s.Len(result, 1)
	})

	s.Run("returns error for unknown cipher suite", func() {
		_, err := ParseTLSCipherSuites([]string{"INVALID_CIPHER_SUITE"})
		s.Error(err)
		s.Contains(err.Error(), "unknown cipher suite")
	})

	s.Run("rejects insecure cipher suites", func() {
		_, err := ParseTLSCipherSuites([]string{"TLS_RSA_WITH_RC4_128_SHA"})
		s.Error(err)
		s.Contains(err.Error(), "unknown cipher suite")
	})

	s.Run("returns combined errors for multiple unknown suites", func() {
		_, err := ParseTLSCipherSuites([]string{"INVALID_1", "INVALID_2"})
		s.Error(err)
		s.Contains(err.Error(), "INVALID_1")
		s.Contains(err.Error(), "INVALID_2")
	})
}

func (s *TLSUtilSuite) TestBuildTLSConfig() {
	s.Run("returns TLS config with default TLS 1.2", func() {
		cfg, err := BuildTLSConfig("", nil)
		s.Require().NoError(err)
		s.NotNil(cfg)
		s.Equal(tls.VersionTLS12, int(cfg.MinVersion))
	})

	s.Run("applies passed min version and cipher suites", func() {
		cfg, err := BuildTLSConfig("1.3", []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"})
		s.Require().NoError(err)
		s.Equal(tls.VersionTLS13, int(cfg.MinVersion))
		s.Len(cfg.CipherSuites, 1)
	})

	s.Run("applies InsecureSkipVerify option", func() {
		cfg, err := BuildTLSConfig("", nil, WithInsecureSkipVerify(true))
		s.Require().NoError(err)
		s.True(cfg.InsecureSkipVerify)
	})

	s.Run("applies RootCAs option", func() {
		certPool := x509.NewCertPool()
		cfg, err := BuildTLSConfig("", nil, WithRootCAs(certPool))
		s.Require().NoError(err)
		s.Equal(certPool, cfg.RootCAs)
	})
}
