package alertmanagement

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"

	"github.com/containers/kubernetes-mcp-server/pkg/obs/tools"
)

var (
	listAlertsOutputSchema       = tools.MustSchema[ListAlertsOutput]()
	listAlertRulesOutputSchema   = tools.MustSchema[ListAlertRulesOutput]()
	createAlertRuleOutputSchema  = tools.MustSchema[CreateAlertRuleOutput]()
	updateAlertRuleOutputSchema  = tools.MustSchema[UpdateAlertRuleOutput]()
	deleteAlertRulesOutputSchema = tools.MustSchema[DeleteAlertRulesOutput]()
	previewAlertRuleOutputSchema = tools.MustSchema[PreviewAlertRuleOutput]()
)

func initListAlerts() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "list_alerts",
			Description: listAlertsPrompt,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: listFilterProperties(false),
			},
			OutputSchema: listAlertsOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "List Alerts",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      listAlertsHandler,
		ClusterAware: new(false),
		RBAC:         rbacNoKubernetes(),
	}
}

func initListAlertRules() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "list_alert_rules",
			Description: listAlertRulesPrompt,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: listFilterProperties(true),
			},
			OutputSchema: listAlertRulesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "List Alert Rules",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      listAlertRulesHandler,
		ClusterAware: new(false),
		RBAC:         rbacNoKubernetes(),
	}
}

func initCreateAlertRule() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "create_alert_rule",
			Description: createAlertRulePrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"alert": {
						Type:        "string",
						Description: "Alert name (PrometheusRule alert field).",
					},
					"expr": {
						Type:        "string",
						Description: "PromQL expression to evaluate.",
					},
					"severity": severitySchema,
					"namespace": {
						Type:        "string",
						Description: "PrometheusRule namespace for user-defined rules. Required with prometheus_rule_name. Otherwise stored as a namespace label on a platform rule.",
					},
					"prometheus_rule_name": {
						Type:        "string",
						Description: "PrometheusRule resource name for a user-defined rule. Omit to create a platform rule.",
					},
					"group_name": {
						Type:        "string",
						Description: "Optional rule group name within the PrometheusRule.",
					},
					"for": {
						Type:        "string",
						Description: "Duration the condition must be true before firing (for example 5m).",
					},
					"labels": {
						Type:                 "object",
						Description:          "Labels to attach to the rule. severity is merged from the severity parameter when set.",
						AdditionalProperties: &jsonschema.Schema{Type: "string"},
					},
					"annotations": {
						Type:                 "object",
						Description:          "Annotations to attach to alerts produced by the rule.",
						AdditionalProperties: &jsonschema.Schema{Type: "string"},
					},
					"cluster":        clusterWriteSchema,
					"cluster_labels": clusterLabelsWriteSchema,
				},
				Required: []string{"alert", "expr"},
			},
			OutputSchema: createAlertRuleOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Create Alert Rule",
				ReadOnlyHint:    new(false),
				DestructiveHint: new(false),
				IdempotentHint:  new(false),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      createAlertRuleHandler,
		ClusterAware: new(false),
		RBAC:         rbacNoKubernetes(),
	}
}

func initUpdateAlertRule() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "update_alert_rule",
			Description: updateAlertRulePrompt,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: mutationProperties(),
			},
			OutputSchema: updateAlertRuleOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Update Alert Rule",
				ReadOnlyHint:    new(false),
				DestructiveHint: new(true),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      updateAlertRuleHandler,
		ClusterAware: new(false),
		RBAC:         rbacNoKubernetes(),
	}
}

func initDeleteAlertRules() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "delete_alert_rules",
			Description: deleteAlertRulesPrompt,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"rule_id": {
						Type:        "string",
						Description: "Single stable alert rule ID to delete.",
					},
					"rule_ids": {
						Type:        "array",
						Description: "Stable alert rule IDs to delete (at most 100 combined with rule_id).",
						Items:       &jsonschema.Schema{Type: "string"},
					},
					"cluster":        clusterWriteSchema,
					"cluster_labels": clusterLabelsWriteSchema,
				},
			},
			OutputSchema: deleteAlertRulesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Delete Alert Rules",
				ReadOnlyHint:    new(false),
				DestructiveHint: new(true),
				IdempotentHint:  new(false),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      deleteAlertRulesHandler,
		ClusterAware: new(false),
		RBAC:         rbacNoKubernetes(),
	}
}

