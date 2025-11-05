package kiali

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"k8s.io/klog/v2"
	"net/http"
	"net/url"
	"strings"
)

type Kiali struct {
	manager *Manager
}

func (m *Manager) GetKiali() *Kiali {
	return &Kiali{manager: m}
}

func (k *Kiali) GetKiali() *Kiali {
	return k
}

// validateAndGetURL validates the Kiali client configuration and returns the full URL
// by safely concatenating the base URL with the provided endpoint, avoiding duplicate
// or missing slashes regardless of trailing/leading slashes.
func (k *Kiali) validateAndGetURL(endpoint string) (string, error) {
	if k == nil || k.manager == nil || k.manager.KialiURL == "" {
		return "", fmt.Errorf("kiali client not initialized")
	}
	baseStr := strings.TrimSpace(k.manager.KialiURL)
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
	ref, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint path: %w", err)
	}
	return baseURL.ResolveReference(ref).String(), nil
}

func (k *Kiali) createHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: k.manager.KialiInsecure,
			},
		},
	}
}

// CurrentAuthorizationHeader returns the Authorization header value that the
// Kiali client is currently configured to use (Bearer <token>), or empty
// if no bearer token is configured.
func (k *Kiali) authorizationHeader() string {
	if k == nil || k.manager == nil {
		return ""
	}
	token := strings.TrimSpace(k.manager.BearerToken)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer ") {
		return token
	}
	return "Bearer " + token
}

// executeRequest executes an HTTP request and handles common error scenarios.
func (k *Kiali) executeRequest(ctx context.Context, endpoint string) (string, error) {
	ApiCallURL, err := k.validateAndGetURL(endpoint)
	if err != nil {
		return "", err
	}

	klog.V(0).Infof("Kiali Call URL: %s", ApiCallURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ApiCallURL, nil)
	if err != nil {
		return "", err
	}
	authHeader := k.authorizationHeader()
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	client := k.createHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(body) > 0 {
			return "", fmt.Errorf("kiali API error: %s", strings.TrimSpace(string(body)))
		}
		return "", fmt.Errorf("kiali API error: status %d", resp.StatusCode)
	}
	return string(body), nil
}
