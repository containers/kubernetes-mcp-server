package logs

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
)

var (
	lokiNamespaceSchema = &jsonschema.Schema{
		Type:        "string",
		Description: "Kubernetes namespace of the LokiStack. Use loki_list_instances to discover valid values.",
	}
	lokiNameSchema = &jsonschema.Schema{
		Type:        "string",
		Description: "Name of the LokiStack. Use loki_list_instances to discover valid values.",
	}
	tenantSchema = &jsonschema.Schema{
		Type:        "string",
		Description: "Loki tenant ID (X-Scope-OrgID). For LokiStack gateway modes (e.g. openshift-network) this selects the `/api/logs/v1/<tenant>` path; use `network` for openshift-network.",
	}
)

func initListInstances() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name: "loki_list_instances",
			Description: `List LokiStack instances available in the Kubernetes cluster.
Call this first when using Loki Operator managed stacks so you can pass lokiNamespace and lokiName to other Loki tools.`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
			},
			OutputSchema: listInstancesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "List LokiStack Instances",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: listInstancesHandler,
	}
}

func initLabelNames() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "loki_label_names",
			Description: lokiLabelNamesPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"lokiNamespace": lokiNamespaceSchema,
					"lokiName":      lokiNameSchema,
					"tenant":        tenantSchema,
					"start": {
						Type:        "string",
						Description: "Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
					},
					"end": {
						Type:        "string",
						Description: "End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
					},
				},
			},
			OutputSchema: labelNamesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "List Loki Label Names",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: labelNamesHandler,
	}
}

func initLabelValues() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "loki_label_values",
			Description: lokiLabelValuesPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"lokiNamespace": lokiNamespaceSchema,
					"lokiName":      lokiNameSchema,
					"tenant":        tenantSchema,
					"label": {
						Type:        "string",
						Description: "Label key to inspect (for example namespace, pod, container).",
					},
					"start": {
						Type:        "string",
						Description: "Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
					},
					"end": {
						Type:        "string",
						Description: "End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
					},
				},
				Required: []string{"label"},
			},
			OutputSchema: labelValuesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "List Loki Label Values",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: labelValuesHandler,
	}
}

func initQueryRange() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "loki_query_range",
			Description: lokiQueryRangePrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"lokiNamespace": lokiNamespaceSchema,
					"lokiName":      lokiNameSchema,
					"tenant":        tenantSchema,
					"query": {
						Type:        "string",
						Description: "LogQL query string.",
					},
					"start": {
						Type:        "string",
						Description: "Start time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
					},
					"end": {
						Type:        "string",
						Description: "End time as RFC3339, Unix timestamp, NOW, or NOW-relative expression (optional).",
					},
					"duration": {
						Type:        "string",
						Description: "Lookback duration from now when start/end are omitted (for example 5m, 1h). Defaults to 15m.",
						Pattern:     `^\d+[smhdwy]$`,
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of log lines to return. Defaults to 100, max 1000.",
					},
					"direction": {
						Type:        "string",
						Description: "Search direction: backward (default) or forward.",
					},
				},
				Required: []string{"query"},
			},
			OutputSchema: queryRangeOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Execute Loki Range Query",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: queryRangeHandler,
	}
}
