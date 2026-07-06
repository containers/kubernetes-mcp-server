package netobserv

import (
	"context"
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
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/tlsutil"
	"k8s.io/client-go/rest"
)

// NetObserv is an HTTP client for the NetObserv console plugin backend API.
type NetObserv struct {
	bearerToken          string
	bearerTokenFile      string
	pluginURL            string
	insecure             bool
	certificateAuthority string
	tlsMinVersion        string
	tlsCipherSuites      []string
	requireTLS           func() bool
}

// NewNetObserv creates a client using toolset config, cluster detection, and the Kubernetes REST config.
func NewNetObserv(ctx context.Context, configProvider api.BaseConfig, k8s api.KubernetesClient, provider api.FilteringProvider) *NetObserv {
	var restConfig *rest.Config
	if k8s != nil {
		restConfig = k8s.RESTConfig()
	}
	client := &NetObserv{
		bearerToken:     "",
		tlsMinVersion:   configProvider.GetTLSMinVersionConfig(),
		tlsCipherSuites: configProvider.GetTLSCipherSuitesConfig(),
		requireTLS:      configProvider.IsRequireTLS,
	}
	if restConfig != nil {
		client.bearerToken = strings.TrimSpace(restConfig.BearerToken)
		client.bearerTokenFile = strings.TrimSpace(restConfig.BearerTokenFile)
	}
	var shared *Config
	if cfg, ok := configProvider.GetToolsetConfig("netobserv"); ok {
		if parsed, ok := cfg.(*Config); ok && parsed != nil {
			shared = parsed
		}
	}
	resolved := Config{}
	if shared != nil {
		resolved = *shared
	}
	isOpenShift := isOpenShiftFromProvider(ctx, provider)
	resolved.applyDefaults(ctx, isOpenShift)
	client.pluginURL = resolved.ResolvedURL(isOpenShift)
	client.insecure = resolved.Insecure
	client.certificateAuthority = resolved.CertificateAuthority
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

var errRedirectsNotAllowed = errors.New("redirects are not allowed for netobserv API requests")

func (n *NetObserv) createHTTPClient(ctx context.Context) (*http.Client, error) {
	logger := klogutil.FromContext(ctx)
	var tlsOpts []tlsutil.TLSConfigOption

	if n.insecure {
		tlsOpts = append(tlsOpts, tlsutil.WithInsecureSkipVerify(true))
	}

	if caValue := strings.TrimSpace(n.certificateAuthority); caValue != "" {
		caPEM, err := os.ReadFile(caValue)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate authority %q: %w", caValue, err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse certificate authority %q", caValue)
		}
		tlsOpts = append(tlsOpts, tlsutil.WithRootCAs(certPool))
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(n.tlsMinVersion, n.tlsCipherSuites, tlsOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       tlsConfig,
			ResponseHeaderTimeout: DefaultPluginHTTPTimeout,
		},
		Timeout: DefaultPluginHTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errRedirectsNotAllowed
		},
	}
	if n.insecure {
		klogutil.LogInfo(logger.V(1), "NetObserv plugin TLS verification disabled")
	}
	return n.wrapWithTLSEnforcement(client), nil
}

func (n *NetObserv) wrapWithTLSEnforcement(client *http.Client) *http.Client {
	if n.requireTLS == nil {
		return client
	}
	return config.NewTLSEnforcingClient(client, n.requireTLS)
}

func (n *NetObserv) authorizationHeader(ctx context.Context) string {
	if n == nil {
		return ""
	}
	token := strings.TrimSpace(n.bearerToken)
	if token == "" && n.bearerTokenFile != "" {
		data, err := os.ReadFile(n.bearerTokenFile)
		if err != nil {
			klogutil.FromContext(ctx).Error(err, "failed to read bearer token file", "path", n.bearerTokenFile)
			return ""
		}
		token = strings.TrimSpace(string(data))
	}
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer ") {
		return token
	}
	return "Bearer " + token
}

const maxJSONResponseBodySize = 4 << 20 // 4 MiB

// ExecuteGet performs a GET request against the plugin API with query parameters derived from arguments.
func (n *NetObserv) ExecuteGet(ctx context.Context, endpoint string, arguments map[string]any) (string, error) {
	response, err := n.executeGet(ctx, endpoint, arguments, "application/json", maxJSONResponseBodySize, false)
	if err != nil {
		return "", err
	}
	return response.Body, nil
}

func (n *NetObserv) executeGet(ctx context.Context, endpoint string, arguments map[string]any, accept string, maxBodySize int64, truncate bool) (GetResponse, error) {
	requestURL, err := n.validateAndGetURL(endpoint)
	if err != nil {
		return GetResponse{}, err
	}
	return n.executeGetAbsolute(ctx, requestURL, arguments, accept, maxBodySize, truncate)
}

func (n *NetObserv) executeGetAbsolute(ctx context.Context, requestURL string, arguments map[string]any, accept string, maxBodySize int64, truncate bool) (GetResponse, error) {
	u, err := url.Parse(requestURL)
	if err != nil {
		return GetResponse{}, fmt.Errorf("failed to parse request URL: %w", err)
	}
	query := ArgumentsToValues(PrepareQueryArguments(arguments))
	u.RawQuery = query.Encode()
	klogutil.LogInfo(klogutil.FromContext(ctx).V(0), "netobserv API call", klogutil.Field("url", u.Redacted()))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return GetResponse{}, err
	}
	if authHeader := n.authorizationHeader(ctx); authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("X-Kubernetes-MCP-Server", "true")
	client, err := n.createHTTPClient(ctx)
	if err != nil {
		return GetResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return GetResponse{}, fmt.Errorf("netobserv API call canceled or timed out waiting for %s: %w", u.Redacted(), err)
		}
		return GetResponse{}, fmt.Errorf("netobserv API call to %s failed: %w", u.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return GetResponse{}, fmt.Errorf("failed to read response body: %w", err)
	}
	truncated := false
	if int64(len(respBody)) > maxBodySize {
		if !truncate {
			return GetResponse{}, fmt.Errorf("netobserv API response exceeded maximum allowed size of %d bytes", maxBodySize)
		}
		respBody = respBody[:maxBodySize]
		truncated = true
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(respBody) > 0 {
			return GetResponse{}, fmt.Errorf("netobserv API error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		return GetResponse{}, fmt.Errorf("netobserv API error: status %d", resp.StatusCode)
	}
	return GetResponse{Body: string(respBody), Truncated: truncated}, nil
}
