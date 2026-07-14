package core

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func initRollout() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "rollout_status",
				Description: "Check the rollout status of a Kubernetes Deployment in the current or provided namespace. Reports whether the rollout is complete, in progress, or has failed (e.g. progress deadline exceeded). Mirrors `kubectl rollout status deployment`",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Deployment (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Deployment to check rollout status for",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Rollout: Status",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: rolloutStatus,
		},
		{
			Tool: api.Tool{
				Name:        "rollout_history",
				Description: "View the rollout history (revisions) of a Kubernetes Deployment in the current or provided namespace. Lists each revision with its image. Mirrors `kubectl rollout history deployment`",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Deployment (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Deployment to view rollout history for",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Rollout: History",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: rolloutHistory,
		},
		{
			Tool: api.Tool{
				Name:        "rollout_undo",
				Description: "Rollback a Kubernetes Deployment to a previous revision. If to_revision is not provided, rolls back to the immediately previous revision. Mirrors `kubectl rollout undo deployment`",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Deployment (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Deployment to roll back",
						},
						"to_revision": {
							Type:        "integer",
							Description: "Revision number to roll back to (Optional, defaults to the previous revision)",
							Minimum:     ptr.To(float64(0)),
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Rollout: Undo",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: rolloutUndo,
		},
		{
			Tool: api.Tool{
				Name:        "rollout_restart",
				Description: "Trigger a rolling restart of a Kubernetes Deployment by patching the pod template with a restart timestamp. Existing pods are replaced gradually. Mirrors `kubectl rollout restart deployment`",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace of the Deployment (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the Deployment to restart",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Rollout: Restart",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: rolloutRestart,
		},
	}
}

func rolloutStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get rollout status: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).DeploymentRolloutStatus(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get rollout status for deployment %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func rolloutHistory(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get rollout history: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).DeploymentRolloutHistory(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get rollout history for deployment %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func rolloutUndo(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	toRevision := p.OptionalInt64("to_revision", 0)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to roll back deployment: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).DeploymentRolloutUndo(params.Context, ns, name, toRevision)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to roll back deployment %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func rolloutRestart(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to restart deployment: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).DeploymentRestart(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to restart deployment %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}
