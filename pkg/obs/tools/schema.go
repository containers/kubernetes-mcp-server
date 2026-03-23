package tools

// ListMetricsOutput defines the output schema for the list_metrics tool.
type ListMetricsOutput struct {
	Metrics []string `json:"metrics" jsonschema:"List of all available metric names"`
}

// InstantQueryOutput defines the output schema for the execute_instant_query tool.
type InstantQueryOutput struct {
	ResultType string          `json:"resultType" jsonschema:"The type of result returned (e.g. vector, scalar, string)"`
	Result     []InstantResult `json:"result" jsonschema:"The query results as an array of instant values"`
	Warnings   []string        `json:"warnings,omitempty" jsonschema:"Any warnings generated during query execution"`
}

// InstantResult represents a single instant query result.
type InstantResult struct {
	Metric map[string]string `json:"metric" jsonschema:"The metric labels"`
	Value  []any             `json:"value" jsonschema:"[timestamp, value] pair for the instant query"`
}

// LabelNamesOutput defines the output schema for the get_label_names tool.
type LabelNamesOutput struct {
	Labels []string `json:"labels" jsonschema:"List of label names available for the specified metric or all metrics"`
}

// LabelValuesOutput defines the output schema for the get_label_values tool.
type LabelValuesOutput struct {
	Values []string `json:"values" jsonschema:"List of unique values for the specified label"`
}

// SeriesOutput defines the output schema for the get_series tool.
type SeriesOutput struct {
	Series      []map[string]string `json:"series" jsonschema:"List of time series matching the selector, each series is a map of label names to values"`
	Cardinality int                 `json:"cardinality" jsonschema:"Total number of series matching the selector"`
}

// RangeQueryOutput defines the output schema for the execute_range_query tool.
type RangeQueryOutput struct {
	ResultType string                `json:"resultType" jsonschema:"The type of result returned: matrix or vector or scalar"`
	Result     []SeriesResult        `json:"result,omitempty" jsonschema:"The query results as an array of time series"`
	Summary    []SeriesResultSummary `json:"summary,omitempty" jsonschema:"Summary statistics for each time series (when summarize flag is enabled)"`
	Warnings   []string              `json:"warnings,omitempty" jsonschema:"Any warnings generated during query execution"`
}

// SeriesResult represents a single time series result from a range query.
type SeriesResult struct {
	Metric map[string]string `json:"metric" jsonschema:"The metric labels"`
	Values [][]any           `json:"values" jsonschema:"Array of [timestamp, value] pairs"`
}

// SeriesResultSummary represents a summary of a time series result from a range query.
type SeriesResultSummary struct {
	Series         map[string]string `json:"series" jsonschema:"The query result series labelset as a map of label names to values"`
	Max            float64           `json:"max" jsonschema:"Maximum value in the series (excluding NaN/Inf)"`
	Min            float64           `json:"min" jsonschema:"Minimum value in the series (excluding NaN/Inf)"`
	Avg            float64           `json:"avg" jsonschema:"Average value of all finite samples in the series"`
	Count          int               `json:"count" jsonschema:"Total number of samples in the series"`
	FirstTimestamp float64           `json:"firstTimestamp" jsonschema:"Timestamp of the first sample (Unix seconds)"`
	LastTimestamp  float64           `json:"lastTimestamp" jsonschema:"Timestamp of the last sample (Unix seconds)"`
	FirstValue     float64           `json:"firstValue" jsonschema:"Value of the first sample"`
	LastValue      float64           `json:"lastValue" jsonschema:"Value of the last sample"`
	Delta          float64           `json:"delta" jsonschema:"Difference between last and first values (lastValue - firstValue)"`
	HasNaN         bool              `json:"hasNaN" jsonschema:"Whether the series contains any NaN values"`
	HasInf         bool              `json:"hasInf" jsonschema:"Whether the series contains any Inf values"`
	NonFiniteCount int               `json:"nonFiniteCount" jsonschema:"Count of NaN and Inf values in the series"`
}

// AlertsOutput defines the output schema for the get_alerts tool.
type AlertsOutput struct {
	Alerts []Alert `json:"alerts" jsonschema:"List of alerts from Alertmanager"`
}

