package metrics

import (
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"

	ammodels "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/common/model"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"

	"github.com/rhobs/obs-mcp/pkg/metrics/prometheus"
)

const (
	// millisecondsPerSecond converts Prometheus millisecond timestamps to seconds.
	millisecondsPerSecond = 1000
)

// GetBoolPtr is a helper to extract an optional boolean parameter as a pointer
func GetBoolPtr(params map[string]any, key string) *bool {
	if val, ok := params[key]; ok {
		if b, ok := val.(bool); ok {
			return &b
		}
	}
	return nil
}

// parseDefaultTimeRange parses optional start/end time strings,
// defaulting to the last hour if both are empty.
func parseDefaultTimeRange(start, end string) (startTime, endTime time.Time, err error) {
	if start == "" && end == "" {
		endTime = time.Now()
		startTime = endTime.Add(-prometheus.ListMetricsTimeRange)
		return startTime, endTime, nil
	}

	if start != "" {
		startTime, err = prometheus.ParseTimestamp(start)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start time format: %w", err)
		}
	}
	if end != "" {
		endTime, err = prometheus.ParseTimestamp(end)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end time format: %w", err)
		}
	}
	return startTime, endTime, nil
}

// parseFilterString splits a comma-separated filter string into trimmed parts.
func parseFilterString(filter string) []string {
	if filter == "" {
		return nil
	}
	parts := strings.Split(filter, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// convertAlert converts an Alertmanager GettableAlert to the Alert output type.
func convertAlert(a *ammodels.GettableAlert) Alert {
	labels := make(map[string]string)
	maps.Copy(labels, a.Labels)

	annotations := make(map[string]string)
	maps.Copy(annotations, a.Annotations)

	var silencedBy, inhibitedBy []string
	var state string
	if a.Status != nil {
		if a.Status.SilencedBy != nil {
			silencedBy = a.Status.SilencedBy
		}
		if a.Status.InhibitedBy != nil {
			inhibitedBy = a.Status.InhibitedBy
		}
		state = ptr.Deref(a.Status.State, "")
	}
	if silencedBy == nil {
		silencedBy = []string{}
	}
	if inhibitedBy == nil {
		inhibitedBy = []string{}
	}

	var startsAt, endsAt string
	if a.StartsAt != nil {
		startsAt = a.StartsAt.String()
	}
	if a.EndsAt != nil {
		endsAt = a.EndsAt.String()
	}

	return Alert{
		Labels:      labels,
		Annotations: annotations,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		Status: AlertStatus{
			State:       state,
			SilencedBy:  silencedBy,
			InhibitedBy: inhibitedBy,
		},
	}
}

// convertMatcher converts an Alertmanager Matcher to the Matcher output type.
func convertMatcher(m *ammodels.Matcher) Matcher {
	isEqual := true
	if m.IsEqual != nil {
		isEqual = *m.IsEqual
	}
	return Matcher{
		Name:    ptr.Deref(m.Name, ""),
		Value:   ptr.Deref(m.Value, ""),
		IsRegex: m.IsRegex != nil && *m.IsRegex,
		IsEqual: isEqual,
	}
}

// convertSilence converts an Alertmanager GettableSilence to the Silence output type.
func convertSilence(s *ammodels.GettableSilence) Silence {
	matchers := make([]Matcher, len(s.Matchers))
	for i, m := range s.Matchers {
		matchers[i] = convertMatcher(m)
	}

	var state string
	if s.Status != nil {
		state = ptr.Deref(s.Status.State, "")
	}

	var startsAt, endsAt string
	if s.StartsAt != nil {
		startsAt = s.StartsAt.String()
	}
	if s.EndsAt != nil {
		endsAt = s.EndsAt.String()
	}

	return Silence{
		ID: ptr.Deref(s.ID, ""),
		Status: SilenceStatus{
			State: state,
		},
		Matchers:  matchers,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		CreatedBy: ptr.Deref(s.CreatedBy, ""),
		Comment:   ptr.Deref(s.Comment, ""),
	}
}

// listMetricsHandler handles the listing of available Prometheus metrics.
func listMetricsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	nameRegex := p.RequiredString("name_regex")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list metrics: %w", err)), nil
	}

	slog.Info("listMetricsHandler called")
	slog.Debug("listMetricsHandler params", "name_regex", nameRegex)

	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	metrics, err := promClient.ListMetrics(params.Context, nameRegex)
	if err != nil {
		slog.Error("failed to list metrics", "error", err)
		return api.NewToolCallResult("", fmt.Errorf("failed to list metrics: %w", err)), nil
	}

	slog.Info("listMetricsHandler executed successfully", "resultLength", len(metrics))
	slog.Debug("listMetricsHandler results", "results", metrics)

	output := ListMetricsOutput{Metrics: metrics}
	return api.NewToolCallResultStructured(output, nil), nil
}

