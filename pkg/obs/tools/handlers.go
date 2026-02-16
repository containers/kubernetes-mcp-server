package tools

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/prometheus/common/model"

	"github.com/rhobs/obs-mcp/pkg/alertmanager"
	"github.com/rhobs/obs-mcp/pkg/prometheus"
	"github.com/rhobs/obs-mcp/pkg/resultutil"
)

// GetString is a helper to extract a string parameter with a default value
func GetString(params map[string]any, key, defaultValue string) string {
	if val, ok := params[key]; ok {
		if str, ok := val.(string); ok && str != "" {
			return str
		}
	}
	return defaultValue
}

// GetBoolPtr is a helper to extract an optional boolean parameter as a pointer
func GetBoolPtr(params map[string]any, key string) *bool {
	if val, ok := params[key]; ok {
		if b, ok := val.(bool); ok {
			return &b
		}
	}
	return nil
}

func BuildListMetricsInput(args map[string]any) ListMetricsInput {
	return ListMetricsInput{
		NameRegex: GetString(args, "name_regex", ""),
	}
}

func BuildInstantQueryInput(args map[string]any) InstantQueryInput {
	return InstantQueryInput{
		Query: GetString(args, "query", ""),
		Time:  GetString(args, "time", ""),
	}
}

func BuildRangeQueryInput(args map[string]any) RangeQueryInput {
	return RangeQueryInput{
		Query:    GetString(args, "query", ""),
		Step:     GetString(args, "step", ""),
		Start:    GetString(args, "start", ""),
		End:      GetString(args, "end", ""),
		Duration: GetString(args, "duration", ""),
	}
}

func BuildLabelNamesInput(args map[string]any) LabelNamesInput {
	return LabelNamesInput{
		Metric: GetString(args, "metric", ""),
		Start:  GetString(args, "start", ""),
		End:    GetString(args, "end", ""),
	}
}

func BuildLabelValuesInput(args map[string]any) LabelValuesInput {
	return LabelValuesInput{
		Label:  GetString(args, "label", ""),
		Metric: GetString(args, "metric", ""),
		Start:  GetString(args, "start", ""),
		End:    GetString(args, "end", ""),
	}
}

func BuildSeriesInput(args map[string]any) SeriesInput {
	return SeriesInput{
		Matches: GetString(args, "matches", ""),
		Start:   GetString(args, "start", ""),
		End:     GetString(args, "end", ""),
	}
}

func BuildAlertsInput(args map[string]any) AlertsInput {
	return AlertsInput{
		Active:      GetBoolPtr(args, "active"),
		Silenced:    GetBoolPtr(args, "silenced"),
		Inhibited:   GetBoolPtr(args, "inhibited"),
		Unprocessed: GetBoolPtr(args, "unprocessed"),
		Filter:      GetString(args, "filter", ""),
		Receiver:    GetString(args, "receiver", ""),
	}
}

func BuildSilencesInput(args map[string]any) SilencesInput {
	return SilencesInput{
		Filter: GetString(args, "filter", ""),
	}
}

// ListMetricsHandler handles the listing of available Prometheus metrics.
func ListMetricsHandler(ctx context.Context, promClient prometheus.Loader, input ListMetricsInput) *resultutil.Result {
	slog.Info("ListMetricsHandler called")
	slog.Debug("ListMetricsHandler params", "input", input)

	// Validate required parameters
	if input.NameRegex == "" {
		return resultutil.NewErrorResult(fmt.Errorf("name_regex parameter is required and must be a string"))
	}

	metrics, err := promClient.ListMetrics(ctx, input.NameRegex)
	if err != nil {
		slog.Error("failed to list metrics", "error", err)
		return resultutil.NewErrorResult(fmt.Errorf("failed to list metrics: %w", err))
	}

	slog.Info("ListMetricsHandler executed successfully", "resultLength", len(metrics))
	slog.Debug("ListMetricsHandler results", "results", metrics)

	output := ListMetricsOutput{Metrics: metrics}
	return resultutil.NewSuccessResult(output)
}

