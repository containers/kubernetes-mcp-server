package netobserv

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// NetObserv is an HTTP client for the NetObserv console plugin backend API.
type NetObserv struct {
	bearerToken          string
	pluginURL            string
	insecure             bool
	certificateAuthority string
	requireTLS           func() bool
}

// NewNetObserv creates a client using toolset config, cluster detection, and the Kubernetes REST config.
func NewNetObserv(configProvider api.BaseConfig, k8s api.KubernetesClient) *NetObserv {
	var restConfig *rest.Config
	if k8s != nil {
		restConfig = k8s.RESTConfig()
	}
	client := &NetObserv{
		bearerToken: "",
		requireTLS:  configProvider.IsRequireTLS,
	}
	if restConfig != nil {
		client.bearerToken = restConfig.BearerToken
	}
	var nc *Config
	if cfg, ok := configProvider.GetToolsetConfig("netobserv"); ok {
		if parsed, ok := cfg.(*Config); ok && parsed != nil {
			nc = parsed
		}
	}
	if nc == nil {
		nc = &Config{}
	}
	isOpenShift := clusterIsOpenShift(k8s)
	requireTLS := configProvider.IsRequireTLS()
	nc.applyDefaults(requireTLS, isOpenShift)
	client.pluginURL = nc.ResolvedURL(isOpenShift)
	client.insecure = nc.Insecure
	client.certificateAuthority = nc.CertificateAuthority
	return client
}

func (n *NetObserv) validateAndGetURL(endpoint string) (string, error) {
	if n == nil || n.pluginURL == "" {
		return "", fmt.Errorf("netobserv client not initialized")
	}
	baseStr := strings.TrimSpace(n.pluginURL)
	if baseStr == "" {
		return "", fmt.Errorf("netobserv plugin URL not configured")
	}
	baseURL, err := url.Parse(baseStr)
	if err != nil {
		return "", fmt.Errorf("invalid netobserv base URL: %w", err)
	}
	if endpoint == "" {
		return baseURL.String(), nil
	}
	endpoint = strings.TrimSpace(endpoint)
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint path: %w", err)
	}
	if endpointURL.Scheme != "" || endpointURL.Host != "" {
		return "", fmt.Errorf("endpoint must be a relative path, not an absolute URL")
	}
	resultURL, err := url.JoinPath(baseURL.String(), endpointURL.Path)
	if err != nil {
		return "", fmt.Errorf("failed to join netobserv base URL with endpoint path: %w", err)
	}
	u, err := url.Parse(resultURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse joined URL: %w", err)
	}
	u.RawQuery = endpointURL.RawQuery
	u.Fragment = endpointURL.Fragment
	return u.String(), nil
}

func (n *NetObserv) createHTTPClient() *http.Client {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: n.insecure,
	}
	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		ResponseHeaderTimeout: DefaultPluginHTTPTimeout,
	}
	if caValue := strings.TrimSpace(n.certificateAuthority); caValue != "" {
		caPEM, err := os.ReadFile(caValue)
		if err != nil {
			klog.Errorf("failed to read CA certificate from file %s: %v; proceeding without custom CA", caValue, err)
			return n.wrapWithTLSEnforcement(&http.Client{
				Transport: transport,
				Timeout:   DefaultPluginHTTPTimeout,
			})
		}
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
	return n.wrapWithTLSEnforcement(&http.Client{
		Transport: transport,
		Timeout:   DefaultPluginHTTPTimeout,
	})
}

func (n *NetObserv) wrapWithTLSEnforcement(client *http.Client) *http.Client {
	if n.requireTLS == nil {
		return client
	}
	return config.NewTLSEnforcingClient(client, n.requireTLS)
}

func (n *NetObserv) authorizationHeader() string {
	if n == nil {
		return ""
	}
	token := strings.TrimSpace(n.bearerToken)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer ") {
		return token
	}
	return "Bearer " + token
}

const maxResponseBodySize = 512 << 10 // 512 KiB

// ExecuteGet performs a GET request against the plugin API with query parameters derived from arguments.
func (n *NetObserv) ExecuteGet(ctx context.Context, endpoint string, arguments map[string]any) (string, error) {
	return n.executeGet(ctx, endpoint, arguments, "application/json")
}

func (n *NetObserv) executeGet(ctx context.Context, endpoint string, arguments map[string]any, accept string) (string, error) {
	requestURL, err := n.validateAndGetURL(endpoint)
	if err != nil {
		return "", err
	}
	return n.executeGetAbsolute(ctx, requestURL, arguments, accept)
}

func (n *NetObserv) executeGetAbsolute(ctx context.Context, requestURL string, arguments map[string]any, accept string) (string, error) {
	u, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse request URL: %w", err)
	}
	query := ArgumentsToValues(arguments)
	u.RawQuery = query.Encode()
	klog.V(0).Infof("netobserv API call: %s", u.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	if authHeader := n.authorizationHeader(); authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("X-Kubernetes-MCP-Server", "true")
	client := n.createHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("netobserv API call canceled or timed out waiting for %s: %w", u.Redacted(), err)
		}
		return "", fmt.Errorf("netobserv API call to %s failed: %w", u.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if int64(len(respBody)) > maxResponseBodySize {
		return "", fmt.Errorf("netobserv API response exceeded maximum allowed size of %d bytes", maxResponseBodySize)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(respBody) > 0 {
			return "", fmt.Errorf("netobserv API error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		return "", fmt.Errorf("netobserv API error: status %d", resp.StatusCode)
	}
	return string(respBody), nil
}