// executeRangeQueryHandler handles the execution of Prometheus range queries.
func executeRangeQueryHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	query := p.RequiredString("query")
	step := p.RequiredString("step")
	start := p.OptionalString("start", "")
	end := p.OptionalString("end", "")
	duration := p.OptionalString("duration", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to execute range query: %w", err)), nil
	}

	slog.Info("executeRangeQueryHandler called")
	slog.Debug("executeRangeQueryHandler params", "query", query, "step", step, "start", start, "end", end, "duration", duration)

	// Validate required parameters
	if query == "" {
		return api.NewToolCallResult("", fmt.Errorf("query parameter is required and must be a string")), nil
	}
	if step == "" {
		return api.NewToolCallResult("", fmt.Errorf("step parameter is required and must be a string")), nil
	}

	// Parse step duration
	stepDuration, err := model.ParseDuration(step)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("invalid step format: %w", err)), nil
	}

	if (start == "") != (end == "") {
		return api.NewToolCallResult("", fmt.Errorf("both start and end must be provided together")), nil
	}

	var startTime, endTime time.Time

	if start != "" && end != "" {
		// Handle explicit start/end times
		startTime, err = prometheus.ParseTimestamp(start)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("invalid start time format: %w", err)), nil
		}

		endTime, err = prometheus.ParseTimestamp(end)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("invalid end time format: %w", err)), nil
		}
	} else {
		// Handle duration-based query (default to 1h if nothing specified)
		durationStr := duration
		if durationStr == "" {
			durationStr = "1h"
		}

		dur, err := model.ParseDuration(durationStr)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("invalid duration format: %w", err)), nil
		}

		endTime = time.Now()
		startTime = endTime.Add(-time.Duration(dur))
	}

	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	cfg := getConfig(params)
	fullResponse := cfg.RangeQueryFullResponse

	// Execute the range query
	result, err := promClient.ExecuteRangeQuery(params.Context, query, startTime, endTime, time.Duration(stepDuration))
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to execute range query: %w", err)), nil
	}

	// Convert to structured output
	output := RangeQueryOutput{
		ResultType: fmt.Sprintf("%v", result["resultType"]),
	}

	resMatrix, ok := result["result"].(model.Matrix)
	if ok {
		slog.Info("executeRangeQueryHandler executed successfully", "resultLength", resMatrix.Len())

		if fullResponse {
			// Return full data
			output.Result = make([]SeriesResult, len(resMatrix))
			for i, series := range resMatrix {
				labels := make(map[string]string)
				for k, v := range series.Metric {
					labels[string(k)] = string(v)
				}
				values := make([][]any, len(series.Values))
				for j, sample := range series.Values {
					values[j] = []any{float64(sample.Timestamp) / millisecondsPerSecond, sample.Value.String()}
				}
				output.Result[i] = SeriesResult{
					Metric: labels,
					Values: values,
				}
			}
		} else {
			// Return summary statistics instead of full data
			output.Summary = make([]SeriesResultSummary, len(resMatrix))
			for i, series := range resMatrix {
				output.Summary[i] = CalculateSeriesSummary(series.Metric, series.Values)
			}
		}

		slog.Debug("executeRangeQueryHandler output", "output", output)
	} else {
		slog.Info("executeRangeQueryHandler executed successfully (unknown format)", "result", result)
	}

	if warnings, ok := result["warnings"].([]string); ok {
		output.Warnings = warnings
	}

	return api.NewToolCallResultStructured(output, nil), nil
}