// ExecuteRangeQueryHandler handles the execution of Prometheus range queries.
func ExecuteRangeQueryHandler(ctx context.Context, promClient prometheus.Loader, input RangeQueryInput) *resultutil.Result {
	slog.Info("ExecuteRangeQueryHandler called")
	slog.Debug("ExecuteRangeQueryHandler params", "input", input)

	// Validate required parameters
	if input.Query == "" {
		return resultutil.NewErrorResult(fmt.Errorf("query parameter is required and must be a string"))
	}
	if input.Step == "" {
		return resultutil.NewErrorResult(fmt.Errorf("step parameter is required and must be a string"))
	}

	// Parse step duration
	stepDuration, err := model.ParseDuration(input.Step)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("invalid step format: %w", err))
	}

	// Validate parameter combinations
	if input.Start != "" && input.End != "" && input.Duration != "" {
		return resultutil.NewErrorResult(fmt.Errorf("cannot specify both start/end and duration parameters"))
	}

	if (input.Start != "" && input.End == "") || (input.Start == "" && input.End != "") {
		return resultutil.NewErrorResult(fmt.Errorf("both start and end must be provided together"))
	}

	var startTime, endTime time.Time

	// Handle duration-based query (default to 1h if nothing specified)
	if input.Duration != "" || (input.Start == "" && input.End == "") {
		durationStr := input.Duration
		if durationStr == "" {
			durationStr = "1h"
		}

		duration, err := model.ParseDuration(durationStr)
		if err != nil {
			return resultutil.NewErrorResult(fmt.Errorf("invalid duration format: %w", err))
		}

		endTime = time.Now()
		startTime = endTime.Add(-time.Duration(duration))
	} else {
		// Handle explicit start/end times
		startTime, err = prometheus.ParseTimestamp(input.Start)
		if err != nil {
			return resultutil.NewErrorResult(fmt.Errorf("invalid start time format: %w", err))
		}

		endTime, err = prometheus.ParseTimestamp(input.End)
		if err != nil {
			return resultutil.NewErrorResult(fmt.Errorf("invalid end time format: %w", err))
		}
	}

	// Execute the range query
	result, err := promClient.ExecuteRangeQuery(ctx, input.Query, startTime, endTime, time.Duration(stepDuration))
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to execute range query: %w", err))
	}

	// Convert to structured output
	output := RangeQueryOutput{
		ResultType: fmt.Sprintf("%v", result["resultType"]),
	}

	resMatrix, ok := result["result"].(model.Matrix)
	if ok {
		slog.Info("ExecuteRangeQueryHandler executed successfully", "resultLength", resMatrix.Len())
		slog.Debug("ExecuteRangeQueryHandler results", "results", resMatrix)

		output.Result = make([]SeriesResult, len(resMatrix))
		for i, series := range resMatrix {
			labels := make(map[string]string)
			for k, v := range series.Metric {
				labels[string(k)] = string(v)
			}
			values := make([][]any, len(series.Values))
			for j, sample := range series.Values {
				values[j] = []any{float64(sample.Timestamp) / 1000, sample.Value.String()}
			}
			output.Result[i] = SeriesResult{
				Metric: labels,
				Values: values,
			}
		}
	} else {
		slog.Info("ExecuteRangeQueryHandler executed successfully (unknown format)", "result", result)
	}

	if warnings, ok := result["warnings"].([]string); ok {
		output.Warnings = warnings
	}

	return resultutil.NewSuccessResult(output)
}

// ExecuteInstantQueryHandler handles the execution of Prometheus instant queries.
func ExecuteInstantQueryHandler(ctx context.Context, promClient prometheus.Loader, input InstantQueryInput) *resultutil.Result {
	slog.Info("ExecuteInstantQueryHandler called")
	slog.Debug("ExecuteInstantQueryHandler params", "input", input)

	// Validate required parameters
	if input.Query == "" {
		return resultutil.NewErrorResult(fmt.Errorf("query parameter is required and must be a string"))
	}

	var queryTime time.Time
	var err error
	if input.Time == "" {
		queryTime = time.Now()
	} else {
		queryTime, err = prometheus.ParseTimestamp(input.Time)
		if err != nil {
			return resultutil.NewErrorResult(fmt.Errorf("invalid time format: %w", err))
		}
	}

	// Execute the instant query
	result, err := promClient.ExecuteInstantQuery(ctx, input.Query, queryTime)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to execute instant query: %w", err))
	}

	// Convert to structured output
	output := InstantQueryOutput{
		ResultType: fmt.Sprintf("%v", result["resultType"]),
	}

	resVector, ok := result["result"].(model.Vector)
	if ok {
		slog.Info("ExecuteInstantQueryHandler executed successfully", "resultLength", len(resVector))
		slog.Debug("ExecuteInstantQueryHandler results", "results", resVector)

		output.Result = make([]InstantResult, len(resVector))
		for i, sample := range resVector {
			labels := make(map[string]string)
			for k, v := range sample.Metric {
				labels[string(k)] = string(v)
			}
			output.Result[i] = InstantResult{
				Metric: labels,
				Value:  []any{float64(sample.Timestamp) / 1000, sample.Value.String()},
			}
		}
	} else {
		slog.Info("ExecuteInstantQueryHandler executed successfully (unknown format)", "result", result)
	}

	if warnings, ok := result["warnings"].([]string); ok {
		output.Warnings = warnings
	}

	return resultutil.NewSuccessResult(output)
}

