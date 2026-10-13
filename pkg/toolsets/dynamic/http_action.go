package dynamic

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

const maxResponseBytes = 8 << 20

var pathPlaceholder = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// runHTTP performs the declared HTTP request. Requests to an in-cluster
// Service go through the API server's service proxy using the caller's REST
// config (so the access-control round tripper and the caller's identity
// apply). Requests to a plain URL carry the caller's bearer token, exactly as
// the Kiali and NetObserv toolsets do.
func (t *compiledTool) runHTTP(action *compiledAction, params api.ToolHandlerParams, vars evalVars) (any, error) {
	a := action.http
	args := vars.args
	restConfig := params.RESTConfig()

	var client *http.Client
	var base string
	if a.Service != nil {
		var err error
		if client, err = rest.HTTPClientFor(restConfig); err != nil {
			return nil, fmt.Errorf("failed to create HTTP client: %w", err)
		}
		host := restConfig.Host
		if !strings.Contains(host, "://") {
			host = "https://" + host
		}
		base = strings.TrimRight(host, "/") + "/api/v1/namespaces/" + url.PathEscape(a.Service.Namespace) +
			"/services/" + url.PathEscape(proxyServiceName(a.Service)) + "/proxy"
	} else {
		tlsConfig := &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: a.InsecureSkipTLSVerify, //nolint:gosec // opt-in per Tool, mirrors the Kiali/NetObserv toolset configs
		}
		if a.CertificateAuthority != "" {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM([]byte(a.CertificateAuthority)) {
				return nil, fmt.Errorf("certificateAuthority does not contain a valid PEM certificate")
			}
			tlsConfig.RootCAs = pool
		}
		client = &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}
		base = strings.TrimRight(a.URL, "/")
	}

	path, err := expandPath(a.Path, args)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return nil, fmt.Errorf("invalid request URL: %w", err)
	}
	query := u.Query()
	values, err := t.resolveQuery(a.Query, action.queryExpr, vars)
	if err != nil {
		return nil, err
	}
	for key, value := range values {
		query.Set(key, value)
	}
	u.RawQuery = query.Encode()

	method := strings.ToUpper(a.Method)
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if a.Body != nil {
		raw, err := t.resolveBody(a.Body, action.bodyExpr, vars)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(params, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range a.Headers {
		req.Header.Set(key, value)
	}
	if a.Service == nil {
		if token := bearerToken(restConfig); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s %s returned HTTP %d: %s", method, u.Redacted(), resp.StatusCode, truncate(string(raw), 512))
	}
	return decodeResponse(raw), nil
}

func proxyServiceName(service *ServiceRef) string {
	name := service.Name
	if service.Scheme != "" {
		name = service.Scheme + ":" + name
	}
	if service.Port != "" {
		name += ":" + service.Port
	}
	return name
}

// expandPath replaces {argument} placeholders with the argument value,
// URL-escaped as a single path segment. Every placeholder must be provided.
func expandPath(template string, args map[string]any) (string, error) {
	var missing []string
	expanded := pathPlaceholder.ReplaceAllStringFunc(template, func(match string) string {
		key := match[1 : len(match)-1]
		value := argString(args, key)
		if value == "" {
			missing = append(missing, key)
		}
		return url.PathEscape(value)
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("%s parameter required", strings.Join(missing, ", "))
	}
	return expanded, nil
}

func bearerToken(restConfig *rest.Config) string {
	if restConfig == nil {
		return ""
	}
	if token := strings.TrimSpace(restConfig.BearerToken); token != "" {
		return token
	}
	if restConfig.BearerTokenFile != "" {
		if raw, err := os.ReadFile(restConfig.BearerTokenFile); err == nil {
			return strings.TrimSpace(string(raw))
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
