package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/yaml"
)

// WindowsEULAValidator enforces EULA acceptance for Windows golden image PipelineRuns.
// This prevents LLMs from bypassing EULA prompts by directly calling resources_create_or_update.
type WindowsEULAValidator struct{}

// elicitor defines the interface for elicitation support.
type elicitor interface {
	Elicit(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error)
}

func (v *WindowsEULAValidator) Name() string {
	return "WindowsEULAValidator"
}

func (v *WindowsEULAValidator) Validate(ctx context.Context, req *api.HTTPValidationRequest) error {
	if !isWindowsPipelineRun(req) {
		return nil
	}

	pr := &tektonv1.PipelineRun{}
	if err := yaml.Unmarshal(req.Body, pr); err != nil {
		return err
	}

	if !requiresEULAPrompt(pr) {
		return nil
	}

	return promptForEULA(ctx)
}

// isWindowsPipelineRun checks if this request should be validated.
func isWindowsPipelineRun(req *api.HTTPValidationRequest) bool {
	// Only validate create/update operations
	if req.Verb != "create" && req.Verb != "update" {
		return false
	}

	// Only validate PipelineRun resources
	return req.GVK != nil && req.GVK.Kind == "PipelineRun" && req.GVK.Group == "tekton.dev"
}

// requiresEULAPrompt checks if the PipelineRun requires EULA acceptance.
func requiresEULAPrompt(pr *tektonv1.PipelineRun) bool {
	// Check if this is a windows-efi-installer pipeline
	if !isWindowsEFIInstallerPipeline(pr) {
		return false
	}

	// Check if EULA is already accepted
	return getPipelineParam(pr.Spec.Params, "acceptEula") != "true"
}

// promptForEULA prompts the user to accept the Windows EULA via elicitation.
func promptForEULA(ctx context.Context) error {
	session, err := getElicitorFromContext(ctx)
	if err != nil {
		return err
	}

	result, err := session.Elicit(ctx, &mcp.ElicitParams{
		Message: "Microsoft EULA Notice: This PipelineRun will create a Windows golden image. By proceeding, you agree to the Microsoft Software License Terms for the Windows operating system. Do you accept the Microsoft End User License Agreement (EULA)?",
	})
	if err != nil {
		if isElicitationNotSupportedError(err) {
			return &api.ValidationError{
				Code:    api.ErrorCodePermissionDenied,
				Message: "Windows EULA acceptance required. Set acceptEula parameter to \"true\" in the PipelineRun to proceed.",
			}
		}
		return err
	}

	if result.Action != api.ElicitActionAccept {
		return &api.ValidationError{
			Code:    api.ErrorCodePermissionDenied,
			Message: "Windows EULA was not accepted. The PipelineRun creation has been cancelled.",
		}
	}

	return nil
}

// getElicitorFromContext retrieves an elicitor from the request context.
func getElicitorFromContext(ctx context.Context) (elicitor, error) {
	sessionValue := ctx.Value(mcplog.MCPSessionContextKey)
	if sessionValue == nil {
		return nil, &api.ValidationError{
			Code:    api.ErrorCodePermissionDenied,
			Message: "Windows EULA acceptance required but no session available for confirmation",
		}
	}

	session, ok := sessionValue.(elicitor)
	if !ok {
		return nil, &api.ValidationError{
			Code:    api.ErrorCodePermissionDenied,
			Message: "Windows EULA acceptance required but session does not support elicitation",
		}
	}

	return session, nil
}

// isWindowsEFIInstallerPipeline checks if the PipelineRun references the windows-efi-installer pipeline.
func isWindowsEFIInstallerPipeline(pr *tektonv1.PipelineRun) bool {
	if pr.Spec.PipelineRef == nil {
		return false
	}

	return getPipelineParam(pr.Spec.PipelineRef.Params, "name") == "windows-efi-installer"
}

// getPipelineParam retrieves a parameter value by name from a Params slice.
func getPipelineParam(params tektonv1.Params, name string) string {
	for _, param := range params {
		if param.Name == name {
			return param.Value.StringVal
		}
	}
	return ""
}
