package alertmanagement

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"

	"github.com/containers/kubernetes-mcp-server/pkg/obs/auth"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/instrumentation"
)

func listAlertsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	in, err := parseListInput(toolArgs(params.Request.GetArguments()), validAlertStates)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if len(in.Matchers) > 0 {
		return api.NewToolCallResult("", fmt.Errorf("matchers is supported only by list_alert_rules; use labels for list_alerts")), nil
	}
	client, err := getManagementClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	resp, err := client.listAlerts(params.Context, in.query())
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return api.NewToolCallResultStructured(ListAlertsOutput{
		Alerts:   flattenListedAlerts(resp.Data.Alerts),
		Warnings: resp.Warnings,
	}, nil), nil
}

func listAlertRulesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	in, err := parseListInput(toolArgs(params.Request.GetArguments()), validRuleStates)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	client, err := getManagementClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	resp, err := client.listRules(params.Context, in.query())
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return api.NewToolCallResultStructured(ListAlertRulesOutput{
		Rules:    flattenListedRules(resp.Data.Groups),
		Warnings: resp.Warnings,
	}, nil), nil
}

func createAlertRuleHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	in, err := parseCreateInput(toolArgs(params.Request.GetArguments()))
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	client, err := getManagementClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	resp, err := client.createRule(params.Context, buildCreateRequest(in))
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return api.NewToolCallResultStructured(CreateAlertRuleOutput(resp), nil), nil
}

func updateAlertRuleHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	in, err := parseMutationInput(toolArgs(params.Request.GetArguments()), true)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	client, err := getManagementClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	resp, err := client.updateRules(params.Context, buildUpdateRequest(in))
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return api.NewToolCallResultStructured(UpdateAlertRuleOutput{Rules: mutationResults(resp.Rules)}, nil), nil
}

func deleteAlertRulesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	ids, err := parseRuleIDs(toolArgs(params.Request.GetArguments()))
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	client, err := getManagementClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	resp, err := client.deleteRules(params.Context, ids)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return api.NewToolCallResultStructured(DeleteAlertRulesOutput{Rules: mutationResults(resp.Rules)}, nil), nil
}

func previewAlertRuleHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	args := toolArgs(params.Request.GetArguments())
	body, err := buildPreviewRequest(args)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	client, err := getManagementClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	resp, err := client.previewRule(params.Context, body)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	out, err := previewOutput(resp)
	return api.NewToolCallResultStructured(out, err), nil
}

func getManagementClient(params api.ToolHandlerParams) (*managementClient, error) {
	cfg := GetConfig(params)
	if cfg == nil || strings.TrimSpace(cfg.ManagementAPIURL) == "" {
		return nil, fmt.Errorf("alert management API URL is not configured; set --alert-mgmt-api-url or ALERT_MGMT_API_URL")
	}

	baseURL := strings.TrimSpace(cfg.ManagementAPIURL)
	useTLS := strings.HasPrefix(baseURL, "https://")
	rt, err := auth.BuildRoundTripper(params.Context, params.Config, params.RESTConfig(), cfg.GetAuthMode(), useTLS, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create round tripper: %w", err)
	}
	rt = instrumentation.RoundTripper(rt, cfg.ClientMetrics, clientMetricName)

	return newManagementClient(auth.NewHTTPClient(rt, requestTimeout), baseURL), nil
}

func buildCreateRequest(in *createInput) createAPIRequest {
	labels := cloneStringMap(in.Labels)
	if in.Severity != "" {
		if labels == nil {
			labels = map[string]string{}
		}
		labels["severity"] = in.Severity
	}
	if in.PrometheusRuleName == "" && in.Namespace != "" {
		if labels == nil {
			labels = map[string]string{}
		}
		labels["namespace"] = in.Namespace
	}

	alert := in.Alert
	expr := in.Expr
	spec := &alertRuleSpecAPI{
		Alert:       &alert,
		Expr:        &expr,
		Labels:      labels,
		Annotations: in.Annotations,
	}
	if in.For != "" {
		forDuration := in.For
		spec.For = &forDuration
	}

	req := createAPIRequest{AlertingRule: spec}
	if in.PrometheusRuleName != "" {
		target := &prometheusRuleTarget{
			PrometheusRuleName:      in.PrometheusRuleName,
			PrometheusRuleNamespace: in.Namespace,
		}
		if in.GroupName != "" {
			groupName := in.GroupName
			target.GroupName = &groupName
		}
		req.PrometheusRule = target
	}
	return req
}