func initPreviewAlertRule() api.ServerTool {
	props := mutationProperties()
	delete(props, "rule_ids")
	props["alert"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Alert name for create preview.",
	}
	props["expr"] = &jsonschema.Schema{
		Type:        "string",
		Description: "PromQL expression for create preview.",
	}
	props["namespace"] = &jsonschema.Schema{
		Type:        "string",
		Description: "PrometheusRule namespace for user-defined create preview.",
	}
	props["prometheus_rule_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "PrometheusRule name for user-defined create preview. Omit for platform rules.",
	}
	props["group_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional rule group name for user-defined create preview.",
	}
	props["for"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Pending duration for create preview (for example 5m).",
	}
	props["annotations"] = &jsonschema.Schema{
		Type:                 "object",
		Description:          "Annotations for create preview.",
		AdditionalProperties: &jsonschema.Schema{Type: "string"},
	}

	return api.ServerTool{
		Tool: api.Tool{
			Name:        "preview_alert_rule",
			Description: previewAlertRulePrompt,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: props,
			},
			OutputSchema: previewAlertRuleOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Preview Alert Rule",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler:      previewAlertRuleHandler,
		ClusterAware: new(false),
		RBAC:         rbacNoKubernetes(),
	}
}

var severitySchema = &jsonschema.Schema{
	Type:        "string",
	Description: "Alert severity: critical, warning, info, or none.",
	Enum:        []any{"critical", "warning", "info", "none"},
}

var clusterWriteSchema = &jsonschema.Schema{
	Type:        "string",
	Description: "Not supported on this operation; omit this field.",
}

var clusterLabelsWriteSchema = &jsonschema.Schema{
	Type:                 "object",
	Description:          "Not supported on this operation; omit this field.",
	AdditionalProperties: &jsonschema.Schema{Type: "string"},
}

func listFilterProperties(forRules bool) map[string]*jsonschema.Schema {
	states := validAlertStates
	if forRules {
		states = validRuleStates
	}
	stateEnum := make([]any, len(states))
	for i, state := range states {
		stateEnum[i] = state
	}
	properties := map[string]*jsonschema.Schema{
		"namespace": {
			Type:        "string",
			Description: "Filter by namespace label (Thanos tenancy for user-workload rules).",
		},
		"severity": severitySchema,
		"state": {
			Type:        "string",
			Description: "Filter by state. Omit for all states.",
			Enum:        stateEnum,
		},
		"source": {
			Type:        "string",
			Description: "Filter by openshift_io_alert_source: platform or user.",
			Enum:        []any{"platform", "user"},
		},
		"cluster": {
			Type:        "string",
			Description: "Exact cluster label filter (query parameter cluster); does not select a remote cluster.",
		},
		"cluster_labels": clusterLabelsWriteSchema,
		"labels": {
			Type:                 "object",
			Description:          "Additional label equality filters forwarded as query parameters.",
			AdditionalProperties: &jsonschema.Schema{Type: "string"},
		},
	}
	if forRules {
		properties["matchers"] = &jsonschema.Schema{
			Type:        "array",
			Description: "Prometheus-style match[] selectors (for example severity=\"critical\").",
			Items:       &jsonschema.Schema{Type: "string"},
		}
	}
	return properties
}

func mutationProperties() map[string]*jsonschema.Schema {
	return map[string]*jsonschema.Schema{
		"rule_id": {
			Type:        "string",
			Description: "Single stable alert rule ID.",
		},
		"rule_ids": {
			Type:        "array",
			Description: "Stable alert rule IDs (at most 100 combined with rule_id).",
			Items:       &jsonschema.Schema{Type: "string"},
		},
		"severity": severitySchema,
		"labels": {
			Type:                 "object",
			Description:          "Label key/value pairs to set.",
			AdditionalProperties: &jsonschema.Schema{Type: "string"},
		},
		"labels_to_remove": {
			Type:        "array",
			Description: "Label keys to remove (sent as null values on the management API).",
			Items:       &jsonschema.Schema{Type: "string"},
		},
		"alerting_rule_enabled": {
			Type:        "boolean",
			Description: "When false, drop a platform alert rule via AlertRelabelConfig. When true, restore a previously dropped rule. Cannot be combined with labels or classification.",
		},
		"classification_component": {
			Types:       []string{"string", "null"},
			Description: "Set openshift_io_alert_rule_component. Empty or null clears the override.",
		},
		"classification_layer": {
			Types:       []string{"string", "null"},
			Description: "Set openshift_io_alert_rule_layer. Empty or null clears the override.",
		},
		"classification_component_from": {
			Types:       []string{"string", "null"},
			Description: "Set openshift_io_alert_rule_component_from. Empty or null clears the override.",
		},
		"classification_layer_from": {
			Types:       []string{"string", "null"},
			Description: "Set openshift_io_alert_rule_layer_from. Empty or null clears the override.",
		},
		"cluster":        clusterWriteSchema,
		"cluster_labels": clusterLabelsWriteSchema,
	}
}