// GetLabelNamesHandler handles the retrieval of label names.
func GetLabelNamesHandler(ctx context.Context, promClient prometheus.Loader, input LabelNamesInput) *resultutil.Result {
	slog.Info("GetLabelNamesHandler called")
	slog.Debug("GetLabelNamesHandler params", "input", input)

	// Default to last hour if not specified
	var startTime, endTime time.Time
	var err error
	if input.Start == "" && input.End == "" {
		endTime = time.Now()
		startTime = endTime.Add(-prometheus.ListMetricsTimeRange)
	} else {
		if input.Start != "" {
			startTime, err = prometheus.ParseTimestamp(input.Start)
			if err != nil {
				return resultutil.NewErrorResult(fmt.Errorf("invalid start time format: %w", err))
			}
		}
		if input.End != "" {
			endTime, err = prometheus.ParseTimestamp(input.End)
			if err != nil {
				return resultutil.NewErrorResult(fmt.Errorf("invalid end time format: %w", err))
			}
		}
	}

	// Get label names
	labels, err := promClient.GetLabelNames(ctx, input.Metric, startTime, endTime)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to get label names: %w", err))
	}

	slog.Info("GetLabelNamesHandler executed successfully", "labelCount", len(labels))
	slog.Debug("GetLabelNamesHandler results", "results", labels)

	output := LabelNamesOutput{Labels: labels}
	return resultutil.NewSuccessResult(output)
}

// GetLabelValuesHandler handles the retrieval of label values.
func GetLabelValuesHandler(ctx context.Context, promClient prometheus.Loader, input LabelValuesInput) *resultutil.Result {
	slog.Info("GetLabelValuesHandler called")
	slog.Debug("GetLabelValuesHandler params", "input", input)

	// Validate required parameters
	if input.Label == "" {
		return resultutil.NewErrorResult(fmt.Errorf("label parameter is required and must be a string"))
	}

	// Default to last hour if not specified
	var startTime, endTime time.Time
	var err error
	if input.Start == "" && input.End == "" {
		endTime = time.Now()
		startTime = endTime.Add(-prometheus.ListMetricsTimeRange)
	} else {
		if input.Start != "" {
			startTime, err = prometheus.ParseTimestamp(input.Start)
			if err != nil {
				return resultutil.NewErrorResult(fmt.Errorf("invalid start time format: %w", err))
			}
		}
		if input.End != "" {
			endTime, err = prometheus.ParseTimestamp(input.End)
			if err != nil {
				return resultutil.NewErrorResult(fmt.Errorf("invalid end time format: %w", err))
			}
		}
	}

	// Get label values
	values, err := promClient.GetLabelValues(ctx, input.Label, input.Metric, startTime, endTime)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to get label values: %w", err))
	}

	slog.Info("GetLabelValuesHandler executed successfully", "valueCount", len(values))
	slog.Debug("GetLabelValuesHandler results", "results", values)

	output := LabelValuesOutput{Values: values}
	return resultutil.NewSuccessResult(output)
}

// GetSeriesHandler handles the retrieval of time series.
func GetSeriesHandler(ctx context.Context, promClient prometheus.Loader, input SeriesInput) *resultutil.Result {
	slog.Info("GetSeriesHandler called")
	slog.Debug("GetSeriesHandler params", "input", input)

	// Validate required parameters
	if input.Matches == "" {
		return resultutil.NewErrorResult(fmt.Errorf("matches parameter is required and must be a string"))
	}

	// Parse matches - could be comma-separated
	matches := []string{input.Matches}
	// If it contains comma outside of braces, split it
	// For simplicity, treat the entire string as one match for now
	// Users can make multiple calls if needed

	// Default to last hour if not specified
	var startTime, endTime time.Time
	var err error
	if input.Start == "" && input.End == "" {
		endTime = time.Now()
		startTime = endTime.Add(-prometheus.ListMetricsTimeRange)
	} else {
		if input.Start != "" {
			startTime, err = prometheus.ParseTimestamp(input.Start)
			if err != nil {
				return resultutil.NewErrorResult(fmt.Errorf("invalid start time format: %w", err))
			}
		}
		if input.End != "" {
			endTime, err = prometheus.ParseTimestamp(input.End)
			if err != nil {
				return resultutil.NewErrorResult(fmt.Errorf("invalid end time format: %w", err))
			}
		}
	}

	// Get series
	series, err := promClient.GetSeries(ctx, matches, startTime, endTime)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to get series: %w", err))
	}

	slog.Info("GetSeriesHandler executed successfully", "cardinality", len(series))
	slog.Debug("GetSeriesHandler results", "results", series)

	output := SeriesOutput{
		Series:      series,
		Cardinality: len(series),
	}
	return resultutil.NewSuccessResult(output)
}

