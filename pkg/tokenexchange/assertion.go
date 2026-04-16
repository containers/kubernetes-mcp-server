package tokenexchange

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

const (
	// ClientAssertionType is the OAuth client assertion type for JWT bearer (RFC 7523)
	ClientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

	// FormKeyClientAssertion is the form parameter name for the JWT assertion
	FormKeyClientAssertion = "client_assertion"

	// FormKeyClientAssertionType is the form parameter name for the assertion type
	FormKeyClientAssertionType = "client_assertion_type"

	// DefaultAssertionLifetime is the default validity period for assertions
	DefaultAssertionLifetime = 5 * time.Minute

	// AssertionRefreshMargin is how early to refresh before expiry
	AssertionRefreshMargin = 30 * time.Second
)

// loadCertificateAndKey reads the certificate and private key from PEM files
func loadCertificateAndKey(certFile, keyFile string) (*x509.Certificate, crypto.Signer, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read certificate file %q: %w", certFile, err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode PEM block from certificate file %q", certFile)
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate from %q: %w", certFile, err)
	}

	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read private key file %q: %w", keyFile, err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode PEM block from private key file %q", keyFile)
	}

	var privateKey crypto.Signer

	// Try PKCS8 first (modern format), then PKCS1
	if key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		if signer, ok := key.(crypto.Signer); ok {
			privateKey = signer
		} else {
			return nil, nil, fmt.Errorf("private key from %q does not implement crypto.Signer", keyFile)
		}
	} else if key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err == nil {
		privateKey = key
	}

	if privateKey == nil {
		return nil, nil, fmt.Errorf("failed to parse private key from %q (tried PKCS#8 and PKCS#1 formats)", keyFile)
	}

	if _, ok := privateKey.(*rsa.PrivateKey); !ok {
		return nil, nil, fmt.Errorf("unsupported key type %T from %q: only RSA keys are currently supported for JWT client assertions", privateKey, keyFile)
	}

	return cert, privateKey, nil
}

// computeX5TS256 computes the x5t#S256 (X.509 certificate SHA-256 thumbprint) header value.
// We use SHA-256 instead of SHA-1 (x5t) because SHA-1 is cryptographically weak.
// Entra ID and other major OIDC providers support x5t#S256 (RFC 7515 Section 4.1.8).
func computeX5TS256(cert *x509.Certificate) string {
	thumbprint := sha256.Sum256(cert.Raw)
	return base64.RawURLEncoding.EncodeToString(thumbprint[:])
}

// getSignatureAlgorithm determines the jose.SignatureAlgorithm based on key type
func getSignatureAlgorithm(key crypto.Signer) (jose.SignatureAlgorithm, error) {
	switch key.(type) {
	case *rsa.PrivateKey:
		return jose.RS256, nil
	default:
		return "", fmt.Errorf("unsupported key type: %T (only RSA keys are currently supported)", key)
	}
}

// BuildClientAssertion creates a signed JWT assertion for client authentication
func BuildClientAssertion(clientID, tokenURL, certFile, keyFile string, lifetime time.Duration) (string, time.Time, error) {
	if lifetime == 0 {
		lifetime = DefaultAssertionLifetime
	}

	cert, privateKey, err := loadCertificateAndKey(certFile, keyFile)
	if err != nil {
		return "", time.Time{}, err
	}

	algorithm, err := getSignatureAlgorithm(privateKey)
	if err != nil {
		return "", time.Time{}, err
	}

	now := time.Now()
	expiry := now.Add(lifetime)

	claims := jwt.Claims{
		Issuer:    clientID,
		Subject:   clientID,
		Audience:  jwt.Audience{tokenURL},
		ID:        uuid.New().String(),
		NotBefore: jwt.NewNumericDate(now),
		Expiry:    jwt.NewNumericDate(expiry),
	}

	signerOpts := jose.SignerOptions{}
	signerOpts.WithHeader("x5t#S256", computeX5TS256(cert))
	signerOpts.WithType("JWT")

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: algorithm, Key: privateKey},
		&signerOpts,
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create JWT signer: %w", err)
	}

	signedJWT, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign JWT assertion: %w", err)
	}

	klog.V(4).Infof("Built JWT client assertion: issuer=%s, audience=%s, jti=%s, x5t=%s, expires=%s",
		clientID, tokenURL, claims.ID, computeX5TS256(cert), expiry.Format(time.RFC3339))

	return signedJWT, expiry, nil
}

// GetOrBuildAssertion returns a cached assertion or builds a new one
func (c *TargetTokenExchangeConfig) GetOrBuildAssertion() (string, error) {
	c.assertionMutex.Lock()
	defer c.assertionMutex.Unlock()

	// Check if cached assertion is still valid (with margin)
	if c.cachedAssertion != "" && time.Now().Add(AssertionRefreshMargin).Before(c.cachedAssertionExpiry) {
		klog.V(4).Infof("Using cached JWT client assertion, expires=%s", c.cachedAssertionExpiry.Format(time.RFC3339))
		return c.cachedAssertion, nil
	}

	klog.V(4).Infof("Building new JWT client assertion: client_id=%s, token_url=%s, cert_file=%s",
		c.ClientID, c.TokenURL, c.ClientCertFile)

	assertion, expiry, err := BuildClientAssertion(
		c.ClientID,
		c.TokenURL,
		c.ClientCertFile,
		c.ClientKeyFile,
		c.AssertionLifetime,
	)
	if err != nil {
		return "", err
	}

	c.cachedAssertion = assertion
	c.cachedAssertionExpiry = expiry

	return assertion, nil
}
