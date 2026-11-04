package alertmanagement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFlattenListedAlerts(t *testing.T) {
	activeAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := flattenListedAlerts([]prometheusAlert{
		{
			Labels: map[string]string{
				"alertname":         "Watchdog",
				"severity":          "none",
				"namespace":         "openshift-monitoring",
				alertSourceLabel:    "platform",
				managedClusterLabel: "cluster-a",
			},
			Annotations:    map[string]string{"summary": "ok"},
			State:          "firing",
			ActiveAt:       &activeAt,
			Value:          "1",
			AlertRuleID:    "id-1",
			AlertComponent: "monitoring",
			AlertLayer:     "core",
		},
		{
			Labels: map[string]string{
				"alertname":      "UserAlert",
				alertRuleIDLabel: "id-from-label",
			},
			State: "pending",
		},
	})
	require.Len(t, got, 2)
	require.Equal(t, "Watchdog", got[0].Name)
	require.Equal(t, "id-1", got[0].RuleID)
	require.Equal(t, "cluster-a", got[0].Cluster)
	require.Equal(t, "2026-01-02T03:04:05Z", got[0].ActiveAt)
	require.Equal(t, "id-from-label", got[1].RuleID)
}

func TestFlattenListedRules(t *testing.T) {
	groups := []prometheusRuleGroup{
		{
			Name: "general",
			Rules: []prometheusRule{
				{
					Name:  "record_cpu",
					Query: "sum(cpu)",
					Type:  "recording",
				},
				{
					Name:  "Watchdog",
					Query: "vector(1)",
					Type:  "alerting",
					Labels: map[string]string{
						alertRuleIDLabel:   "id-1",
						"severity":         "none",
						"namespace":        "openshift-monitoring",
						alertSourceLabel:   "platform",
						ruleManagedByLabel: "operator",
					},
				},
				{
					Name:  "UserAlert",
					Query: "up == 0",
					Type:  "alerting",
					Labels: map[string]string{
						alertRuleIDLabel: "id-2",
						"severity":       "critical",
						"namespace":      "app",
						alertSourceLabel: alertSourceUser,
					},
				},
			},
		},
	}

	got := flattenListedRules(groups)
	require.Len(t, got, 2)
	require.Equal(t, "id-1", got[0].ID)
	require.Equal(t, "operator", got[0].ManagementStatus)
	require.Equal(t, "id-2", got[1].ID)
	require.Equal(t, managementUserCreated, got[1].ManagementStatus)
	require.Equal(t, "critical", got[1].Severity)
}

func TestFlattenListedRules_ClusterLabel(t *testing.T) {
	got := flattenListedRules([]prometheusRuleGroup{{
		Rules: []prometheusRule{{
			Name: "NamespacedAlert",
			Type: "alerting",
			Labels: map[string]string{
				alertRuleIDLabel:    "id-c",
				alertSourceLabel:    alertSourceUser,
				managedClusterLabel: "cluster-a",
				clusterQueryKey:     "ignored-when-managed-cluster-set",
			},
		}},
	}})
	require.Len(t, got, 1)
	require.Equal(t, managementUserCreated, got[0].ManagementStatus)
	require.Equal(t, "cluster-a", got[0].Cluster)
}

func TestFlattenListedRules_GitOpsLabel(t *testing.T) {
	got := flattenListedRules([]prometheusRuleGroup{{
		Rules: []prometheusRule{{
			Name: "GitOpsAlert",
			Type: "alerting",
			Labels: map[string]string{
				alertRuleIDLabel:   "id-g",
				alertSourceLabel:   alertSourceUser,
				ruleManagedByLabel: managementGitOps,
			},
		}},
	}})
	require.Len(t, got, 1)
	require.Equal(t, managementGitOps, got[0].ManagementStatus)
}

func TestParseCreateInput(t *testing.T) {
	_, err := parseCreateInput(map[string]any{"alert": "A"})
	require.EqualError(t, err, "expr is required")

	_, err = parseCreateInput(map[string]any{"alert": "A", "expr": "up", "severity": "fatal"})
	require.EqualError(t, err, `invalid severity "fatal": must be one of critical, warning, info, none`)

	_, err = parseCreateInput(map[string]any{
		"alert":                "A",
		"expr":                 "up",
		"prometheus_rule_name": "pr",
	})
	require.EqualError(t, err, "namespace is required when prometheus_rule_name is set")

	_, err = parseCreateInput(map[string]any{
		"alert":   "A",
		"expr":    "up",
		"cluster": "cluster-a",
	})
	require.EqualError(t, err, errClusterWriteUnsupported)
}

