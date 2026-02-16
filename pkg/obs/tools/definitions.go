package tools

// All tool definitions as a single source of truth
var (
	ListMetrics = ToolDef{
		Name:        "list_metrics",
		Description: ListMetricsPrompt,
		Title:       "List Available Metrics",
		Params: []ParamDef{
			{
				Name:        "name_regex",
				Type:        ParamTypeString,
				Description: "Regex pattern to filter metric names (e.g., 'http_.*', 'node_.*', 'kube.*'). This parameter is required. Don't pass in blanket regex.",
				Required:    true,
			},
		},
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
	}

	ExecuteInstantQuery = ToolDef{
		Name:        "execute_instant_query",
		Description: ExecuteInstantQueryPrompt,
		Title:       "Execute Instant Query",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []ParamDef{
			{
				Name:        "query",
				Type:        ParamTypeString,
				Description: "PromQL query string using metric names verified via list_metrics",
				Required:    true,
			},
			{
				Name:        "time",
				Type:        ParamTypeString,
				Description: "Evaluation time as RFC3339 or Unix timestamp. Omit or use 'NOW' for current time.",
				Required:    false,
			},
		},
	}

	ExecuteRangeQuery = ToolDef{
		Name:        "execute_range_query",
		Description: ExecuteRangeQueryPrompt,
		Title:       "Execute Range Query",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []ParamDef{
			{
				Name:        "query",
				Type:        ParamTypeString,
				Description: "PromQL query string using metric names verified via list_metrics",
				Required:    true,
			},
			{
				Name:        "step",
				Type:        ParamTypeString,
				Description: "Query resolution step width (e.g., '15s', '1m', '1h'). Choose based on time range: shorter ranges use smaller steps.",
				Required:    true,
				Pattern:     `^\d+[smhdwy]$`,
			},
			{
				Name:        "start",
				Type:        ParamTypeString,
				Description: "Start time as RFC3339 or Unix timestamp (optional)",
				Required:    false,
			},
			{
				Name:        "end",
				Type:        ParamTypeString,
				Description: "End time as RFC3339 or Unix timestamp (optional). Use `NOW` for current time.",
				Required:    false,
			},
			{
				Name:        "duration",
				Type:        ParamTypeString,
				Description: "Duration to look back from now (e.g., '1h', '30m', '1d', '2w') (optional)",
				Required:    false,
				Pattern:     `^\d+[smhdwy]$`,
			},
		},
	}

	GetLabelNames = ToolDef{
		Name:        "get_label_names",
		Description: GetLabelNamesPrompt,
		Title:       "Get Label Names",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []ParamDef{
			{
				Name:        "metric",
				Type:        ParamTypeString,
				Description: "Metric name (from list_metrics) to get label names for. Leave empty for all metrics.",
				Required:    false,
			},
			{
				Name:        "start",
				Type:        ParamTypeString,
				Description: "Start time for label discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)",
				Required:    false,
			},
			{
				Name:        "end",
				Type:        ParamTypeString,
				Description: "End time for label discovery as RFC3339 or Unix timestamp (optional, defaults to now)",
				Required:    false,
			},
		},
	}

	GetLabelValues = ToolDef{
		Name:        "get_label_values",
		Description: GetLabelValuesPrompt,
		Title:       "Get Label Values",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []ParamDef{
			{
				Name:        "label",
				Type:        ParamTypeString,
				Description: "Label name (from get_label_names) to get values for",
				Required:    true,
			},
			{
				Name:        "metric",
				Type:        ParamTypeString,
				Description: "Metric name (from list_metrics) to scope the label values to. Leave empty for all metrics.",
				Required:    false,
			},
			{
				Name:        "start",
				Type:        ParamTypeString,
				Description: "Start time for label value discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)",
				Required:    false,
			},
			{
				Name:        "end",
				Type:        ParamTypeString,
				Description: "End time for label value discovery as RFC3339 or Unix timestamp (optional, defaults to now)",
				Required:    false,
			},
		},
	}

	GetSeries = ToolDef{
		Name:        "get_series",
		Description: GetSeriesPrompt,
		Title:       "Get Series",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []ParamDef{
			{
				Name:        "matches",
				Type:        ParamTypeString,
				Description: "PromQL series selector using metric names from list_metrics",
				Required:    true,
			},
			{
				Name:        "start",
				Type:        ParamTypeString,
				Description: "Start time for series discovery as RFC3339 or Unix timestamp (optional, defaults to 1 hour ago)",
				Required:    false,
			},
			{
				Name:        "end",
				Type:        ParamTypeString,
				Description: "End time for series discovery as RFC3339 or Unix timestamp (optional, defaults to now)",
				Required:    false,
			},
		},
	}

	GetAlerts = ToolDef{
		Name:        "get_alerts",
		Description: GetAlertsPrompt,
		Title:       "Get Alerts",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []ParamDef{
			{
				Name:        "active",
				Type:        ParamTypeBoolean,
				Description: "Filter for active alerts only (true/false, optional)",
				Required:    false,
			},
			{
				Name:        "silenced",
				Type:        ParamTypeBoolean,
				Description: "Filter for silenced alerts only (true/false, optional)",
				Required:    false,
			},
			{
				Name:        "inhibited",
				Type:        ParamTypeBoolean,
				Description: "Filter for inhibited alerts only (true/false, optional)",
				Required:    false,
			},
			{
				Name:        "unprocessed",
				Type:        ParamTypeBoolean,
				Description: "Filter for unprocessed alerts only (true/false, optional)",
				Required:    false,
			},
			{
				Name:        "filter",
				Type:        ParamTypeString,
				Description: "Label matchers to filter alerts (e.g., 'alertname=HighCPU', optional)",
				Required:    false,
			},
			{
				Name:        "receiver",
				Type:        ParamTypeString,
				Description: "Receiver name to filter alerts (optional)",
				Required:    false,
			},
		},
	}

	GetSilences = ToolDef{
		Name:        "get_silences",
		Description: GetSilencesPrompt,
		Title:       "Get Silences",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []ParamDef{
			{
				Name:        "filter",
				Type:        ParamTypeString,
				Description: "Label matchers to filter silences (e.g., 'alertname=HighCPU', optional)",
				Required:    false,
			},
		},
	}
)

// AllTools returns all tool definitions
func AllTools() []ToolDef {
	return []ToolDef{
		ListMetrics,
		ExecuteInstantQuery,
		ExecuteRangeQuery,
		GetLabelNames,
		GetLabelValues,
		GetSeries,
		GetAlerts,
		GetSilences,
	}
}
