package dynamic

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

const maxResponseBytes = 8 << 20

var pathPlaceholder = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// runHTTP performs the declared HTTP request. Requests to an in-cluster
// Service go through the API server's service proxy using the caller's REST
// config (so the access-control round tripper and the caller's identity
// apply). Requests to a plain URL carry the caller's bearer token, exactly as
// the Kiali and NetObserv toolsets do.
func (t *compiledTool) runHTTP(action *compiledAction, env runEnv, vars evalVars) (any, error) {
	a := action.http
	restConfig := env.client.RESTConfig()

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
		client = &http.Client{Transport: action.transport}
		if env.requireTLS != nil {
			// Layer 2 of require_tls: refuse plain HTTP at request time, as the
			// in-tree HTTP toolsets did.
			client = config.NewTLSEnforcingClient(client, env.requireTLS)
		}
		base = a.URL
		if action.urlExpr != nil {
			value, err := action.urlExpr.eval(vars)
			if err != nil {
				return nil, err
			}
			if base = scalarString(value); !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
				return nil, fmt.Errorf("urlExpression produced %q, which is not an absolute http(s) URL", base)
			}
		}
		base = strings.TrimRight(base, "/")
		if allowed := env.allowedHosts; allowed != nil && a.Service == nil {
			// The url or urlExpression host is operator policy, unlike the
			// service proxy, which carries no user token and is bounded by
			// services/proxy RBAC.
			host := hostOf(base)
			if !httpHostAllowed(allowed(), host) {
				return nil, fmt.Errorf("host %q is not in the server's allowed_http_hosts", host)
			}
		}
	}

	path, err := t.resolvePath(a.Path, vars)
	if err != nil {
		return nil, err
	}
	if err := checkRequestPath(path); err != nil {
		return nil, err
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return nil, fmt.Errorf("invalid request URL: %w", err)
	}
	if err := checkSameOrigin(base, u); err != nil {
		return nil, err
	}
	query := u.Query()
	values, err := t.resolveQuery(a.Query, action.queryExpr, vars)
	if err != nil {
		return nil, err
	}
	for key, list := range values {
		query.Del(key)
		for _, value := range list {
			query.Add(key, value)
		}
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
	req, err := http.NewRequestWithContext(env.ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, ref := range a.Headers {
		value, err := t.resolveRef(&ref, vars)
		if err != nil {
			return nil, fmt.Errorf("header %s: %w", key, err)
		}
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	if a.Service == nil {
		credentials := restConfig
		if env.serverConfig != nil {
			// The family acts as the server: its own credentials, never the
			// caller's token (the former auth_mode kubeconfig).
			if credentials, err = env.serverConfig(); err != nil {
				return nil, err
			}
		}
		if token := bearerToken(credentials); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, readErr := readResponseBody(resp.Body)
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s %s returned HTTP %d: %s", method, u.Redacted(), resp.StatusCode, truncate(string(raw), 512))
	}
	if readErr != nil {
		return nil, readErr
	}
	contentType, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	return decodeResponseAs(raw, a.Response, strings.TrimSpace(contentType), a.MIMEType, a.Redact), nil
}

// newHTTPTransport builds the transport of a url-mode action from the
// system defaults (proxy from the environment, idle connection reaping,
// handshake timeouts) and the action's TLS settings.
func newHTTPTransport(a *HTTPAction) (*http.Transport, error) {
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
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return transport, nil
}

// readResponseBody reads at most maxResponseBytes. A larger body is an
// error (returned together with what was read, for error messages) rather
// than a silently truncated, no longer parseable document.
func readResponseBody(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		return raw, fmt.Errorf("failed to read response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return raw[:maxResponseBytes], fmt.Errorf("response exceeds the maximum size of %d bytes", maxResponseBytes)
	}
	return raw, nil
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

// stepsHTTPURLs returns the http urls a step list declares literally.
func stepsHTTPURLs(steps []Step) []string {
	var urls []string
	for i := range steps {
		if action := steps[i].HTTP; action != nil && action.URL != "" {
			urls = append(urls, action.URL)
		}
	}
	return urls
}

// fixedHTTPURLs returns the http urls a Tool declares literally: the
// top-level action's, and those of its steps and of its variants' options.
// A urlExpression is not among them; its host is checked at call time.
func fixedHTTPURLs(spec ToolSpec) []string {
	urls := stepsHTTPURLs(spec.Steps)
	if action := spec.HTTP; action != nil && action.URL != "" {
		urls = append(urls, action.URL)
	}
	if spec.Variants != nil {
		for _, option := range spec.Variants.Options {
			if action := option.HTTP; action != nil && action.URL != "" {
				urls = append(urls, action.URL)
			}
			urls = append(urls, stepsHTTPURLs(option.Steps)...)
		}
	}
	return urls
}

// hostOf returns the host of an absolute http(s) URL, lower-cased, without
// the port.
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// httpHostAllowed reports whether host falls within allowed_http_hosts:
// an empty list allows every host (the trust model is then whoever can
// create Tool resources); an entry is an exact host or a "*.domain"
// wildcard matching subdomains (the bare domain must be listed separately);
// a port may be given and is ignored, since it is not a security boundary.
func httpHostAllowed(allowed []string, host string) bool {
	if len(allowed) == 0 {
		return true
	}
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	for _, entry := range allowed {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "*.") {
			domain := entry[1:]
			// A port may be given; match on the domain alone.
			if h, _, err := net.SplitHostPort(domain); err == nil {
				domain = h
			}
			if strings.HasSuffix(host, domain) {
				return true
			}
			continue
		}
		// A port may be given; match on the host alone.
		if h, _, err := net.SplitHostPort(entry); err == nil {
			entry = h
		}
		if host == entry {
			return true
		}
	}
	return false
}

// checkRequestPath refuses a resolved path that could change where the
// request goes. An argument or expression path is appended verbatim, so a
// value such as "@evil.example/x" would otherwise turn the base host into
// userinfo and send the caller's token elsewhere, and ".." would walk out of
// the service proxy prefix onto arbitrary API paths. The path must be empty
// or start with a single "/", and must carry no ".." segment, query or
// fragment (those belong in `query`).
func checkRequestPath(path string) error {
	if path == "" {
		return nil
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return fmt.Errorf("request path %q must start with a single /", path)
	}
	if strings.ContainsAny(path, "?#") {
		return fmt.Errorf("request path %q must not contain a query or fragment", path)
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." || strings.EqualFold(segment, "%2e%2e") || strings.EqualFold(segment, ".%2e") || strings.EqualFold(segment, "%2e.") {
			return fmt.Errorf("request path %q must not contain a .. segment", path)
		}
	}
	return nil
}

// checkSameOrigin is the second guard behind checkRequestPath: whatever the
// path resolved to, the parsed request must still target the base URL's
// scheme and host, and its decoded path must not escape through "..".
func checkSameOrigin(base string, u *url.URL) error {
	origin, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	if u.Scheme != origin.Scheme || u.Host != origin.Host || u.User != nil {
		return fmt.Errorf("request path changed the request origin from %s to %s", origin.Redacted(), u.Redacted())
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == ".." {
			return fmt.Errorf("request path %q must not contain a .. segment", u.Path)
		}
	}
	return nil
}

// resolvePath returns the request path: a plain string has its {argument}
// placeholders replaced with the URL-escaped argument value; an argument or
// expression reference is appended verbatim, so the definition escapes what
// it interpolates.
func (t *compiledTool) resolvePath(path *Ref, vars evalVars) (string, error) {
	switch {
	case path == nil:
		return "", nil
	case path.Argument != "" || path.Expression != "":
		return t.resolveRef(path, vars)
	default:
		return expandPlaceholders(path.Value, vars.args, url.PathEscape)
	}
}

// expandPlaceholders replaces {argument} placeholders with the argument
// value passed through escape, and reports the arguments that are missing.
func expandPlaceholders(template string, args map[string]any, escape func(string) string) (string, error) {
	var missing []string
	expanded := pathPlaceholder.ReplaceAllStringFunc(template, func(match string) string {
		key := match[1 : len(match)-1]
		value := argString(args, key)
		if value == "" {
			missing = append(missing, key)
		}
		return escape(value)
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
