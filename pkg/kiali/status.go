package kiali

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

const (
	statusPath         = "/api/status"
	statusProbeTimeout = time.Second
)

// statusResponse is the subset of Kiali GET /api/status used for reachability checks.
type statusResponse struct {
	Status map[string]any `json:"status"`
}

// Available reports whether the client has a configured URL and Kiali responds to GET /api/status,
// using the same authorization, TLS client, and URL rules as ExecuteRequest. Returns true when
// Kiali is reachable, or when the probe hits a temporary DNS resolution failure (fail open).
func (k *Kiali) Available(ctx context.Context) bool {
	if k == nil || strings.TrimSpace(k.kialiURL) == "" {
		return false
	}
	if k.probeStatus(ctx) {
		return true
	}
	klogutil.FromContext(ctx).V(1).Info("configured Kiali URL failed /api/status probe; disabling Kiali tools",
		"url", k.kialiURL)
	return false
}

func (k *Kiali) probeStatus(ctx context.Context) bool {
	limit, err := k.backendLimit()
	if err != nil {
		return false
	}
	statusURL, err := k.validateAndGetURL(statusPath)
	if err != nil {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe: invalid URL", "error", err)
		return false
	}

	probeCtx, cancel := context.WithTimeout(ctx, statusProbeTimeout)
	defer cancel()

	client, err := k.createHTTPClient(probeCtx)
	if err != nil {
		klogutil.FromContext(ctx).V(2).Info("kiali status probe: failed to create HTTP client", "error", err)
		return false
	}

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, statusURL, nil)
	if err != nil {
		return false
	}
	if auth := k.authorizationHeader(); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := client.Do(req)
	if err != nil {
		if isTemporaryDNSProbeError(err) {
			klogutil.FromContext(ctx).V(2).Info("kiali status probe: temporary DNS failure; assuming Kiali is available",
				"url", statusURL, "error", err)
			return true
		}
		klogutil.FromContext(ctx).V(2).Info("kiali status probe failed", "url", statusURL, "error", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := kubernetes.ReadCapped(resp.Body, limit)
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

func isTemporaryDNSProbeError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsTemporary
}
