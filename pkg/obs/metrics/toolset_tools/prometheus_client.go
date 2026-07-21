package toolset_tools

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	promapi "github.com/prometheus/client_golang/api"

	"github.com/rhobs/obs-mcp/pkg/auth"
	"github.com/rhobs/obs-mcp/pkg/metrics"
	"github.com/rhobs/obs-mcp/pkg/metrics/alertmanager"
	"github.com/rhobs/obs-mcp/pkg/metrics/prometheus"
)

const (
	defaultPrometheusURL = "http://localhost:9090"
)

// getConfig retrieves the obs-mcp toolset configuration from params.
func getConfig(params api.ToolHandlerParams) *metrics.Config {
	if cfg, ok := params.GetToolsetConfig(metrics.ToolsetName); ok {
		if obsCfg, ok := cfg.(*metrics.Config); ok {
			return obsCfg
		}
	}
	// Return default config if not found
	return &metrics.Config{}
}

// getPromClient creates a Prometheus client using the toolset configuration.
func getPromClient(params api.ToolHandlerParams) (prometheus.Loader, error) {
	cfg := getConfig(params)

	// Get metrics backend URL from config, fallback to default
	metricsBackendURL := cfg.PrometheusURL
	if metricsBackendURL == "" {
		metricsBackendURL = defaultPrometheusURL
		slog.Info("No prometheus_url configured, using default", "url", defaultPrometheusURL)
	}

	// Get guardrails configuration
	guardrails, err := cfg.GetGuardrails()
	if err != nil {
		slog.Warn("Failed to parse guardrails configuration", "err", err)
	}

	apiConfig, err := buildAPIConfig(params, metricsBackendURL, cfg.Insecure, cfg.GetAuthMode())
	if err != nil {
		return nil, fmt.Errorf("failed to create API config: %w", err)
	}

	promClient, err := prometheus.NewPrometheusLoader(apiConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	promClient.WithGuardrails(guardrails)

	return promClient, nil
}

// buildAPIConfig creates a Prometheus API config using the configured auth mode.
func buildAPIConfig(params api.ToolHandlerParams, prometheusURL string, insecure bool, authMode auth.AuthMode) (promapi.Config, error) {
	tls := strings.HasPrefix(prometheusURL, "https://")
	rt, err := auth.BuildRoundTripper(params.Context, params.RESTConfig(), authMode, tls, insecure)
	if err != nil {
		return promapi.Config{}, fmt.Errorf("failed to create round tripper: %w", err)
	}

	return promapi.Config{
		Address:      prometheusURL,
		RoundTripper: rt,
	}, nil
}

// getAlertmanagerClient creates an Alertmanager client using the toolset configuration.
func getAlertmanagerClient(params api.ToolHandlerParams) (alertmanager.Loader, error) {
	cfg := getConfig(params)

	alertmanagerURL := cfg.AlertmanagerURL
	if alertmanagerURL == "" {
		return nil, fmt.Errorf("alertmanager_url not configured")
	}

	apiConfig, err := buildAPIConfig(params, alertmanagerURL, cfg.Insecure, cfg.GetAuthMode())
	if err != nil {
		return nil, fmt.Errorf("failed to create API config: %w", err)
	}

	amClient, err := alertmanager.NewAlertmanagerClient(apiConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Alertmanager client: %w", err)
	}

	return amClient, nil
}
