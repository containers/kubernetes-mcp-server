package kiali

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

const (
	statusPath         = "/api/status"
	statusProbeTimeout = 3 * time.Second
)

// statusResponse is the subset of Kiali GET /api/status used for reachability checks.
type statusResponse struct {
	Status map[string]any `json:"status"`
}

// probeStatusURL GETs {baseURL}/api/status and returns true when the response
// is HTTP 2xx and contains a non-empty status object (Kiali is reachable).
func probeStatusURL(ctx context.Context, baseURL string, cfg *Config, bearerToken string) bool {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return false
	}
	statusURL := baseURL + statusPath

	probeCfg := cfg
	if probeCfg == nil {
		probeCfg = &Config{Url: baseURL}
	} else if strings.TrimSpace(probeCfg.Url) == "" {
		// Use a shallow copy so TLS options are preserved without mutating Url.
		cp := *probeCfg
		cp.Url = baseURL
		probeCfg = &cp
	}

	k := &Kiali{
		bearerToken:          bearerToken,
		kialiURL:             probeCfg.Url,
		kialiInsecure:        probeCfg.Insecure,
		certificateAuthority: probeCfg.CertificateAuthority,
	}

	probeCtx, cancel := context.WithTimeout(ctx, statusProbeTimeout)
	defer cancel()

	client, err := k.createHTTPClient(probeCtx)
	if err != nil {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe: failed to create HTTP client", "error", err)
		return false
	}
	client.Timeout = statusProbeTimeout

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, statusURL, nil)
	if err != nil {
		return false
	}
	if auth := k.authorizationHeader(); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := client.Do(req)
	if err != nil {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe failed", "url", statusURL, "error", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		return false
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe non-success",
			"url", statusURL, "status_code", resp.StatusCode)
		return false
	}

	var parsed statusResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe: invalid JSON", "error", err)
		return false
	}
	if len(parsed.Status) == 0 {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe: missing status object", "url", statusURL)
		return false
	}
	klogutil.FromContext(ctx).V(2).Info("kiali status probe succeeded", "url", statusURL)
	return true
}
