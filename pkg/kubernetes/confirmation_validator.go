package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/confirmation"
	"github.com/containers/kubernetes-mcp-server/pkg/confirmation/rulexpr"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog" //nolint:staticcheck // MCP logging deprecated (SEP-2577)
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ConfirmationValidator validates Kubernetes API requests against confirmation rules.
type ConfirmationValidator struct {
	rules    []config.ConfirmationRule
	fallback string
}

func (v *ConfirmationValidator) Name() string {
	return "ConfirmationValidator"
}

func (v *ConfirmationValidator) Validate(ctx context.Context, req *api.HTTPValidationRequest) error {
	kind := ""
	group := ""
	version := ""
	if req.GVK != nil {
		kind = req.GVK.Kind
		group = req.GVK.Group
		version = req.GVK.Version
	}
	request := rulexpr.Request{Verb: req.Verb, Kind: kind, Group: group, Version: version, Name: req.ResourceName, Namespace: req.Namespace}
	if len(req.Body) > 0 && confirmationRulesNeedObject(v.rules) {
		// Only decode when a rule can look at it; a non-JSON body (e.g. a
		// JSON patch array) simply leaves object unset.
		_ = json.Unmarshal(req.Body, &request.Object)
	}
	err := confirmation.CheckKubeRequest(ctx, v.rules, v.fallback, &contextElicitor{}, request)
	if errors.Is(err, confirmation.ErrConfirmationDenied) {
		return &api.ValidationError{
			Code:    api.ErrorCodePermissionDenied,
			Message: err.Error(),
		}
	}
	return err
}

// contextElicitor extracts the MCP session from the request context to perform elicitation.
type contextElicitor struct{}

func (e *contextElicitor) Elicit(ctx context.Context, params *api.ElicitParams) (*api.ElicitResult, error) {
	session, ok := ctx.Value(mcplog.MCPSessionContextKey).(*mcp.ServerSession)
	if !ok || session == nil {
		return nil, fmt.Errorf("no MCP session found in context")
	}

	result, err := session.Elicit(ctx, &mcp.ElicitParams{
		Message:         params.Message,
		RequestedSchema: params.RequestedSchema,
		URL:             params.URL,
		ElicitationID:   params.ElicitationID,
	})
	if err != nil {
		// Mirror the detection logic from pkg/mcp/elicit.go sessionElicitor
		if isElicitationNotSupportedError(err) {
			return nil, fmt.Errorf("%w: %s", api.ErrElicitationNotSupported, err.Error())
		}
		return nil, err
	}

	return &api.ElicitResult{Action: result.Action, Content: result.Content}, nil
}

// isElicitationNotSupportedError checks whether the error from the go-sdk indicates
// the client does not support elicitation. The go-sdk does not export a typed error,
// so this uses string matching consistent with pkg/mcp/elicit.go.
func isElicitationNotSupportedError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "does not support") && strings.Contains(msg, "elicitation")
}

func confirmationRulesNeedObject(rules []config.ConfirmationRule) bool {
	for i := range rules {
		if rules[i].Expression != "" {
			return true
		}
	}
	return false
}
