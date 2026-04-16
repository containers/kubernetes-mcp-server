package tokenexchange

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/suite"
)

type AssertionTestSuite struct {
	suite.Suite
	tempDir  string
	certFile string
	keyFile  string
}

func (s *AssertionTestSuite) SetupTest() {
	var err error
	s.tempDir, err = os.MkdirTemp("", "assertion-test-*")
	s.Require().NoError(err)

	s.certFile, s.keyFile = s.generateTestCertAndKey()
}

func (s *AssertionTestSuite) TearDownTest() {
	_ = os.RemoveAll(s.tempDir)
}

func (s *AssertionTestSuite) generateTestCertAndKey() (string, string) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	s.Require().NoError(err)

	certFile := filepath.Join(s.tempDir, "cert.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	s.Require().NoError(os.WriteFile(certFile, certPEM, 0644))

	keyFile := filepath.Join(s.tempDir, "key.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	s.Require().NoError(os.WriteFile(keyFile, keyPEM, 0600))

	return certFile, keyFile
}

func (s *AssertionTestSuite) TestBuildClientAssertion() {
	s.Run("builds valid JWT with correct claims", func() {
		clientID := "test-client-id"
		tokenURL := "https://login.microsoftonline.com/tenant/oauth2/v2.0/token"

		assertion, expiry, err := BuildClientAssertion(clientID, tokenURL, s.certFile, s.keyFile, 5*time.Minute)
		s.Require().NoError(err)
		s.NotEmpty(assertion)
		s.True(expiry.After(time.Now()))

		token, err := jwt.ParseSigned(assertion, []jose.SignatureAlgorithm{jose.RS256})
		s.Require().NoError(err)

		var claims jwt.Claims
		err = token.UnsafeClaimsWithoutVerification(&claims)
		s.Require().NoError(err)

		s.Equal(clientID, claims.Issuer)
		s.Equal(clientID, claims.Subject)
		s.True(claims.Audience.Contains(tokenURL))
		s.NotEmpty(claims.ID)
	})

	s.Run("includes x5t#S256 header", func() {
		assertion, _, err := BuildClientAssertion("client", "https://token.url", s.certFile, s.keyFile, 0)
		s.Require().NoError(err)

		token, err := jwt.ParseSigned(assertion, []jose.SignatureAlgorithm{jose.RS256})
		s.Require().NoError(err)

		s.Len(token.Headers, 1)
		x5tS256, ok := token.Headers[0].ExtraHeaders["x5t#S256"]
		s.True(ok, "x5t#S256 header should be present")
		s.NotEmpty(x5tS256)
	})

	s.Run("uses default lifetime when zero", func() {
		_, expiry, err := BuildClientAssertion("client", "https://token.url", s.certFile, s.keyFile, 0)
		s.Require().NoError(err)

		expectedExpiry := time.Now().Add(DefaultAssertionLifetime)
		s.InDelta(expectedExpiry.Unix(), expiry.Unix(), 5)
	})

	s.Run("returns error for missing cert file", func() {
		_, _, err := BuildClientAssertion("client", "https://token.url", "/nonexistent/cert.pem", s.keyFile, 0)
		s.Error(err)
		s.Contains(err.Error(), "failed to read certificate file")
	})

	s.Run("returns error for missing key file", func() {
		_, _, err := BuildClientAssertion("client", "https://token.url", s.certFile, "/nonexistent/key.pem", 0)
		s.Error(err)
		s.Contains(err.Error(), "failed to read private key file")
	})

	s.Run("returns error for invalid cert PEM", func() {
		invalidCert := filepath.Join(s.tempDir, "invalid-cert.pem")
		s.Require().NoError(os.WriteFile(invalidCert, []byte("not a valid PEM"), 0644))

		_, _, err := BuildClientAssertion("client", "https://token.url", invalidCert, s.keyFile, 0)
		s.Error(err)
		s.Contains(err.Error(), "failed to decode PEM block")
	})

	s.Run("returns error for invalid key PEM", func() {
		invalidKey := filepath.Join(s.tempDir, "invalid-key.pem")
		s.Require().NoError(os.WriteFile(invalidKey, []byte("not a valid PEM"), 0600))

		_, _, err := BuildClientAssertion("client", "https://token.url", s.certFile, invalidKey, 0)
		s.Error(err)
		s.Contains(err.Error(), "failed to decode PEM block")
	})
}

func (s *AssertionTestSuite) TestGetOrBuildAssertion() {
	s.Run("caches assertion", func() {
		cfg := &TargetTokenExchangeConfig{
			TokenURL:       "https://token.url",
			ClientID:       "test-client",
			ClientCertFile: s.certFile,
			ClientKeyFile:  s.keyFile,
		}

		assertion1, err := cfg.GetOrBuildAssertion()
		s.Require().NoError(err)

		assertion2, err := cfg.GetOrBuildAssertion()
		s.Require().NoError(err)

		s.Equal(assertion1, assertion2, "should return cached assertion")
	})

	s.Run("returns error for missing files", func() {
		cfg := &TargetTokenExchangeConfig{
			TokenURL:       "https://token.url",
			ClientID:       "test-client",
			ClientCertFile: "/nonexistent/cert.pem",
			ClientKeyFile:  "/nonexistent/key.pem",
		}

		_, err := cfg.GetOrBuildAssertion()
		s.Error(err)
	})
}

func (s *AssertionTestSuite) TestLoadCertificateAndKey() {
	s.Run("loads PKCS1 RSA key", func() {
		cert, key, err := loadCertificateAndKey(s.certFile, s.keyFile)
		s.Require().NoError(err)
		s.NotNil(cert)
		s.NotNil(key)
	})

	s.Run("loads PKCS8 RSA key", func() {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		s.Require().NoError(err)

		pkcs8Key, err := x509.MarshalPKCS8PrivateKey(privateKey)
		s.Require().NoError(err)

		pkcs8KeyFile := filepath.Join(s.tempDir, "pkcs8-key.pem")
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Key})
		s.Require().NoError(os.WriteFile(pkcs8KeyFile, keyPEM, 0600))

		cert, key, err := loadCertificateAndKey(s.certFile, pkcs8KeyFile)
		s.Require().NoError(err)
		s.NotNil(cert)
		s.NotNil(key)
	})
}