// Alert represents a single alert from Alertmanager.
type Alert struct {
	Labels      map[string]string `json:"labels" jsonschema:"Labels of the alert"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the alert"`
	StartsAt    string            `json:"startsAt" jsonschema:"Start time of the alert"`
	EndsAt      string            `json:"endsAt,omitempty" jsonschema:"End time of the alert (if resolved)"`
	Status      AlertStatus       `json:"status" jsonschema:"Current status of the alert"`
}

// AlertStatus represents the status of an alert.
type AlertStatus struct {
	State       string   `json:"state" jsonschema:"State of the alert (active, suppressed, unprocessed)"`
	SilencedBy  []string `json:"silencedBy,omitempty" jsonschema:"List of silences that are silencing this alert"`
	InhibitedBy []string `json:"inhibitedBy,omitempty" jsonschema:"List of alerts that are inhibiting this alert"`
}

// SilencesOutput defines the output schema for the get_silences tool.
type SilencesOutput struct {
	Silences []Silence `json:"silences" jsonschema:"List of silences from Alertmanager"`
}

// Silence represents a single silence from Alertmanager.
type Silence struct {
	ID        string        `json:"id" jsonschema:"Unique identifier of the silence"`
	Status    SilenceStatus `json:"status" jsonschema:"Current status of the silence"`
	Matchers  []Matcher     `json:"matchers" jsonschema:"Label matchers for this silence"`
	StartsAt  string        `json:"startsAt" jsonschema:"Start time of the silence"`
	EndsAt    string        `json:"endsAt" jsonschema:"End time of the silence"`
	CreatedBy string        `json:"createdBy" jsonschema:"Creator of the silence"`
	Comment   string        `json:"comment" jsonschema:"Comment describing the silence"`
}

// SilenceStatus represents the status of a silence.
type SilenceStatus struct {
	State string `json:"state" jsonschema:"State of the silence (active, pending, expired)"`
}

// Matcher represents a label matcher for a silence.
type Matcher struct {
	Name    string `json:"name" jsonschema:"Label name to match"`
	Value   string `json:"value" jsonschema:"Label value to match"`
	IsRegex bool   `json:"isRegex" jsonschema:"Whether the match is a regex match"`
	IsEqual bool   `json:"isEqual" jsonschema:"Whether the match is an equality match (true) or inequality match (false)"`
}

// Input structs for handler parameters

// ListMetricsInput defines the input parameters for ListMetricsHandler.
type ListMetricsInput struct {
	NameRegex string `json:"name_regex"`
}

// RangeQueryInput defines the input parameters for ExecuteRangeQueryHandler.
type RangeQueryInput struct {
	Query    string `json:"query"`
	Step     string `json:"step"`
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
	Duration string `json:"duration,omitempty"`
}

// InstantQueryInput defines the input parameters for ExecuteInstantQueryHandler.
type InstantQueryInput struct {
	Query string `json:"query"`
	Time  string `json:"time,omitempty"`
}

// LabelNamesInput defines the input parameters for GetLabelNamesHandler.
type LabelNamesInput struct {
	Metric string `json:"metric,omitempty"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
}

// LabelValuesInput defines the input parameters for GetLabelValuesHandler.
type LabelValuesInput struct {
	Label  string `json:"label"`
	Metric string `json:"metric,omitempty"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
}

// SeriesInput defines the input parameters for GetSeriesHandler.
type SeriesInput struct {
	Matches string `json:"matches"`
	Start   string `json:"start,omitempty"`
	End     string `json:"end,omitempty"`
}

// AlertsInput defines the input parameters for GetAlertsHandler.
type AlertsInput struct {
	Active      *bool  `json:"active,omitempty"`
	Silenced    *bool  `json:"silenced,omitempty"`
	Inhibited   *bool  `json:"inhibited,omitempty"`
	Unprocessed *bool  `json:"unprocessed,omitempty"`
	Filter      string `json:"filter,omitempty"`
	Receiver    string `json:"receiver,omitempty"`
}

// SilencesInput defines the input parameters for GetSilencesHandler.
type SilencesInput struct {
	Filter string `json:"filter,omitempty"`
}
