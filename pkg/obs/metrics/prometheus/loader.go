package prometheus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

const (
	// ListMetricsTimeRange is the time range used when listing metrics
	ListMetricsTimeRange = 1 * time.Hour
	// DefaultQueryTimeout is the default timeout for Prometheus queries
	DefaultQueryTimeout = 30 * time.Second
)

// Loader defines the interface for querying Prometheus
type Loader interface {
	ListMetrics(ctx context.Context, nameRegex string) ([]string, error)
	ExecuteRangeQuery(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error)
	ExecuteInstantQuery(ctx context.Context, query string, time time.Time) (map[string]any, error)
	GetLabelNames(ctx context.Context, metricName string, start, end time.Time) ([]string, error)
	GetLabelValues(ctx context.Context, label string, metricName string, start, end time.Time) ([]string, error)
	GetSeries(ctx context.Context, matches []string, start, end time.Time) ([]map[string]string, error)
}

// RealLoader implements Loader using the Prometheus HTTP API.
type RealLoader struct {
	client     v1.API
	guardrails *Guardrails
	backend    string
}

var _ Loader = (*RealLoader)(nil)

func NewPrometheusLoader(apiConfig api.Config) (*RealLoader, error) {
	client, err := api.NewClient(apiConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	backend := "prometheus"
	if strings.Contains(strings.ToLower(apiConfig.Address), "thanos") {
		backend = "thanos"
	}

	v1api := v1.NewAPI(client)
	return &RealLoader{
		client:     v1api,
		guardrails: DefaultGuardrails(true),
		backend:    backend,
	}, nil
}

// WithGuardrails sets a custom Guardrails configuration for the client.
func (p *RealLoader) WithGuardrails(g *Guardrails) *RealLoader {
	p.guardrails = g
	return p
}

func (p *RealLoader) ListMetrics(ctx context.Context, nameRegex string) ([]string, error) {
	var matches []string

	// For blanket regex patterns like ".*", use empty matcher to get all metrics to not get 4xx.
	if nameRegex != ".*" && nameRegex != ".+" && nameRegex != "" {
		if _, err := regexp.Compile(nameRegex); err != nil {
			return nil, fmt.Errorf("invalid name_regex %q: %w", nameRegex, err)
		}
		if strings.ContainsAny(nameRegex, `"}`) {
			return nil, fmt.Errorf("invalid name_regex %q: contains disallowed characters", nameRegex)
		}
		matcher := fmt.Sprintf("{__name__=~\"%s\"}", nameRegex) //nolint:gocritic
		matches = []string{matcher}
	}

	start := time.Now()
	labelValues, _, err := p.client.LabelValues(ctx, "__name__", matches, time.Now().Add(-ListMetricsTimeRange), time.Now())
	duration := time.Since(start)
	if err != nil {
		slog.Error("Backend call failed", "backend", p.backend, "operation", "list_metrics",
			"duration_ms", duration.Milliseconds(), "error", err)
		return nil, fmt.Errorf("error fetching metric names: %w", err)
	}
	slog.Debug("Backend call completed", "backend", p.backend, "operation", "list_metrics",
		"duration_ms", duration.Milliseconds(), "result_count", len(labelValues))

	metrics := make([]string, len(labelValues))
	for i, value := range labelValues {
		metrics[i] = string(value)
	}
	return metrics, nil
}

// ValidateMetricsExist validates that all metrics referenced in a query exist in Prometheus TSDB.
// This is an always-on validation that should be called before executing any query.
// It uses ListMetrics to fetch available metrics and ensures all metrics in the query exist.
func (p *RealLoader) ValidateMetricsExist(ctx context.Context, query string) error {
	metricNames, err := ExtractMetricNames(query)
	if err != nil {
		return fmt.Errorf("failed to extract metric names: %w", err)
	}

	// If no metrics in query, nothing to validate
	if len(metricNames) == 0 {
		return nil
	}

	// Use ".*" to match all metrics for validation
	availableMetricsList, err := p.ListMetrics(ctx, ".*")
	if err != nil {
		return fmt.Errorf("failed to fetch available metrics: %w", err)
	}

	availableMetrics := make(map[string]bool)
	for _, metric := range availableMetricsList {
		availableMetrics[metric] = true
	}

	for _, metricName := range metricNames {
		if !availableMetrics[metricName] {
			return fmt.Errorf("metric %q does not exist in the metrics backend, please check the query and try again", metricName)
		}
	}

	return nil
}

// validateQuery checks that all metrics in the query exist and that
// the query passes any configured guardrails.
func (p *RealLoader) validateQuery(ctx context.Context, query string) error {
	if err := p.ValidateMetricsExist(ctx, query); err != nil {
		slog.Warn("Query validation rejected", "reason", "metric-not-found", "query", query, "error", err)
		return fmt.Errorf("metric validation failed: %w", err)
	}

	if p.guardrails != nil {
		isSafe, err := p.guardrails.IsSafeQuery(ctx, query, p.client)
		if err != nil {
			guardrail := "unknown"
			var gv *GuardrailViolation
			if errors.As(err, &gv) {
				guardrail = gv.Guardrail
			}
			slog.Warn("Guardrail rejected query", "guardrail", guardrail, "query", query, "error", err)
			return fmt.Errorf("query validation failed: %w", err)
		}
		if !isSafe {
			return fmt.Errorf("query is not safe")
		}
	}

	return nil
}

func (p *RealLoader) ExecuteRangeQuery(ctx context.Context, query string, queryStart, queryEnd time.Time, step time.Duration) (map[string]any, error) {
	if err := p.validateQuery(ctx, query); err != nil {
		return nil, err
	}

	r := v1.Range{
		Start: queryStart,
		End:   queryEnd,
		Step:  step,
	}

	start := time.Now()
	result, warnings, err := p.client.QueryRange(ctx, query, r, v1.WithTimeout(DefaultQueryTimeout))
	duration := time.Since(start)
	if err != nil {
		slog.Error("Backend call failed", "backend", p.backend, "operation", "range_query",
			"duration_ms", duration.Milliseconds(), "query", query, "error", err)
		return nil, fmt.Errorf("error executing range query: %w", err)
	}
	slog.Debug("Backend call completed", "backend", p.backend, "operation", "range_query",
		"duration_ms", duration.Milliseconds(), "query", query)

	response := map[string]any{
		"resultType": result.Type().String(),
		"result":     result,
	}

	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	return response, nil
}

func (p *RealLoader) ExecuteInstantQuery(ctx context.Context, query string, ts time.Time) (map[string]any, error) {
	if err := p.validateQuery(ctx, query); err != nil {
		return nil, err
	}

	start := time.Now()
	result, warnings, err := p.client.Query(ctx, query, ts)
	duration := time.Since(start)
	if err != nil {
		slog.Error("Backend call failed", "backend", p.backend, "operation", "instant_query",
			"duration_ms", duration.Milliseconds(), "query", query, "error", err)
		return nil, fmt.Errorf("error executing instant query: %w", err)
	}
	slog.Debug("Backend call completed", "backend", p.backend, "operation", "instant_query",
		"duration_ms", duration.Milliseconds(), "query", query)

	response := map[string]any{
		"resultType": result.Type().String(),
		"result":     result,
	}

	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	return response, nil
}

func (p *RealLoader) GetLabelNames(ctx context.Context, metricName string, start, end time.Time) ([]string, error) {
	var matches []string
	if metricName != "" {
		matches = []string{metricName}
	}

	apiStart := time.Now()
	labelNames, _, err := p.client.LabelNames(ctx, matches, start, end)
	duration := time.Since(apiStart)
	if err != nil {
		slog.Error("Backend call failed", "backend", p.backend, "operation", "label_names",
			"duration_ms", duration.Milliseconds(), "error", err)
		return nil, fmt.Errorf("error fetching label names: %w", err)
	}
	slog.Debug("Backend call completed", "backend", p.backend, "operation", "label_names",
		"duration_ms", duration.Milliseconds(), "result_count", len(labelNames))

	labels := make([]string, len(labelNames))
	copy(labels, labelNames)
	return labels, nil
}

func (p *RealLoader) GetLabelValues(ctx context.Context, label, metricName string, start, end time.Time) ([]string, error) {
	var matches []string
	if metricName != "" {
		matches = []string{metricName}
	}

	apiStart := time.Now()
	labelValues, _, err := p.client.LabelValues(ctx, label, matches, start, end)
	duration := time.Since(apiStart)
	if err != nil {
		slog.Error("Backend call failed", "backend", p.backend, "operation", "label_values",
			"duration_ms", duration.Milliseconds(), "label", label, "error", err)
		return nil, fmt.Errorf("error fetching label values: %w", err)
	}
	slog.Debug("Backend call completed", "backend", p.backend, "operation", "label_values",
		"duration_ms", duration.Milliseconds(), "label", label, "result_count", len(labelValues))

	values := make([]string, len(labelValues))
	for i, value := range labelValues {
		values[i] = string(value)
	}
	return values, nil
}

func (p *RealLoader) GetSeries(ctx context.Context, matches []string, start, end time.Time) ([]map[string]string, error) {
	apiStart := time.Now()
	seriesList, _, err := p.client.Series(ctx, matches, start, end)
	duration := time.Since(apiStart)
	if err != nil {
		slog.Error("Backend call failed", "backend", p.backend, "operation", "series",
			"duration_ms", duration.Milliseconds(), "error", err)
		return nil, fmt.Errorf("error fetching series: %w", err)
	}
	slog.Debug("Backend call completed", "backend", p.backend, "operation", "series",
		"duration_ms", duration.Milliseconds(), "result_count", len(seriesList))

	result := make([]map[string]string, len(seriesList))
	for i, series := range seriesList {
		labels := make(map[string]string)
		for k, v := range series {
			labels[string(k)] = string(v)
		}
		result[i] = labels
	}
	return result, nil
}
