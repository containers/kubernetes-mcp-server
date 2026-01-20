package kcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kcppkg "github.com/containers/kubernetes-mcp-server/pkg/kcp"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func initWorkspaceTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "kcp_workspaces_list",
				Description: "List all available kcp workspaces in the current cluster ",
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
	// Recursively discover all workspaces
	core := kubernetes.NewCore(params)
	restConfig := core.RESTConfig()

	// Determine current workspace from server URL
	currentWorkspace := kcppkg.ExtractWorkspaceFromURL(restConfig.Host)
	if currentWorkspace == "" {
		currentWorkspace = "root"
	}

	// Discover all workspaces recursively
	workspaces, err := kcppkg.DiscoverAllWorkspaces(params.Context, restConfig, currentWorkspace)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to discover workspaces: %v", err)), nil
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

	dynamicClient := kubernetes.NewCore(params).DynamicClient()

	workspace, err := dynamicClient.Resource(kcppkg.WorkspaceGVR).
		Get(context.TODO(), workspaceName, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get workspace: %v", err)), nil
	}

	// Format workspace details as YAML
	yamlData, err := output.MarshalYaml(workspace.Object)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal workspace: %v", err)), nil
	}

	return api.NewToolCallResult(yamlData, nil), nil
}
