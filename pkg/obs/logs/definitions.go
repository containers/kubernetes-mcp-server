package logs

import "github.com/rhobs/obs-mcp/pkg/tools"

var (
	lokiNamespaceParameter = tools.ParamDef{
		Name:        "lokiNamespace",
		Type:        tools.ParamTypeString,
		Description: "Kubernetes namespace of the LokiStack. Use loki_list_instances to discover valid values.",
		Required:    false,
	}
	lokiNameParameter = tools.ParamDef{
		Name:        "lokiName",
		Type:        tools.ParamTypeString,
		Description: "Name of the LokiStack. Use loki_list_instances to discover valid values.",
		Required:    false,
	}
	tenantParameter = tools.ParamDef{
		Name:        "tenant",
		Type:        tools.ParamTypeString,
		Description: "Loki tenant ID (X-Scope-OrgID). For LokiStack gateway modes (e.g. openshift-network) this selects the `/api/logs/v1/<tenant>` path; use `network` for openshift-network.",
		Required:    false,
	}
)

var (
	ListInstancesTool = tools.ToolDef[ListInstancesOutput]{
		Name: "loki_list_instances",
		Description: `List LokiStack instances available in the Kubernetes cluster.
Call this first when using Loki Operator managed stacks so you can pass lokiNamespace and lokiName to other Loki tools.`,
		Title:       "List LokiStack Instances",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
	}

	LabelNamesTool = tools.ToolDef[LabelNamesOutput]{
		Name:        "loki_label_names",
		Description: lokiLabelNamesPrompt,
		Title:       "List Loki Label Names",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []tools.ParamDef{
			lokiNamespaceParameter,
			lokiNameParameter,
			tenantParameter,
			{
				Name:        "start",
				Type:        tools.ParamTypeString,
				Description: "Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
				Required:    false,
			},
			{
				Name:        "end",
				Type:        tools.ParamTypeString,
				Description: "End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
				Required:    false,
			},
		},
	}

	LabelValuesTool = tools.ToolDef[LabelValuesOutput]{
		Name:        "loki_label_values",
		Description: lokiLabelValuesPrompt,
		Title:       "List Loki Label Values",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []tools.ParamDef{
			lokiNamespaceParameter,
			lokiNameParameter,
			tenantParameter,
			{
				Name:        "label",
				Type:        tools.ParamTypeString,
				Description: "Label key to inspect (for example namespace, pod, container).",
				Required:    true,
			},
			{
				Name:        "start",
				Type:        tools.ParamTypeString,
				Description: "Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
				Required:    false,
			},
			{
				Name:        "end",
				Type:        tools.ParamTypeString,
				Description: "End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
				Required:    false,
			},
		},
	}

	QueryRangeTool = tools.ToolDef[QueryRangeOutput]{
		Name:        "loki_query_range",
		Description: lokiQueryRangePrompt,
		Title:       "Execute Loki Range Query",
		ReadOnly:    true,
		Destructive: false,
		Idempotent:  true,
		OpenWorld:   true,
		Params: []tools.ParamDef{
			lokiNamespaceParameter,
			lokiNameParameter,
			tenantParameter,
			{
				Name:        "query",
				Type:        tools.ParamTypeString,
				Description: "LogQL query string.",
				Required:    true,
			},
			{
				Name:        "start",
				Type:        tools.ParamTypeString,
				Description: "Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
				Required:    false,
			},
			{
				Name:        "end",
				Type:        tools.ParamTypeString,
				Description: "End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
				Required:    false,
			},
			{
				Name:        "duration",
				Type:        tools.ParamTypeString,
				Description: "Lookback duration from now when start/end are omitted (for example 5m, 1h). Defaults to 15m.",
				Required:    false,
				Pattern:     `^\d+[smhdwy]$`,
			},
			{
				Name:        "limit",
				Type:        tools.ParamTypeNumber,
				Description: "Maximum number of log lines to return. Defaults to 100, max 1000.",
				Required:    false,
			},
			{
				Name:        "direction",
				Type:        tools.ParamTypeString,
				Description: "Search direction: backward (default) or forward.",
				Required:    false,
			},
		},
	}
)

func AllTools() []tools.ToolDefInterface {
	return []tools.ToolDefInterface{
		ListInstancesTool,
		LabelNamesTool,
		LabelValuesTool,
		QueryRangeTool,
	}
}