func buildUpdateRequest(in *mutationInput) map[string]any {
	body := map[string]any{
		"ruleIds": in.RuleIDs,
	}
	if in.AlertingRuleEnabled != nil {
		body["alertingRuleEnabled"] = *in.AlertingRuleEnabled
		return body
	}
	if labels := mutationLabels(in); len(labels) > 0 {
		body["labels"] = labels
	}
	if len(in.Classification) > 0 {
		body["classification"] = in.Classification
	}
	return body
}

func mutationLabels(in *mutationInput) map[string]*string {
	if len(in.Labels) == 0 && len(in.LabelsToRemove) == 0 && in.Severity == "" {
		return nil
	}
	out := make(map[string]*string, len(in.Labels)+len(in.LabelsToRemove)+1)
	for key, value := range in.Labels {
		v := value
		out[key] = &v
	}
	if in.Severity != "" {
		v := in.Severity
		out["severity"] = &v
	}
	for _, key := range in.LabelsToRemove {
		out[key] = nil
	}
	return out
}

func buildPreviewRequest(args map[string]any) (map[string]any, error) {
	if ids, err := stringSliceArg(args, "rule_ids"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		return nil, fmt.Errorf("preview_alert_rule accepts a single rule_id, not rule_ids")
	}

	ruleID := stringArg(args, "rule_id")
	if ruleID != "" {
		in, err := parseMutationInput(args, true)
		if err != nil {
			return nil, err
		}
		body := map[string]any{"ruleId": ruleID}
		if in.AlertingRuleEnabled != nil {
			body["alertingRuleEnabled"] = *in.AlertingRuleEnabled
			return body, nil
		}
		if labels := mutationLabels(in); len(labels) > 0 {
			body["labels"] = labels
		}
		if len(in.Classification) > 0 {
			body["classification"] = in.Classification
		}
		return body, nil
	}

	in, err := parseCreateInput(args)
	if err != nil {
		return nil, err
	}
	req := buildCreateRequest(in)
	body := map[string]any{
		"alertingRule": req.AlertingRule,
	}
	if req.PrometheusRule != nil {
		body["prometheusRule"] = req.PrometheusRule
	}
	return body, nil
}

func previewOutput(resp map[string]any) (PreviewAlertRuleOutput, error) {
	out := PreviewAlertRuleOutput{}
	if writable, ok := resp["writable"].(bool); ok {
		out.Writable = writable
	}
	if managedBy, ok := resp["managedBy"].(string); ok {
		out.ManagedBy = managedBy
	}
	if desired, ok := resp["desiredRule"].(map[string]any); ok {
		out.DesiredRule = desired
	}
	if resources, ok := resp["resources"]; ok {
		raw, err := json.Marshal(resources)
		if err != nil {
			return out, fmt.Errorf("encode preview resources: %w", err)
		}
		if err := json.Unmarshal(raw, &out.Resources); err != nil {
			return out, fmt.Errorf("decode preview resources: %w", err)
		}
	}
	if out.ManagedBy == managementGitOps {
		out.GitApplyHint = gitApplyHint
	}
	return out, nil
}

func mutationResults(in []ruleMutationAPIResult) []RuleMutationResult {
	out := make([]RuleMutationResult, 0, len(in))
	for _, item := range in {
		result := RuleMutationResult{
			ID:         item.ID,
			StatusCode: item.StatusCode,
		}
		if item.Message != nil {
			result.Message = *item.Message
		}
		out = append(out, result)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