// showTimeseriesHandler handles the show_timeseries tool, returning full range query data for chart rendering.
func showTimeseriesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	slog.Info("showTimeseriesHandler called")

	// Executing the query handler just to validate the query is correct.
	result, err := executeRangeQueryHandler(params)
	if err != nil || result.Error != nil {
		return result, err
	}

	return api.NewToolCallResultStructured(struct{}{}, nil), nil
}

// executeInstantQueryHandler handles the execution of Prometheus instant queries.
func executeInstantQueryHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	query := p.RequiredString("query")
	timeStr := p.OptionalString("time", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to execute instant query: %w", err)), nil
	}

	slog.Info("executeInstantQueryHandler called")
	slog.Debug("executeInstantQueryHandler params", "query", query, "time", timeStr)

	// Validate required parameters
	if query == "" {
		return api.NewToolCallResult("", fmt.Errorf("query parameter is required and must be a string")), nil
	}

	var queryTime time.Time
	var err error
	if timeStr == "" {
		queryTime = time.Now()
	} else {
		queryTime, err = prometheus.ParseTimestamp(timeStr)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("invalid time format: %w", err)), nil
		}
	}

	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	// Execute the instant query
	result, err := promClient.ExecuteInstantQuery(params.Context, query, queryTime)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to execute instant query: %w", err)), nil
	}

	// Convert to structured output
	output := InstantQueryOutput{
		ResultType: fmt.Sprintf("%v", result["resultType"]),
	}

	resVector, ok := result["result"].(model.Vector)
	if ok {
		slog.Info("executeInstantQueryHandler executed successfully", "resultLength", len(resVector))
		slog.Debug("executeInstantQueryHandler results", "results", resVector)

		output.Result = make([]InstantResult, len(resVector))
		for i, sample := range resVector {
			labels := make(map[string]string)
			for k, v := range sample.Metric {
				labels[string(k)] = string(v)
			}
			output.Result[i] = InstantResult{
				Metric: labels,
				Value:  []any{float64(sample.Timestamp) / millisecondsPerSecond, sample.Value.String()},
			}
		}
	} else {
		slog.Info("executeInstantQueryHandler executed successfully (unknown format)", "result", result)
	}

	if warnings, ok := result["warnings"].([]string); ok {
		output.Warnings = warnings
	}

	return api.NewToolCallResultStructured(output, nil), nil
}

// getLabelNamesHandler handles the retrieval of label names.
func getLabelNamesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	metric := p.OptionalString("metric", "")
	start := p.OptionalString("start", "")
	end := p.OptionalString("end", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get label names: %w", err)), nil
	}

	slog.Info("getLabelNamesHandler called")
	slog.Debug("getLabelNamesHandler params", "metric", metric, "start", start, "end", end)

	startTime, endTime, err := parseDefaultTimeRange(start, end)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	// Get label names
	labels, err := promClient.GetLabelNames(params.Context, metric, startTime, endTime)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get label names: %w", err)), nil
	}

	slog.Info("getLabelNamesHandler executed successfully", "labelCount", len(labels))
	slog.Debug("getLabelNamesHandler results", "results", labels)

	output := LabelNamesOutput{Labels: labels}
	return api.NewToolCallResultStructured(output, nil), nil
}

