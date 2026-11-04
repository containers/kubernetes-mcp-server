package alertmanagement

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

type createInput struct {
	Alert              string
	Expr               string
	Severity           string
	Namespace          string
	For                string
	PrometheusRuleName string
	GroupName          string
	Labels             map[string]string
	Annotations        map[string]string
}

type listInput struct {
	Namespace string
	Severity  string
	State     string
	Source    string
	Cluster   string
	Matchers  []string
	Labels    map[string]string
}

type mutationInput struct {
	RuleIDs             []string
	Severity            string
	Labels              map[string]string
	LabelsToRemove      []string
	AlertingRuleEnabled *bool
	Classification      map[string]any
}

func parseCreateInput(args map[string]any) (*createInput, error) {
	if err := rejectWriteClusterScope(args); err != nil {
		return nil, err
	}
	in := &createInput{
		Alert:              stringArg(args, "alert"),
		Expr:               stringArg(args, "expr"),
		Severity:           strings.ToLower(stringArg(args, "severity")),
		Namespace:          stringArg(args, "namespace"),
		For:                stringArg(args, "for"),
		PrometheusRuleName: stringArg(args, "prometheus_rule_name"),
		GroupName:          stringArg(args, "group_name"),
	}
	var err error
	in.Labels, err = stringMapArg(args, "labels")
	if err != nil {
		return nil, err
	}
	in.Annotations, err = stringMapArg(args, "annotations")
	if err != nil {
		return nil, err
	}
	if in.Alert == "" {
		return nil, fmt.Errorf("alert is required")
	}
	if in.Expr == "" {
		return nil, fmt.Errorf("expr is required")
	}
	if err := validateSeverity(in.Severity); err != nil {
		return nil, err
	}
	if in.PrometheusRuleName != "" && in.Namespace == "" {
		return nil, fmt.Errorf("namespace is required when prometheus_rule_name is set")
	}
	if in.GroupName != "" && in.PrometheusRuleName == "" {
		return nil, fmt.Errorf("group_name requires prometheus_rule_name")
	}
	return in, nil
}

func parseListInput(args map[string]any, states []string) (*listInput, error) {
	scope, err := parseClusterScope(args)
	if err != nil {
		return nil, err
	}
	if len(scope.ClusterLabels) > 0 {
		return nil, fmt.Errorf("cluster_labels is not supported by the management API; use labels for alert or rule label filters")
	}
	in := &listInput{
		Namespace: stringArg(args, "namespace"),
		Severity:  strings.ToLower(stringArg(args, "severity")),
		State:     strings.ToLower(stringArg(args, "state")),
		Source:    strings.ToLower(stringArg(args, "source")),
		Cluster:   scope.Cluster,
	}
	in.Matchers, err = stringSliceArg(args, "matchers")
	if err != nil {
		return nil, err
	}
	in.Labels, err = stringMapArg(args, "labels")
	if err != nil {
		return nil, err
	}
	if err := validateSeverity(in.Severity); err != nil {
		return nil, err
	}
	if in.State != "" {
		if !slices.Contains(states, in.State) {
			return nil, fmt.Errorf("invalid state %q: must be one of %s", in.State, strings.Join(states, ", "))
		}
	}
	if in.Source != "" {
		if _, ok := validSources[in.Source]; !ok {
			return nil, fmt.Errorf("invalid source %q: must be platform or user", in.Source)
		}
	}
	for key := range in.Labels {
		if _, reserved := reservedListLabelKeys[key]; reserved {
			return nil, fmt.Errorf("%s must be set via the %s parameter, not labels", key, key)
		}
	}
	return in, nil
}

func (in *listInput) query() url.Values {
	q := url.Values{}
	for key, value := range in.Labels {
		if key == "" || value == "" {
			continue
		}
		q.Set(key, value)
	}
	if in.Namespace != "" {
		q.Set("namespace", in.Namespace)
	}
	if in.Severity != "" {
		q.Set("severity", in.Severity)
	}
	if in.State != "" {
		q.Set("state", in.State)
	}
	if in.Source != "" {
		q.Set(alertSourceLabel, in.Source)
	}
	if in.Cluster != "" {
		q.Set(clusterQueryKey, in.Cluster)
	}
	for _, matcher := range in.Matchers {
		q.Add("match[]", matcher)
	}
	return q
}

type clusterScope struct {
	Cluster       string
	ClusterLabels map[string]string
}

func parseClusterScope(args map[string]any) (clusterScope, error) {
	scope := clusterScope{Cluster: stringArg(args, clusterQueryKey)}
	labels, err := stringMapArg(args, clusterLabelsQueryKey)
	if err != nil {
		return clusterScope{}, err
	}
	scope.ClusterLabels = labels
	return scope, nil
}

