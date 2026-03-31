package confirmation

import (
	"context"
	"errors"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/klog/v2"
)

// ErrConfirmationDenied is returned when the user declines a confirmation prompt
// or when the client does not support elicitation and the fallback is "deny".
var ErrConfirmationDenied = errors.New("action requires confirmation")

// CheckToolRules finds matching tool-level rules, merges them, and elicits confirmation.
// Returns nil if no rules match or the user accepts. Returns an error if the user
// declines or elicitation is not supported and the fallback is "deny".
func CheckToolRules(ctx context.Context, provider api.ConfirmationRulesProvider, elicitor api.Elicitor,
	toolName string, destructiveHint *bool) error {

	matched := MatchToolLevelRules(provider.GetConfirmationRules(), toolName, destructiveHint)
	if len(matched) == 0 {
		return nil
	}
	message, fallback := MergeMatchedRules(matched, provider.GetConfirmationFallback())
	return CheckConfirmation(ctx, elicitor, message, fallback)
}

// CheckKubeRules finds matching kube-level rules, merges them, and elicits confirmation.
// Returns nil if no rules match or the user accepts.
func CheckKubeRules(ctx context.Context, provider api.ConfirmationRulesProvider, elicitor api.Elicitor,
	verb, kind, group, version, name, namespace string) error {

	matched := MatchKubeLevelRules(provider.GetConfirmationRules(), verb, kind, group, version, name, namespace)
	if len(matched) == 0 {
		return nil
	}
	message, fallback := MergeMatchedRules(matched, provider.GetConfirmationFallback())
	return CheckConfirmation(ctx, elicitor, message, fallback)
}

// CheckConfirmation prompts the user for confirmation via the elicitor.
// If the client does not support elicitation, the fallback determines behavior:
// "deny" returns ErrConfirmationDenied, "allow" returns nil (with a warning log).
func CheckConfirmation(ctx context.Context, elicitor api.Elicitor, message, fallback string) error {
	result, err := elicitor.Elicit(ctx, &api.ElicitParams{
		Message: message,
		RequestedSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
	})
	if err != nil {
		if isElicitationNotSupported(err) {
			if fallback == "deny" {
				return ErrConfirmationDenied
			}
			klog.Warningf("Confirmation rules matched but client does not support elicitation, proceeding with fallback \"allow\": %s", message)
			return nil
		}
		return err
	}
	if result.Action == api.ElicitActionAccept {
		return nil
	}
	return ErrConfirmationDenied
}

// isElicitationNotSupported checks whether the error indicates the client does not
// support elicitation. This mirrors the string check in pkg/mcp/elicit.go since
// the go-sdk does not export a typed error for this case.
func isElicitationNotSupported(err error) bool {
	return errors.Is(err, api.ErrElicitationNotSupported)
}
