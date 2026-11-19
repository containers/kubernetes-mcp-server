package alertmanagement

import (
	"strings"
	"testing"
)

func TestServerPromptRequiresDisambiguationAndConfirmation(t *testing.T) {
	checks := []string{
		"same expression",
		"Do not pass every matching id",
		"wait until the user explicitly agrees",
		"does not open pull requests",
		"git or GitHub MCP",
		"the operator-owned rule cannot be edited",
		"mute, silence, or stop paging",
		"Do not default to either, and do not invent a PrometheusRule name",
		"Omitting prometheus_rule_name creates a platform",
		"the HTTP API supports labels/severity/classification",
		"Cannot change expr, alert name, for, or annotations",
		"platform only (ARC Drop)",
		"writable AlertingRule",
		"list_alerts returns firing, pending, or silenced instances",
		"List silences with get_silences",
		"per-rule statusCode",
		"Do not put secrets",
		"Do not retry 400, 404, or 413",
		"list_metrics",
		"alerting_rule_enabled=false",
		"Write tools are registered only when read-only mode is off",
		"Tell the user writes are disabled",
		"--read-only=false",
	}
	for _, want := range checks {
		if !strings.Contains(ServerPrompt, want) {
			t.Errorf("ServerPrompt missing %q", want)
		}
	}
}

func TestWriteToolPromptsCoverCapabilityLimits(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want []string
	}{
		{
			name: "create",
			got:  createAlertRulePrompt,
			want: []string{
				"platform-managed PrometheusRule",
				"Do not invent a PrometheusRule name",
				"list_metrics",
				"Do not put secrets",
			},
		},
		{
			name: "update",
			got:  updateAlertRulePrompt,
			want: []string{
				"The HTTP API does not accept expr, alert name, for, or annotations",
				"Disable/enable: alerting_rule_enabled false/true is platform only",
				"Classification is platform-only",
				"create_silence",
				"returned id",
				"cannot be edited",
			},
		},
		{
			name: "delete",
			got:  deleteAlertRulesPrompt,
			want: []string{
				"writable AlertingRule",
				"CMO/operator-managed platform",
				"mute notifications",
				"GitOps: never delete",
				"per-rule statusCode",
			},
		},
		{
			name: "list rules",
			got:  listAlertRulesPrompt,
			want: []string{
				"not firing instances",
				"do not invent an id",
				"list_alerts",
			},
		},
		{
			name: "list alerts",
			got:  listAlertsPrompt,
			want: []string{
				"rule_id",
				"list_alert_rules",
			},
		},
		{
			name: "preview",
			got:  previewAlertRulePrompt,
			want: []string{
				"gitApplyHint",
				"cannot be edited",
				"desiredObject",
				"writes are disabled",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range tt.want {
				if !strings.Contains(tt.got, want) {
					t.Errorf("missing %q", want)
				}
			}
		})
	}
}
