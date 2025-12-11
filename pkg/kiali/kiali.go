package kiali

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type Kiali struct {
	bearerToken          string
	kialiURL             string
	kialiInsecure        bool
	certificateAuthority string
}

// NewKiali creates a new Kiali instance
func NewKiali(configProvider api.ExtendedConfigProvider, kubernetes *rest.Config) *Kiali {
	kiali := &Kiali{bearerToken: kubernetes.BearerToken}
	if cfg, ok := configProvider.GetToolsetConfig("kiali"); ok {
		if kc, ok := cfg.(*Config); ok && kc != nil {
			kiali.kialiURL = kc.Url
			kiali.kialiInsecure = kc.Insecure
			kiali.certificateAuthority = kc.CertificateAuthority
		}
	}
	return kiali
}

// validateAndGetURL validates the Kiali client configuration and returns the full URL
// by safely concatenating the base URL with the provided endpoint, avoiding duplicate
// or missing slashes regardless of trailing/leading slashes.
func (k *Kiali) validateAndGetURL(endpoint string) (string, error) {
	if k == nil || k.kialiURL == "" {
		return "", fmt.Errorf("kiali client not initialized")
	}
	baseStr := strings.TrimSpace(k.kialiURL)
	if baseStr == "" {
		return "", fmt.Errorf("kiali server URL not configured")
	}
	baseURL, err := url.Parse(baseStr)
	if err != nil {
		return "", fmt.Errorf("invalid kiali base URL: %w", err)
	}
	if endpoint == "" {
		return baseURL.String(), nil
	}
	// Parse the endpoint to extract path, query, and fragment
	endpoint = strings.TrimSpace(endpoint)
	endpointURL, err := url.Parse(endpoint)

	if err != nil {
		return "", fmt.Errorf("invalid endpoint path: %w", err)
	}
	// Reject absolute URLs - endpoint should be a relative path
	if endpointURL.Scheme != "" || endpointURL.Host != "" {
		return "", fmt.Errorf("endpoint must be a relative path, not an absolute URL")
	}
	resultURL, err := url.JoinPath(baseURL.String(), endpointURL.Path)
	if err != nil {
		return "", fmt.Errorf("failed to join kiali base URL with endpoint path: %w", err)
	}

	u, err := url.Parse(resultURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse joined URL: %w", err)
	}
	u.RawQuery = endpointURL.RawQuery
	u.Fragment = endpointURL.Fragment

	return u.String(), nil
}

func (k *Kiali) createHTTPClient() *http.Client {
	// Base TLS configuration, optionally extended with a custom CA
	tlsConfig := &tls.Config{
		InsecureSkipVerify: k.kialiInsecure,
	}

	// If a custom Certificate Authority is configured, load and add it
	if caValue := strings.TrimSpace(k.certificateAuthority); caValue != "" {
		// Read the certificate from file
		caPEM, err := os.ReadFile(caValue)
		if err != nil {
			klog.Errorf("failed to read CA certificate from file %s: %v; proceeding without custom CA", caValue, err)
			return &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: tlsConfig,
				},
			}
		}

		// Start with the host system pool when possible so we don't drop system roots
		var certPool *x509.CertPool
		if systemPool, err := x509.SystemCertPool(); err == nil && systemPool != nil {
			certPool = systemPool
		} else {
			certPool = x509.NewCertPool()
		}
		if ok := certPool.AppendCertsFromPEM(caPEM); ok {
			tlsConfig.RootCAs = certPool
		} else {
			klog.V(0).Infof("failed to append provided certificate authority; proceeding without custom CA")
		}
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
}

// CurrentAuthorizationHeader returns the Authorization header value that the
// Kiali client is currently configured to use (Bearer <token>), or empty
// if no bearer token is configured.
func (k *Kiali) authorizationHeader() string {
	if k == nil {
		return ""
	}
	token := strings.TrimSpace(k.bearerToken)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer ") {
		return token
	}
	return "Bearer " + token
}

// executeRequest executes an HTTP request (optionally with a body) and handles common error scenarios.
func (k *Kiali) executeRequest(ctx context.Context, method, endpoint, contentType string, body io.Reader) (string, error) {
	if method == "" {
		method = http.MethodGet
	}
	ApiCallURL, err := k.validateAndGetURL(endpoint)
	if err != nil {
		return "", err
	}
	klog.V(0).Infof("kiali API call: %s %s", method, ApiCallURL)
	req, err := http.NewRequestWithContext(ctx, method, ApiCallURL, body)
	if err != nil {
		return "", err
	}
	authHeader := k.authorizationHeader()
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	client := k.createHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(respBody) > 0 {
			return "", fmt.Errorf("kiali API error: %s", strings.TrimSpace(string(respBody)))
		}
		return "", fmt.Errorf("kiali API error: status %d", resp.StatusCode)
	}
	return string(respBody), nil
}
