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

	s.Run("federated style is valid with token file", func() {
		cfg := &TargetTokenExchangeConfig{
			AuthStyle:          AuthStyleFederated,
			FederatedTokenFile: "/path/to/token",
		}
		err := cfg.Validate()
		s.NoError(err)
	})

	s.Run("federated style requires token file", func() {
		cfg := &TargetTokenExchangeConfig{
			AuthStyle: AuthStyleFederated,
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "federated_token_file is required")
	})
}

func (s *AssertionTestSuite) TestLoadCertificateAndKeyWithECKeys() {
	s.Run("loads EC P-256 private key in PKCS8 format", func() {
		ecCertFile, ecKeyFile := s.generateECCertAndKey(elliptic.P256())
		cert, key, err := loadCertificateAndKey(ecCertFile, ecKeyFile)
		s.Require().NoError(err)
		s.NotNil(cert)
		s.IsType(&ecdsa.PrivateKey{}, key)
	})

	s.Run("loads EC P-384 private key in PKCS8 format", func() {
		ecCertFile, ecKeyFile := s.generateECCertAndKey(elliptic.P384())
		cert, key, err := loadCertificateAndKey(ecCertFile, ecKeyFile)
		s.Require().NoError(err)
		s.NotNil(cert)
		s.IsType(&ecdsa.PrivateKey{}, key)
	})

	s.Run("rejects EC private key in SEC1 format", func() {
		ecKeyFile := s.generateECKeyFile("EC PRIVATE KEY", false)
		_, _, err := loadCertificateAndKey(s.certFile, ecKeyFile)
		s.Error(err)
		s.Contains(err.Error(), "failed to parse private key")
	})
}

func (s *AssertionTestSuite) TestBuildClientAssertionWithECKey() {
	s.Run("builds valid JWT with EC P-256 key", func() {
		ecCertFile, ecKeyFile := s.generateECCertAndKey(elliptic.P256())
		clientID := "spiffe-client"
		tokenURL := "https://login.microsoftonline.com/tenant/oauth2/v2.0/token"

		assertion, expiry, err := BuildClientAssertion(clientID, tokenURL, ecCertFile, ecKeyFile, 5*time.Minute)
		s.Require().NoError(err)
		s.NotEmpty(assertion)
		s.True(expiry.After(time.Now()))

		token, err := jwt.ParseSigned(assertion, []jose.SignatureAlgorithm{jose.ES256})
		s.Require().NoError(err)

		var claims jwt.Claims
		err = token.UnsafeClaimsWithoutVerification(&claims)
		s.Require().NoError(err)

		s.Equal(clientID, claims.Issuer)
		s.Equal(clientID, claims.Subject)
		s.True(claims.Audience.Contains(tokenURL))
	})
}

func (s *AssertionTestSuite) generateECCertAndKey(curve elliptic.Curve) (string, string) {
	ecKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	s.Require().NoError(err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-ec"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &ecKey.PublicKey, ecKey)
	s.Require().NoError(err)

	certFile := filepath.Join(s.tempDir, curve.Params().Name+"-cert.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	s.Require().NoError(os.WriteFile(certFile, certPEM, 0644))

	keyBytes, err := x509.MarshalPKCS8PrivateKey(ecKey)
	s.Require().NoError(err)

	keyFile := filepath.Join(s.tempDir, curve.Params().Name+"-key.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	s.Require().NoError(os.WriteFile(keyFile, keyPEM, 0600))

	return certFile, keyFile
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

func (s *AssertionTestSuite) TestInjectClientAuthWithFederated() {
	s.Run("reads token from file and sets client assertion fields", func() {
		tokenFile := filepath.Join(s.tempDir, "federated-token")
		s.Require().NoError(os.WriteFile(tokenFile, []byte("eyJhbGciOiJSUzI1NiJ9.test.signature"), 0600))

		cfg := &TargetTokenExchangeConfig{
			ClientID:           "test-client",
			AuthStyle:          AuthStyleFederated,
			FederatedTokenFile: tokenFile,
		}

		data := url.Values{}
		header := http.Header{}
		err := injectClientAuth(cfg, data, header)
		s.Require().NoError(err)

		s.Equal("test-client", data.Get(FormKeyClientID))
		s.Equal(ClientAssertionType, data.Get(FormKeyClientAssertionType))
		s.Equal("eyJhbGciOiJSUzI1NiJ9.test.signature", data.Get(FormKeyClientAssertion))
		s.Empty(data.Get(FormKeyClientSecret))
		s.Empty(header.Get(HeaderAuthorization))
	})

	s.Run("trims whitespace from token file contents", func() {
		tokenFile := filepath.Join(s.tempDir, "federated-token-ws")
		s.Require().NoError(os.WriteFile(tokenFile, []byte("  eyJ0b2tlbi5qd3Q  \n"), 0600))

		cfg := &TargetTokenExchangeConfig{
			ClientID:           "test-client",
			AuthStyle:          AuthStyleFederated,
			FederatedTokenFile: tokenFile,
		}

		data := url.Values{}
		header := http.Header{}
		err := injectClientAuth(cfg, data, header)
		s.Require().NoError(err)

		s.Equal("eyJ0b2tlbi5qd3Q", data.Get(FormKeyClientAssertion))
	})

	s.Run("returns error for missing token file", func() {
		cfg := &TargetTokenExchangeConfig{
			ClientID:           "test-client",
			AuthStyle:          AuthStyleFederated,
			FederatedTokenFile: "/nonexistent/token",
		}

		data := url.Values{}
		header := http.Header{}
		err := injectClientAuth(cfg, data, header)
		s.Error(err)
		s.Contains(err.Error(), "failed to read federated token file")
	})

	s.Run("returns error for empty token file", func() {
		tokenFile := filepath.Join(s.tempDir, "empty-token")
		s.Require().NoError(os.WriteFile(tokenFile, []byte(""), 0600))

		cfg := &TargetTokenExchangeConfig{
			ClientID:           "test-client",
			AuthStyle:          AuthStyleFederated,
			FederatedTokenFile: tokenFile,
		}

		data := url.Values{}
		header := http.Header{}
		err := injectClientAuth(cfg, data, header)
		s.Error(err)
		s.Contains(err.Error(), "is empty")
	})

	s.Run("returns error for whitespace-only token file", func() {
		tokenFile := filepath.Join(s.tempDir, "ws-token")
		s.Require().NoError(os.WriteFile(tokenFile, []byte("  \n\t  \n"), 0600))

		cfg := &TargetTokenExchangeConfig{
			ClientID:           "test-client",
			AuthStyle:          AuthStyleFederated,
			FederatedTokenFile: tokenFile,
		}

		data := url.Values{}
		header := http.Header{}
		err := injectClientAuth(cfg, data, header)
		s.Error(err)
		s.Contains(err.Error(), "is empty")
	})
}

func TestAssertion(t *testing.T) {
	suite.Run(t, new(AssertionTestSuite))
}
