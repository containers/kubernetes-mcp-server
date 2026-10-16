package confirmation

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/confirmation/rulexpr"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

// MatchToolLevelRules returns all tool-level rules that match the given tool call.
// A rule matches if all of its non-empty fields match the call:
//   - tool: exact match on tool name
//   - destructive: matches when the tool's DestructiveHint equals the rule value
func MatchToolLevelRules(rules []config.ConfirmationRule, toolName string, destructiveHint *bool) []config.ConfirmationRule {
	var matched []config.ConfirmationRule
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
//
// MatchKubeLevelRequest is for a full request: rules with
// an expression additionally evaluate it against the request, including the
// decoded body (`object`) for create/update/patch.
//
// A rule whose expression cannot be evaluated (a field access the request
// body does not satisfy, a type mismatch, a typo outside a has() guard)
// fails closed: it counts as matched, with the failure appended to its
// message, and the error is logged. Silently skipping it would disable the
// guard exactly when the request is not what the rule's author expected.
func MatchKubeLevelRequest(ctx context.Context, rules []config.ConfirmationRule, request rulexpr.Request) []config.ConfirmationRule {
	verb, kind, group, version, name, namespace := request.Verb, request.Kind, request.Group, request.Version, request.Name, request.Namespace
	var matched []config.ConfirmationRule
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
		rule := *r
		if rule.Expression != "" {
			ok, err := rulexpr.Matches(rule.Expression, request)
			if err != nil {
				klogutil.FromContext(ctx).Error(err, "confirmation rule expression could not be evaluated, requiring confirmation",
					"verb", verb, "kind", kind, "group", group, "name", name, "namespace", namespace)
				rule.Message = fmt.Sprintf("%s (the rule's expression could not be evaluated, so confirmation is required: %v)", rule.Message, err)
			} else if !ok {
				continue
			}
		}
		matched = append(matched, rule)
	}
	return matched
}

// MergeMatchedRules combines matched rules into a single message.
// If a single rule matched, its message is used directly.
// If multiple rules matched, messages are combined as a bulleted list.
// The global fallback is always used as the effective fallback.
func MergeMatchedRules(matched []config.ConfirmationRule, globalFallback string) (message string, effectiveFallback string) {
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
