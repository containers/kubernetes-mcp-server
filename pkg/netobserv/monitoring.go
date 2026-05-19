package netobserv

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	pluginPrometheusRulesPath      = "/api/prometheus/api/v1/rules"
	pluginAlertmanagerSilencesPath = "/api/alertmanager/api/v2/silences"
	prometheusRulesPath            = "/api/v1/rules"
	alertmanagerSilencesPath       = "/api/v2/silences"
)

// ExecuteGetAlertRules lists Prometheus rules via the plugin proxy, falling back to the configured
// Prometheus URL when the plugin does not expose the route (typical on OpenShift Console deployments).
func (n *NetObserv) ExecuteGetAlertRules(ctx context.Context, arguments map[string]any) (string, error) {
	return n.executeGetWithMonitoringFallback(ctx, pluginPrometheusRulesPath, n.prometheusURL, prometheusRulesPath, arguments)
}

// ExecuteGetAlertSilences lists Alertmanager silences via the plugin proxy, falling back to the
// configured Alertmanager URL when the plugin does not expose the route.
func (n *NetObserv) ExecuteGetAlertSilences(ctx context.Context, arguments map[string]any) (string, error) {
	return n.executeGetWithMonitoringFallback(ctx, pluginAlertmanagerSilencesPath, n.alertmanagerURL, alertmanagerSilencesPath, arguments)
}

func (n *NetObserv) executeGetWithMonitoringFallback(
	ctx context.Context,
	pluginEndpoint, directBase, directPath string,
	arguments map[string]any,
) (string, error) {
	content, err := n.ExecuteGet(ctx, pluginEndpoint, arguments)
	if err == nil {
		return content, nil
	}
	directBase = strings.TrimSpace(directBase)
	if !isHTTPNotFound(err) {
		return "", err
	}
	if directBase == "" {
		return "", fmt.Errorf("%w; no monitoring fallback URL configured (%s)", err, monitoringFallbackHint(pluginEndpoint))
	}
	directURL, joinErr := joinBaseURL(directBase, directPath)
	if joinErr != nil {
		return "", fmt.Errorf("invalid monitoring URL: %w", joinErr)
	}
	content, fallbackErr := n.executeGetAbsolute(ctx, directURL, arguments, "application/json")
	if fallbackErr != nil {
		return "", fmt.Errorf("plugin request failed (%v); direct monitoring request failed: %w", err, fallbackErr)
	}
	return content, nil
}

func isHTTPNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, fmt.Sprintf("status %d", http.StatusNotFound)) ||
		strings.Contains(msg, "status code 404")
}

func joinBaseURL(base, path string) (string, error) {
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	if base == "" {
		return "", fmt.Errorf("base URL is empty")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return base, nil
	}
	joined, err := url.JoinPath(base, path)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(joined, "/"), nil
}