// getLabelValuesHandler handles the retrieval of label values.
func getLabelValuesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	label := p.RequiredString("label")
	metric := p.OptionalString("metric", "")
	start := p.OptionalString("start", "")
	end := p.OptionalString("end", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get label values: %w", err)), nil
	}

	slog.Info("getLabelValuesHandler called")
	slog.Debug("getLabelValuesHandler params", "label", label, "metric", metric, "start", start, "end", end)

	// Validate required parameters
	if label == "" {
		return api.NewToolCallResult("", fmt.Errorf("label parameter is required and must be a string")), nil
	}

	startTime, endTime, err := parseDefaultTimeRange(start, end)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	// Get label values
	values, err := promClient.GetLabelValues(params.Context, label, metric, startTime, endTime)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get label values: %w", err)), nil
	}

	slog.Info("getLabelValuesHandler executed successfully", "valueCount", len(values))
	slog.Debug("getLabelValuesHandler results", "results", values)

	output := LabelValuesOutput{Values: values}
	return api.NewToolCallResultStructured(output, nil), nil
}

// getSeriesHandler handles the retrieval of time series.
func getSeriesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	matches := p.RequiredString("matches")
	start := p.OptionalString("start", "")
	end := p.OptionalString("end", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get series: %w", err)), nil
	}

	slog.Info("getSeriesHandler called")
	slog.Debug("getSeriesHandler params", "matches", matches, "start", start, "end", end)

	// Validate required parameters
	if matches == "" {
		return api.NewToolCallResult("", fmt.Errorf("matches parameter is required and must be a string")), nil
	}

	// Parse matches - could be comma-separated
	matchList := []string{matches}
	// If it contains comma outside of braces, split it
	// For simplicity, treat the entire string as one match for now
	// Users can make multiple calls if needed

	startTime, endTime, err := parseDefaultTimeRange(start, end)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	// Get series
	series, err := promClient.GetSeries(params.Context, matchList, startTime, endTime)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get series: %w", err)), nil
	}

	slog.Info("getSeriesHandler executed successfully", "cardinality", len(series))
	slog.Debug("getSeriesHandler results", "results", series)

	output := SeriesOutput{
		Series:      series,
		Cardinality: len(series),
	}
	return api.NewToolCallResultStructured(output, nil), nil
}

// getAlertsHandler handles the retrieval of alerts from Alertmanager.
func getAlertsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	filter := p.OptionalString("filter", "")
	receiver := p.OptionalString("receiver", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get alerts: %w", err)), nil
	}

	args := params.GetArguments()
	active := GetBoolPtr(args, "active")
	silenced := GetBoolPtr(args, "silenced")
	inhibited := GetBoolPtr(args, "inhibited")
	unprocessed := GetBoolPtr(args, "unprocessed")

	slog.Info("getAlertsHandler called")
	slog.Debug("getAlertsHandler params", "active", active, "silenced", silenced, "inhibited", inhibited, "unprocessed", unprocessed, "filter", filter, "receiver", receiver)

	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}

	alerts, err := amClient.GetAlerts(params.Context, active, silenced, inhibited, unprocessed, parseFilterString(filter), receiver)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get alerts: %w", err)), nil
	}

	output := AlertsOutput{
		Alerts: make([]Alert, len(alerts)),
	}
	for i, alert := range alerts {
		output.Alerts[i] = convertAlert(alert)
	}

	slog.Info("getAlertsHandler executed successfully", "alertCount", len(alerts))
	slog.Debug("getAlertsHandler results", "results", output.Alerts)

	return api.NewToolCallResultStructured(output, nil), nil
}

// getSilencesHandler handles the retrieval of silences from Alertmanager.
func getSilencesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	filter := p.OptionalString("filter", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get silences: %w", err)), nil
	}

	slog.Info("getSilencesHandler called")
	slog.Debug("getSilencesHandler params", "filter", filter)

	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}

	silences, err := amClient.GetSilences(params.Context, parseFilterString(filter))
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get silences: %w", err)), nil
	}

	output := SilencesOutput{
		Silences: make([]Silence, len(silences)),
	}
	for i, silence := range silences {
		output.Silences[i] = convertSilence(silence)
	}

	slog.Info("getSilencesHandler executed successfully", "silenceCount", len(silences))
	slog.Debug("getSilencesHandler results", "results", output.Silences)

	return api.NewToolCallResultStructured(output, nil), nil
}
