package confirmation

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// MatchToolLevelRules returns all tool-level rules that match the given tool call.
// A rule matches if all of its non-empty fields match the call:
//   - tool: exact match on tool name
//   - destructive: matches when the tool's DestructiveHint equals the rule value
//   - input: all key-value pairs must match the tool's arguments (string comparison)
func MatchToolLevelRules(rules []api.ConfirmationRule, toolName string, args map[string]any, destructiveHint *bool) []api.ConfirmationRule {
	var matched []api.ConfirmationRule
	for i := range rules {
		r := &rules[i]
		if !r.IsToolLevel() {
			continue
		}
		if r.Tool != "" && r.Tool != toolName {
			continue
		}
		if r.Destructive != nil {
			if destructiveHint == nil || *r.Destructive != *destructiveHint {
				continue
			}
		}
		if !inputMatches(r.Input, args) {
			continue
		}
		matched = append(matched, *r)
	}
	return matched
}

// inputMatches returns true if every key-value pair in ruleInput is present in args
// with an equal value. Rule input values must be normalized to JSON types at config
// load time (via NormalizeInput) so that plain == comparison works here.
func inputMatches(ruleInput map[string]any, args map[string]any) bool {
	for k, ruleVal := range ruleInput {
		argVal, ok := args[k]
		if !ok {
			return false
		}
		if ruleVal != argVal {
			return false
		}
	}
	return true
}

// NormalizeInput converts TOML-parsed input values to JSON-equivalent Go types.
// TOML parses integers as int64 while JSON parses numbers as float64. Converting
// int64 to float64 at load time ensures plain == comparison works at match time
// without per-match type coercion.
func NormalizeInput(input map[string]any) map[string]any {
	for k, v := range input {
		if i, ok := v.(int64); ok {
			input[k] = float64(i)
		}
	}
	return input
}

// MatchKubeLevelRules returns all kube-level rules that match the given Kubernetes API request.
// A rule matches if all of its non-empty fields match the request:
//   - verb: exact match (e.g. "get", "delete", "list")
//   - kind: exact match on the resource kind
//   - group: exact match on the API group
//   - version: exact match on the API version
//   - namespace: exact match on the namespace
func MatchKubeLevelRules(rules []api.ConfirmationRule, verb, kind, group, version, namespace string) []api.ConfirmationRule {
	var matched []api.ConfirmationRule
	for i := range rules {
		r := &rules[i]
		if !r.IsKubeLevel() {
			continue
		}
		if r.Verb != "" && r.Verb != verb {
			continue
		}
		if r.Kind != "" && r.Kind != kind {
			continue
		}
		if r.Group != "" && r.Group != group {
			continue
		}
		if r.Version != "" && r.Version != version {
			continue
		}
		if r.Namespace != "" && r.Namespace != namespace {
			continue
		}
		matched = append(matched, *r)
	}
	return matched
}

// MergeMatchedRules combines matched rules into a single message and effective fallback.
// If a single rule matched, its message and fallback are used directly.
// If multiple rules matched, messages are combined as a bulleted list and the
// most restrictive fallback wins ("deny" beats "allow").
func MergeMatchedRules(matched []api.ConfirmationRule, globalFallback string) (message string, effectiveFallback string) {
	if len(matched) == 0 {
		return "", globalFallback
	}
	if len(matched) == 1 {
		return matched[0].Message, matched[0].EffectiveFallback(globalFallback)
	}

	var sb strings.Builder
	sb.WriteString("Confirmation required:")
	for _, r := range matched {
		sb.WriteString(fmt.Sprintf("\n- %s", r.Message))
	}
	message = sb.String()

	effectiveFallback = "allow"
	for _, r := range matched {
		if r.EffectiveFallback(globalFallback) == "deny" {
			effectiveFallback = "deny"
			break
		}
	}
	return message, effectiveFallback
}
