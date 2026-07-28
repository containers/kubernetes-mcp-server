package metrics

import (
	"maps"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"

	"github.com/rhobs/obs-mcp/pkg/tools"
)

var (
	listMetricsOutputSchema  = tools.MustSchema[ListMetricsOutput]()
	instantQueryOutputSchema = tools.MustSchema[InstantQueryOutput]()
	rangeQueryOutputSchema   = tools.MustSchema[RangeQueryOutput]()
	labelNamesOutputSchema   = tools.MustSchema[LabelNamesOutput]()
	labelValuesOutputSchema  = tools.MustSchema[LabelValuesOutput]()
	seriesOutputSchema       = tools.MustSchema[SeriesOutput]()
	alertsOutputSchema       = tools.MustSchema[AlertsOutput]()
	silencesOutputSchema     = tools.MustSchema[SilencesOutput]()
)

func initListMetrics() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "list_metrics",
			Description: ListMetricsPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name_regex": {
						Type:        "string",
						Description: "Regex pattern to filter metric names. IMPORTANT: Metric names are typically prefixed (e.g., 'prometheus_tsdb_head_series'). Use wildcards to match substrings: '.*tsdb.*' matches any metric containing 'tsdb', while 'tsdb' only matches the exact string 'tsdb'. Examples: 'http_.*' (starts with http_), '.*memory.*' (contains memory), 'node_.*' (starts with node_). This parameter is required. Don't pass in blanket regex like '.*' or '.+'.",
					},
				},
				Required: []string{"name_regex"},
			},
			OutputSchema: listMetricsOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "List Available Metrics",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      ListMetricsToolsetHandler,
		ClusterAware: new(false),
	}
}

func initExecuteInstantQuery() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "execute_instant_query",
			Description: ExecuteInstantQueryPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"query": {
						Type:        "string",
						Description: "PromQL query string using metric names verified via list_metrics",
					},
					"time": {
						Type:        "string",
						Description: "Evaluation time as RFC3339 or Unix timestamp. Omit or use 'NOW' for current time.",
					},
				},
				Required: []string{"query"},
			},
			OutputSchema: instantQueryOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Execute Instant Query",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      ExecuteInstantQueryToolsetHandler,
		ClusterAware: new(false),
	}
}

var rangeQueryParams = map[string]*jsonschema.Schema{
	"query": {
		Type:        "string",
		Description: "PromQL query string using metric names verified via list_metrics",
	},
	"step": {
		Type:        "string",
		Description: "Query resolution step width (e.g., '15s', '1m', '1h'). Choose based on time range: shorter ranges use smaller steps.",
		Pattern:     `^\d+[smhdwy]$`,
	},
	"start": {
		Type:        "string",
		Description: "Start time as RFC3339 or Unix timestamp (optional)",
	},
	"end": {
		Type:        "string",
		Description: "End time as RFC3339 or Unix timestamp (optional). Use `NOW` for current time.",
	},
	"duration": {
		Type:        "string",
		Description: "Duration to look back from now (e.g., '1h', '30m', '1d', '2w') (optional)",
		Pattern:     `^\d+[smhdwy]$`,
	},
}

func initExecuteRangeQuery() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "execute_range_query",
			Description: ExecuteRangeQueryPrompt,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: rangeQueryParams,
				Required:   []string{"query", "step"},
			},
			OutputSchema: rangeQueryOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Execute Range Query",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      ExecuteRangeQueryToolsetHandler,
		ClusterAware: new(false),
	}
}

func initShowTimeseries() api.ServerTool {
	props := make(map[string]*jsonschema.Schema, len(rangeQueryParams)+2)
	maps.Copy(props, rangeQueryParams)
	props["title"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Human-readable chart title describing what the query shows (e.g., 'API Error Rate Over Last Hour'). Displayed above the chart when provided.",
	}
	props["description"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Explanation of the chart's meaning or context (e.g., 'Shows the rate of HTTP 5xx errors per second, broken down by pod'). Displayed below the title when provided.",
	}

	return api.ServerTool{
		Tool: api.Tool{
			Name:        "show_timeseries",
			Description: ShowTimeseriesPrompt,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: props,
				Required:   []string{"query", "step"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Show Timeseries Chart",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
			Meta: map[string]any{
				"olsUi": map[string]any{
					"id": "mcp-obs/show-timeseries",
				},
			},
		},
		Handler:      ShowTimeseriesToolsetHandler,
		ClusterAware: new(false),
	}
}

func initGetLabelNames() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "get_label_names",
			Description: GetLabelNamesPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"metric": {
						Type:        "string",
						Description: "Metric name (from list_metrics) to get label names for. Leave empty for all metrics.",
					},
					"start": {
						Type:        "string",
						Description: "Start time for label discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)",
					},
					"end": {
						Type:        "string",
						Description: "End time for label discovery as RFC3339 or Unix timestamp (optional, defaults to now)",
					},
				},
			},
			OutputSchema: labelNamesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Get Label Names",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      GetLabelNamesToolsetHandler,
		ClusterAware: new(false),
	}
}

func initGetLabelValues() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "get_label_values",
			Description: GetLabelValuesPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"label": {
						Type:        "string",
						Description: "Label name (from get_label_names) to get values for",
					},
					"metric": {
						Type:        "string",
						Description: "Metric name (from list_metrics) to scope the label values to. Leave empty for all metrics.",
					},
					"start": {
						Type:        "string",
						Description: "Start time for label value discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)",
					},
					"end": {
						Type:        "string",
						Description: "End time for label value discovery as RFC3339 or Unix timestamp (optional, defaults to now)",
					},
				},
				Required: []string{"label"},
			},
			OutputSchema: labelValuesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Get Label Values",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      GetLabelValuesToolsetHandler,
		ClusterAware: new(false),
	}
}

func initGetSeries() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "get_series",
			Description: GetSeriesPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"matches": {
						Type:        "string",
						Description: "PromQL series selector using metric names from list_metrics",
					},
					"start": {
						Type:        "string",
						Description: "Start time for series discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)",
					},
					"end": {
						Type:        "string",
						Description: "End time for series discovery as RFC3339 or Unix timestamp (optional, defaults to now)",
					},
				},
				Required: []string{"matches"},
			},
			OutputSchema: seriesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Get Series",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      GetSeriesToolsetHandler,
		ClusterAware: new(false),
	}
}

func initGetAlerts() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "get_alerts",
			Description: GetAlertsPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"active": {
						Type:        "boolean",
						Description: "Filter for active alerts only (true/false, optional)",
					},
					"silenced": {
						Type:        "boolean",
						Description: "Filter for silenced alerts only (true/false, optional)",
					},
					"inhibited": {
						Type:        "boolean",
						Description: "Filter for inhibited alerts only (true/false, optional)",
					},
					"unprocessed": {
						Type:        "boolean",
						Description: "Filter for unprocessed alerts only (true/false, optional)",
					},
					"filter": {
						Type:        "string",
						Description: "Label matchers to filter alerts (e.g., 'alertname=HighCPU', optional)",
					},
					"receiver": {
						Type:        "string",
						Description: "Receiver name to filter alerts (optional)",
					},
				},
			},
			OutputSchema: alertsOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Get Alerts",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      GetAlertsToolsetHandler,
		ClusterAware: new(false),
	}
}

func initGetSilences() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "get_silences",
			Description: GetSilencesPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"filter": {
						Type:        "string",
						Description: "Label matchers to filter silences (e.g., 'alertname=HighCPU', optional)",
					},
				},
			},
			OutputSchema: silencesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Get Silences",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      GetSilencesToolsetHandler,
		ClusterAware: new(false),
	}
}
