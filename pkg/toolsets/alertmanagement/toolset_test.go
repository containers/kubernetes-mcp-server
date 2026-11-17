package alertmanagement

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/require"
)

func TestToolsetIncludesListAlerts(t *testing.T) {
	names := map[string]struct{}{}
	for _, st := range (&Toolset{}).GetTools(t.Context(), api.ToolsetContext{}) {
		names[st.Tool.Name] = struct{}{}
	}
	for _, want := range []string{
		"list_alerts",
		"list_alert_rules",
		"preview_alert_rule",
		"create_alert_rule",
		"update_alert_rule",
		"delete_alert_rules",
	} {
		if _, ok := names[want]; !ok {
			t.Errorf("missing tool %s", want)
		}
	}
}

func TestListSchemasMatchManagementAPI(t *testing.T) {
	alerts := initListAlerts().Tool.InputSchema.Properties
	rules := initListAlertRules().Tool.InputSchema.Properties
	require.Equal(t, []any{"pending", "firing", "silenced"}, alerts["state"].Enum)
	require.Equal(t, []any{"inactive", "pending", "firing"}, rules["state"].Enum)
	require.NotContains(t, alerts, "matchers", "the alerts API does not apply match[]")
	require.Contains(t, rules, "matchers")
}

func TestClassificationSchemasAllowClearingOverrides(t *testing.T) {
	for _, tool := range []api.ServerTool{initUpdateAlertRule(), initPreviewAlertRule()} {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			schema, err := tool.Tool.InputSchema.Resolve(nil)
			require.NoError(t, err)
			for _, field := range []string{"classification_component", "classification_layer", "classification_component_from", "classification_layer_from"} {
				args := map[string]any{"rule_id": "rule-1", field: nil}
				require.NoError(t, schema.Validate(args), "explicit null must clear %s", field)
			}
		})
	}
}