// GetAlertsHandler handles the retrieval of alerts from Alertmanager.
func GetAlertsHandler(ctx context.Context, amClient alertmanager.Loader, input AlertsInput) *resultutil.Result {
	slog.Info("GetAlertsHandler called")
	slog.Debug("GetAlertsHandler params", "input", input)

	var filter []string
	if input.Filter != "" {
		// Split by comma if multiple filters are provided
		filter = strings.Split(input.Filter, ",")
		for i := range filter {
			filter[i] = strings.TrimSpace(filter[i])
		}
	}

	alerts, err := amClient.GetAlerts(ctx, input.Active, input.Silenced, input.Inhibited, input.Unprocessed, filter, input.Receiver)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to get alerts: %w", err))
	}

	// Convert to output format
	output := AlertsOutput{
		Alerts: make([]Alert, len(alerts)),
	}

	for i, alert := range alerts {
		labels := make(map[string]string)
		maps.Copy(labels, alert.Labels)

		annotations := make(map[string]string)
		maps.Copy(annotations, alert.Annotations)

		var silencedBy, inhibitedBy []string
		var state string
		if alert.Status != nil {
			if alert.Status.SilencedBy != nil {
				silencedBy = alert.Status.SilencedBy
			}
			if alert.Status.InhibitedBy != nil {
				inhibitedBy = alert.Status.InhibitedBy
			}
			if alert.Status.State != nil {
				state = *alert.Status.State
			}
		}
		if silencedBy == nil {
			silencedBy = []string{}
		}
		if inhibitedBy == nil {
			inhibitedBy = []string{}
		}

		var startsAt, endsAt string
		if alert.StartsAt != nil {
			startsAt = alert.StartsAt.String()
		}
		if alert.EndsAt != nil {
			endsAt = alert.EndsAt.String()
		}

		output.Alerts[i] = Alert{
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

	slog.Info("GetAlertsHandler executed successfully", "alertCount", len(alerts))
	slog.Debug("GetAlertsHandler results", "results", output.Alerts)

	return resultutil.NewSuccessResult(output)
}

// GetSilencesHandler handles the retrieval of silences from Alertmanager.
func GetSilencesHandler(ctx context.Context, amClient alertmanager.Loader, input SilencesInput) *resultutil.Result {
	slog.Info("GetSilencesHandler called")
	slog.Debug("GetSilencesHandler params", "input", input)

	var filter []string
	if input.Filter != "" {
		// Split by comma if multiple filters are provided
		filter = strings.Split(input.Filter, ",")
		for i := range filter {
			filter[i] = strings.TrimSpace(filter[i])
		}
	}

	silences, err := amClient.GetSilences(ctx, filter)
	if err != nil {
		return resultutil.NewErrorResult(fmt.Errorf("failed to get silences: %w", err))
	}

	output := SilencesOutput{
		Silences: make([]Silence, len(silences)),
	}

	for i, silence := range silences {
		matchers := make([]Matcher, len(silence.Matchers))
		for j, m := range silence.Matchers {
			isEqual := true
			if m.IsEqual != nil {
				isEqual = *m.IsEqual
			}
			var name, value string
			var isRegex bool
			if m.Name != nil {
				name = *m.Name
			}
			if m.Value != nil {
				value = *m.Value
			}
			if m.IsRegex != nil {
				isRegex = *m.IsRegex
			}
			matchers[j] = Matcher{
				Name:    name,
				Value:   value,
				IsRegex: isRegex,
				IsEqual: isEqual,
			}
		}

		var id, state, createdBy, comment, startsAt, endsAt string
		if silence.ID != nil {
			id = *silence.ID
		}
		if silence.Status != nil && silence.Status.State != nil {
			state = *silence.Status.State
		}
		if silence.StartsAt != nil {
			startsAt = silence.StartsAt.String()
		}
		if silence.EndsAt != nil {
			endsAt = silence.EndsAt.String()
		}
		if silence.CreatedBy != nil {
			createdBy = *silence.CreatedBy
		}
		if silence.Comment != nil {
			comment = *silence.Comment
		}

		output.Silences[i] = Silence{
			ID: id,
			Status: SilenceStatus{
				State: state,
			},
			Matchers:  matchers,
			StartsAt:  startsAt,
			EndsAt:    endsAt,
			CreatedBy: createdBy,
			Comment:   comment,
		}
	}

	slog.Info("GetSilencesHandler executed successfully", "silenceCount", len(silences))
	slog.Debug("GetSilencesHandler results", "results", output.Silences)

	return resultutil.NewSuccessResult(output)
}