func (s *AssertionTestSuite) TestComputeX5TS256() {
	certPEM, err := os.ReadFile(s.certFile)
	s.Require().NoError(err)

	block, _ := pem.Decode(certPEM)
	s.Require().NotNil(block)

	cert, err := x509.ParseCertificate(block.Bytes)
	s.Require().NoError(err)

	x5tS256 := computeX5TS256(cert)
	s.NotEmpty(x5tS256)
	s.Len(x5tS256, 43) // Base64url encoded SHA-256 (32 bytes) = 43 chars without padding
}

func (s *AssertionTestSuite) TestValidate() {
	s.Run("valid assertion config", func() {
		cfg := &TargetTokenExchangeConfig{
			AuthStyle:      AuthStyleAssertion,
			ClientCertFile: "/path/to/cert.pem",
			ClientKeyFile:  "/path/to/key.pem",
		}
		err := cfg.Validate()
		s.NoError(err)
	})

	s.Run("assertion requires cert file", func() {
		cfg := &TargetTokenExchangeConfig{
			AuthStyle:     AuthStyleAssertion,
			ClientKeyFile: "/path/to/key.pem",
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "client_cert_file is required")
	})

	s.Run("assertion requires key file", func() {
		cfg := &TargetTokenExchangeConfig{
			AuthStyle:      AuthStyleAssertion,
			ClientCertFile: "/path/to/cert.pem",
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "client_key_file is required")
	})

	s.Run("params style is valid", func() {
		cfg := &TargetTokenExchangeConfig{
			AuthStyle: AuthStyleParams,
		}
		err := cfg.Validate()
		s.NoError(err)
	})

	s.Run("header style is valid", func() {
		cfg := &TargetTokenExchangeConfig{
			AuthStyle: AuthStyleHeader,
		}
		err := cfg.Validate()
		s.NoError(err)
	})

	s.Run("invalid auth style", func() {
		cfg := &TargetTokenExchangeConfig{
			AuthStyle: "invalid",
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "invalid auth_style")
	})
}

func (s *AssertionTestSuite) TestLoadCertificateAndKeyRejectsECKeys() {
	s.Run("rejects EC private key in SEC1 format", func() {
		ecKeyFile := s.generateECKeyFile("EC PRIVATE KEY", false)
		_, _, err := loadCertificateAndKey(s.certFile, ecKeyFile)
		s.Error(err)
		s.Contains(err.Error(), "failed to parse private key")
	})

	s.Run("rejects EC private key in PKCS8 format", func() {
		ecKeyFile := s.generateECKeyFile("PRIVATE KEY", true)
		_, _, err := loadCertificateAndKey(s.certFile, ecKeyFile)
		s.Error(err)
		s.Contains(err.Error(), "unsupported key type")
	})
}

func (s *AssertionTestSuite) generateECKeyFile(pemType string, pkcs8 bool) string {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s.Require().NoError(err)

	var keyBytes []byte
	if pkcs8 {
		keyBytes, err = x509.MarshalPKCS8PrivateKey(ecKey)
		s.Require().NoError(err)
	} else {
		keyBytes, err = x509.MarshalECPrivateKey(ecKey)
		s.Require().NoError(err)
	}

	ecKeyFile := filepath.Join(s.tempDir, "ec-key.pem")
	ecPEM := pem.EncodeToMemory(&pem.Block{Type: pemType, Bytes: keyBytes})
	s.Require().NoError(os.WriteFile(ecKeyFile, ecPEM, 0600))
	return ecKeyFile
}

func (s *AssertionTestSuite) TestInjectClientAuthWithAssertion() {
	s.Run("sets client assertion form fields", func() {
		cfg := &TargetTokenExchangeConfig{
			TokenURL:       "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
			ClientID:       "test-client",
			AuthStyle:      AuthStyleAssertion,
			ClientCertFile: s.certFile,
			ClientKeyFile:  s.keyFile,
		}

		data := url.Values{}
		header := http.Header{}
		err := injectClientAuth(cfg, data, header)
		s.Require().NoError(err)

		s.Equal("test-client", data.Get(FormKeyClientID))
		s.Equal(ClientAssertionType, data.Get(FormKeyClientAssertionType))
		s.NotEmpty(data.Get(FormKeyClientAssertion), "client_assertion should be set")
		s.Empty(data.Get(FormKeyClientSecret), "client_secret should not be set for assertion auth")
		s.Empty(header.Get(HeaderAuthorization), "Authorization header should not be set for assertion auth")
	})

	s.Run("returns error for invalid cert files", func() {
		cfg := &TargetTokenExchangeConfig{
			TokenURL:       "https://token.url",
			ClientID:       "test-client",
			AuthStyle:      AuthStyleAssertion,
			ClientCertFile: "/nonexistent/cert.pem",
			ClientKeyFile:  "/nonexistent/key.pem",
		}

		data := url.Values{}
		header := http.Header{}
		err := injectClientAuth(cfg, data, header)
		s.Error(err)
		s.Contains(err.Error(), "failed to build client assertion")
	})
}

func TestAssertion(t *testing.T) {
	suite.Run(t, new(AssertionTestSuite))
}
