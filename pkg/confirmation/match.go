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
func MatchToolLevelRules(rules []api.ConfirmationRule, toolName string, destructiveHint *bool) []api.ConfirmationRule {
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
		matched = append(matched, *r)
	}
	return matched
}

// MatchKubeLevelRules returns all kube-level rules that match the given Kubernetes API request.
// A rule matches if all of its non-empty fields match the request:
//   - verb: exact match (e.g. "get", "delete", "list")
//   - kind: exact match on the resource kind
//   - group: exact match on the API group
//   - version: exact match on the API version
//   - name: exact match on the resource name
//   - namespace: exact match on the namespace
func MatchKubeLevelRules(rules []api.ConfirmationRule, verb, kind, group, version, name, namespace string) []api.ConfirmationRule {
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
		if r.Name != "" && r.Name != name {
			continue
		}
		if r.Namespace != "" && r.Namespace != namespace {
			continue
		}
		matched = append(matched, *r)
	}
	return matched
}

// MergeMatchedRules combines matched rules into a single message.
// If a single rule matched, its message is used directly.
// If multiple rules matched, messages are combined as a bulleted list.
// The global fallback is always used as the effective fallback.
func MergeMatchedRules(matched []api.ConfirmationRule, globalFallback string) (message string, effectiveFallback string) {
	if len(matched) == 0 {
		return "", globalFallback
	}
	if len(matched) == 1 {
		return matched[0].Message, globalFallback
	}

	var sb strings.Builder
	sb.WriteString("Confirmation required:")
	for _, r := range matched {
		fmt.Fprintf(&sb, "\n- %s", r.Message)
	}
	return sb.String(), globalFallback
}
