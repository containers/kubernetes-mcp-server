package alertmanagement

import "time"

func flattenListedAlerts(alerts []prometheusAlert) []ListedAlert {
	out := make([]ListedAlert, 0, len(alerts))
	for _, alert := range alerts {
		labels := alert.Labels
		listed := ListedAlert{
			Name:        labels["alertname"],
			State:       alert.State,
			Severity:    labels["severity"],
			Namespace:   labels["namespace"],
			Cluster:     ruleCluster(labels),
			Source:      labels[alertSourceLabel],
			RuleID:      alert.AlertRuleID,
			Component:   alert.AlertComponent,
			Layer:       alert.AlertLayer,
			Value:       alert.Value,
			Labels:      labels,
			Annotations: alert.Annotations,
		}
		if listed.RuleID == "" {
			listed.RuleID = labels[alertRuleIDLabel]
		}
		if alert.ActiveAt != nil && !alert.ActiveAt.IsZero() {
			listed.ActiveAt = alert.ActiveAt.UTC().Format(time.RFC3339)
		}
		out = append(out, listed)
	}
	return out
}

func flattenListedRules(groups []prometheusRuleGroup) []ListedAlertRule {
	out := make([]ListedAlertRule, 0)
	for _, group := range groups {
		for _, rule := range group.Rules {
			if rule.Type == "recording" {
				continue
			}
			labels := rule.Labels
			listed := ListedAlertRule{
				ID:               labels[alertRuleIDLabel],
				Name:             rule.Name,
				Expr:             rule.Query,
				Severity:         labels["severity"],
				Namespace:        labels["namespace"],
				Cluster:          ruleCluster(labels),
				Source:           labels[alertSourceLabel],
				ManagementStatus: managementStatus(labels),
				Group:            group.Name,
				Type:             rule.Type,
				Health:           rule.Health,
				Labels:           labels,
				Annotations:      rule.Annotations,
			}
			out = append(out, listed)
		}
	}
	return out
}

func managementStatus(labels map[string]string) string {
	if managedBy := labels[ruleManagedByLabel]; managedBy != "" {
		return managedBy
	}
	switch labels[alertSourceLabel] {
	case "", alertSourcePlatform:
		return managementOperator
	default:
		return managementUserCreated
	}
}

func ruleCluster(labels map[string]string) string {
	if cluster := labels[managedClusterLabel]; cluster != "" {
		return cluster
	}
	return labels[clusterQueryKey]
}
