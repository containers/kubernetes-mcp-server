package kcp

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kcppkg "github.com/containers/kubernetes-mcp-server/pkg/kcp"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func initWorkspaceTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "kcp_workspaces_list",
				Description: "List all available kcp workspaces in the current cluster",
				InputSchema: &jsonschema.Schema{
					Type: "object",
				},
				Annotations: api.ToolAnnotations{
					Title:           "kcp: Workspaces List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			ClusterAware:       ptr.To(false),
			TargetListProvider: ptr.To(false),
			Handler:            workspacesList,
		},
		{
			Tool: api.Tool{
				Name:        "kcp_workspace_describe",
				Description: "Get detailed information about a specific kcp workspace",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"workspace": {
							Type:        "string",
							Description: "Name or path of the workspace to describe",
						},
					},
					Required: []string{"workspace"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "kcp: Workspace Describe",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			ClusterAware: ptr.To(false),
			Handler:      workspaceDescribe,
		},
	}
}

func workspacesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kcp := kcppkg.NewKcp(params)

	workspaces, err := kcp.ListWorkspaces(params.Context)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to discover workspaces: %w", err)), nil
	}

	if len(workspaces) == 0 {
		return api.NewToolCallResult("No workspaces found", nil), nil
	}

	result := fmt.Sprintf("Available kcp workspaces (%d total):\n\n", len(workspaces))
	for _, ws := range workspaces {
		result += fmt.Sprintf("- %s\n", ws)
	}

	return api.NewToolCallResult(result, nil), nil
}

func workspaceDescribe(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	workspaceName, ok := params.GetArguments()["workspace"].(string)
	if !ok {
		return api.NewToolCallResult("", fmt.Errorf("workspace parameter is required")), nil
	}

	kcp := kcppkg.NewKcp(params)

	workspaceObj, err := kcp.DescribeWorkspace(params.Context, workspaceName)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get workspace: %w", err)), nil
	}

	// Format workspace details as YAML
	yamlData, err := output.MarshalYaml(workspaceObj)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal workspace: %w", err)), nil
	}

	return api.NewToolCallResult(yamlData, nil), nil
}
