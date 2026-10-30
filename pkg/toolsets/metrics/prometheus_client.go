package metrics

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	promapi "github.com/prometheus/client_golang/api"

	"github.com/containers/kubernetes-mcp-server/pkg/obs/auth"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/metrics/alertmanager"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/metrics/prometheus"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/toolcfg"
)

const (
	defaultPrometheusURL = "http://localhost:9090"
)

type contextKey string

const (
	testPromClientKey contextKey = "testPromClient"
	testAMClientKey   contextKey = "testAMClient"
)

// getConfig retrieves the observability metrics toolset configuration from params.
func getConfig(params api.ToolHandlerParams) *Config {
	if cfg, ok := toolcfg.From(params, ToolsetName); ok {
		if obsCfg, ok := cfg.(*Config); ok {
			return obsCfg
		}
	}
	// Return default config if not found
	return &Config{}
}

// getPromClient creates a Prometheus client using the toolset configuration.
func getPromClient(params api.ToolHandlerParams) (prometheus.Loader, error) {
	if client, ok := params.Value(testPromClientKey).(prometheus.Loader); ok {
		return client, nil
	}

	cfg := getConfig(params)

	// Get metrics backend URL from config, fallback to default
	metricsBackendURL := cfg.PrometheusURL
	if metricsBackendURL == "" {
		metricsBackendURL = defaultPrometheusURL
		klogutil.FromContext(params.Context).Info("No prometheus_url configured, using default", "url", defaultPrometheusURL)
	}

	// Get guardrails configuration
	guardrails, err := cfg.GetGuardrails()
	if err != nil {
		klogutil.LogWarn(klogutil.FromContext(params.Context), "Failed to parse guardrails configuration", klogutil.Err(err))
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
	rt, err := auth.BuildRoundTripper(params.Context, params.Config, params.RESTConfig(), authMode, tls, insecure)
	if err != nil {
		return promapi.Config{}, fmt.Errorf("failed to create round tripper: %w", err)
	}
	// Prometheus and Alertmanager read the body inside their client libraries.
	// The wrapper stops that read at max_backend_response_bytes.
	rt = kubernetes.NewBackendResponseRoundTripper(rt, kubernetes.LiveBackendLimit(params.Config))

	return promapi.Config{
		Address:      prometheusURL,
		RoundTripper: rt,
	}, nil
}

// getAlertmanagerClient creates an Alertmanager client using the toolset configuration.
func getAlertmanagerClient(params api.ToolHandlerParams) (alertmanager.Loader, error) {
	if client, ok := params.Value(testAMClientKey).(alertmanager.Loader); ok {
		return client, nil
	}

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
