package kiali

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

const (
	statusPath         = "/api/status"
	statusProbeTimeout = time.Second
)

// statusResponse is the subset of Kiali GET /api/status used for reachability checks.
type statusResponse struct {
	Status map[string]any `json:"status"`
}

// probeStatusURL GETs {baseURL}/api/status and returns true when Kiali is
// reachable, false when it is unavailable, or nil only for temporary DNS
// resolution failures (fail open).
func probeStatusURL(ctx context.Context, baseURL string, cfg *Config, bearerToken string) *bool {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ptr(false)
	}
	statusURL := baseURL + statusPath

	probeCfg := cfg
	if probeCfg == nil {
		probeCfg = &Config{Url: baseURL}
	} else if strings.TrimSpace(probeCfg.Url) == "" {
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
		return ptr(false)
	}

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, statusURL, nil)
	if err != nil {
		return ptr(false)
	}
	if auth := k.authorizationHeader(); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := client.Do(req)
	if err != nil {
		if isTemporaryDNSProbeError(err) {
			klogutil.FromContext(ctx).V(2).Info("kiali status probe: temporary DNS failure; assuming Kiali is available",
				"url", statusURL, "error", err)
			return nil
		}
		klogutil.FromContext(ctx).V(2).Info("kiali status probe failed", "url", statusURL, "error", err)
		return ptr(false)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		return ptr(false)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe non-success",
			"url", statusURL, "status_code", resp.StatusCode)
		return ptr(false)
	}

	var parsed statusResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe: invalid JSON", "error", err)
		return ptr(false)
	}
	if len(parsed.Status) == 0 {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe: missing status object", "url", statusURL)
		return ptr(false)
	}
	klogutil.FromContext(ctx).V(2).Info("kiali status probe succeeded", "url", statusURL)
	return ptr(true)
}

func isTemporaryDNSProbeError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsTemporary
}

func ptr(v bool) *bool {
	return &v
}