func TestParseListInputClusterScope(t *testing.T) {
	in, err := parseListInput(map[string]any{
		"source":  "user",
		"cluster": "cluster-a",
	}, validRuleStates)
	require.NoError(t, err)
	q := in.query()
	require.Equal(t, "cluster-a", q.Get(clusterQueryKey))
	require.Empty(t, q.Get(clusterLabelsQueryKey))
	require.Equal(t, "user", q.Get(alertSourceLabel))

	_, err = parseListInput(map[string]any{
		"labels": map[string]any{clusterQueryKey: "cluster-a"},
	}, validRuleStates)
	require.EqualError(t, err, "cluster must be set via the cluster parameter, not labels")

	_, err = parseListInput(map[string]any{
		"cluster_labels": map[string]any{"env": "prod", "region": "west"},
	}, validRuleStates)
	require.EqualError(t, err, "cluster_labels is not supported by the management API; use labels for alert or rule label filters")
}

func TestListQueryDedicatedFiltersOverrideLabels(t *testing.T) {
	in, err := parseListInput(map[string]any{
		"severity":  "warning",
		"namespace": "app",
		"state":     "firing",
		"source":    "user",
		"labels": map[string]any{
			"severity":       "critical",
			"namespace":      "other",
			"state":          "pending",
			alertSourceLabel: "platform",
			"foo":            "bar",
		},
	}, validRuleStates)
	require.NoError(t, err)
	q := in.query()
	require.Equal(t, "warning", q.Get("severity"))
	require.Equal(t, "app", q.Get("namespace"))
	require.Equal(t, "firing", q.Get("state"))
	require.Equal(t, "user", q.Get(alertSourceLabel))
	require.Equal(t, "bar", q.Get("foo"))
}

func TestParseMutationInputRejectsDropWithLabels(t *testing.T) {
	_, err := parseMutationInput(map[string]any{
		"rule_id":               "id-1",
		"alerting_rule_enabled": false,
		"severity":              "warning",
	}, true)
	require.EqualError(t, err, "alerting_rule_enabled cannot be combined with labels, severity, or classification")
}

func TestParseMutationInputRejectsOverlappingLabelMutations(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "severity and labels_to_remove severity",
			args: map[string]any{
				"rule_id":          "id-1",
				"severity":         "warning",
				"labels_to_remove": []any{"severity"},
			},
			want: `cannot set and remove label "severity" in the same request`,
		},
		{
			name: "labels and labels_to_remove same key",
			args: map[string]any{
				"rule_id":          "id-1",
				"labels":           map[string]any{"foo": "bar"},
				"labels_to_remove": []any{"foo"},
			},
			want: `cannot set and remove label "foo" in the same request`,
		},
		{
			name: "labels severity and labels_to_remove severity",
			args: map[string]any{
				"rule_id":          "id-1",
				"labels":           map[string]any{"severity": "warning"},
				"labels_to_remove": []any{"severity"},
			},
			want: `cannot set and remove label "severity" in the same request`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseMutationInput(tt.args, true)
			require.EqualError(t, err, tt.want)
		})
	}

	in, err := parseMutationInput(map[string]any{
		"rule_id":          "id-1",
		"severity":         "warning",
		"labels_to_remove": []any{"noise"},
	}, true)
	require.NoError(t, err)
	require.Equal(t, []string{"noise"}, in.LabelsToRemove)
	require.Equal(t, "warning", in.Severity)
}

func TestParseAPIError(t *testing.T) {
	err := parseAPIError(409, []byte(`{"error":"rule already exists"}`))
	require.EqualError(t, err, "management API returned HTTP 409: rule already exists")
}

func TestBuildCreateRequest_DedicatedNamespaceOverridesLabel(t *testing.T) {
	req := buildCreateRequest(&createInput{
		Alert:     "HighErrorRate",
		Expr:      "up == 0",
		Namespace: "team-a",
		Labels:    map[string]string{"namespace": "team-b", "foo": "bar"},
	})
	require.Equal(t, "team-a", req.AlertingRule.Labels["namespace"])
	require.Equal(t, "bar", req.AlertingRule.Labels["foo"])
}