func (s clusterScope) empty() bool {
	return s.Cluster == "" && len(s.ClusterLabels) == 0
}

func rejectWriteClusterScope(args map[string]any) error {
	scope, err := parseClusterScope(args)
	if err != nil {
		return err
	}
	if scope.empty() {
		return nil
	}
	return errors.New(errClusterWriteUnsupported)
}

func parseMutationInput(args map[string]any, requireMutation bool) (*mutationInput, error) {
	ruleIDs, err := parseRuleIDs(args)
	if err != nil {
		return nil, err
	}
	in := &mutationInput{
		RuleIDs:  ruleIDs,
		Severity: strings.ToLower(stringArg(args, "severity")),
	}
	in.Labels, err = stringMapArg(args, "labels")
	if err != nil {
		return nil, err
	}
	in.LabelsToRemove, err = stringSliceArg(args, "labels_to_remove")
	if err != nil {
		return nil, err
	}
	in.AlertingRuleEnabled, err = boolPtrArg(args, "alerting_rule_enabled")
	if err != nil {
		return nil, err
	}
	if err := validateSeverity(in.Severity); err != nil {
		return nil, err
	}
	in.Classification, err = parseClassification(args)
	if err != nil {
		return nil, err
	}
	if err := rejectContradictoryLabelMutations(in); err != nil {
		return nil, err
	}
	hasLabels := len(in.Labels) > 0 || len(in.LabelsToRemove) > 0 || in.Severity != ""
	hasClassification := len(in.Classification) > 0
	if in.AlertingRuleEnabled != nil && (hasLabels || hasClassification) {
		return nil, fmt.Errorf("alerting_rule_enabled cannot be combined with labels, severity, or classification")
	}
	if requireMutation && in.AlertingRuleEnabled == nil && !hasLabels && !hasClassification {
		return nil, fmt.Errorf("at least one of labels, labels_to_remove, severity, classification, or alerting_rule_enabled is required")
	}
	return in, nil
}

func rejectContradictoryLabelMutations(in *mutationInput) error {
	set := make(map[string]struct{}, len(in.Labels)+1)
	for key := range in.Labels {
		set[key] = struct{}{}
	}
	if in.Severity != "" {
		set["severity"] = struct{}{}
	}
	for _, key := range in.LabelsToRemove {
		if _, ok := set[key]; ok {
			return fmt.Errorf("cannot set and remove label %q in the same request", key)
		}
	}
	return nil
}

func parseRuleIDs(args map[string]any) ([]string, error) {
	if err := rejectWriteClusterScope(args); err != nil {
		return nil, err
	}
	single := stringArg(args, "rule_id")
	ids, err := stringSliceArg(args, "rule_ids")
	if err != nil {
		return nil, err
	}
	if single != "" {
		ids = append([]string{single}, ids...)
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("rule_id or rule_ids is required")
	}
	if len(out) > maxBulkRuleIDs {
		return nil, fmt.Errorf("rule_ids exceeds maximum of %d", maxBulkRuleIDs)
	}
	return out, nil
}

func parseClassification(args map[string]any) (map[string]any, error) {
	out := map[string]any{}
	keys := []struct {
		arg string
		api string
	}{
		{"classification_component", "openshift_io_alert_rule_component"},
		{"classification_layer", "openshift_io_alert_rule_layer"},
		{"classification_component_from", "openshift_io_alert_rule_component_from"},
		{"classification_layer_from", "openshift_io_alert_rule_layer_from"},
	}
	for _, key := range keys {
		if _, ok := args[key.arg]; !ok {
			continue
		}
		value, err := nullableStringArg(args, key.arg)
		if err != nil {
			return nil, err
		}
		out[key.api] = value
	}
	return out, nil
}

func validateSeverity(severity string) error {
	if severity == "" {
		return nil
	}
	if _, ok := validSeverities[severity]; !ok {
		return fmt.Errorf("invalid severity %q: must be one of critical, warning, info, none", severity)
	}
	return nil
}

func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func nullableStringArg(args map[string]any, key string) (any, error) {
	value, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("internal: missing %s", key)
	}
	if value == nil {
		return nil, nil
	}
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("%s must be a string or null", key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	return s, nil
}

func stringSliceArg(args map[string]any, key string) ([]string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(typed))
		for i, item := range typed {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] must be a string", key, i)
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
}

func stringMapArg(args map[string]any, key string) (map[string]string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object of string values", key)
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%s] must be a string", key, k)
		}
		out[k] = strings.TrimSpace(s)
	}
	return out, nil
}

func boolPtrArg(args map[string]any, key string) (*bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	b, ok := value.(bool)
	if !ok {
		return nil, fmt.Errorf("%s must be a boolean", key)
	}
	return &b, nil
}

func toolArgs(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	return params
}
