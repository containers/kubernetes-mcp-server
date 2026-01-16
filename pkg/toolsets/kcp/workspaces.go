package kcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func initWorkspaceTools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "kcp_workspaces_list",
				Description: "List all available kcp workspaces in the cluster",
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
	dynamicClient := core.DynamicClient()

	workspaceGVR := schema.GroupVersionResource{
		Group:    "tenancy.kcp.io",
		Version:  "v1alpha1",
		Resource: "workspaces",
	}

	// Determine current workspace from server URL
	currentWorkspace := extractWorkspaceFromURL(restConfig.Host)
	if currentWorkspace == "" {
		currentWorkspace = "root"
	}

	// Discover all workspaces recursively
	allWorkspaces := make(map[string]bool)
	allWorkspaces[currentWorkspace] = true

	err := discoverWorkspacesRecursive(params.Context, dynamicClient, restConfig, currentWorkspace, workspaceGVR, allWorkspaces)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to discover workspaces: %v", err)), nil
	}

	if len(allWorkspaces) == 0 {
		return api.NewToolCallResult("No workspaces found", nil), nil
	}

	// Convert to slice
	workspaces := make([]string, 0, len(allWorkspaces))
	for ws := range allWorkspaces {
		workspaces = append(workspaces, ws)
	}

	result := fmt.Sprintf("Available kcp workspaces (%d total):\n\n", len(workspaces))
	for _, ws := range workspaces {
		result += fmt.Sprintf("- %s\n", ws)
	}

	return api.NewToolCallResult(result, nil), nil
}

// extractWorkspaceFromURL extracts the workspace name from a kcp server URL
func extractWorkspaceFromURL(serverURL string) string {
	// Look for /clusters/ pattern
	idx := len(serverURL)
	for i := len(serverURL) - 1; i >= 0; i-- {
		if serverURL[i] == '/' {
			idx = i
			break
		}
	}
	if idx < len(serverURL) && idx > 0 {
		// Check if this is part of /clusters/ pattern
		clustersIdx := idx - len("/clusters")
		if clustersIdx >= 0 && serverURL[clustersIdx:idx] == "/clusters" {
			return serverURL[idx+1:]
		}
	}
	return ""
}

// discoverWorkspacesRecursive recursively discovers child workspaces
func discoverWorkspacesRecursive(
	ctx context.Context,
	baseDynamicClient dynamic.Interface,
	baseRestConfig *rest.Config,
	parentWorkspace string,
	workspaceGVR schema.GroupVersionResource,
	discovered map[string]bool,
) error {
	// Create a client for the parent workspace
	workspaceRestConfig := rest.CopyConfig(baseRestConfig)
	// Construct workspace URL
	baseURL := workspaceRestConfig.Host
	if idx := len(baseURL) - 1; idx >= 0 {
		for i := len(baseURL) - 1; i >= 0; i-- {
			if baseURL[i:] == "/clusters/"+parentWorkspace {
				baseURL = baseURL[:i]
				break
			}
		}
	}
	// Remove any trailing /clusters/... part
	if clustersIdx := len(baseURL); clustersIdx > 0 {
		for i := len(baseURL) - 1; i >= 0; i-- {
			if i+len("/clusters") <= len(baseURL) && baseURL[i:i+len("/clusters")] == "/clusters" {
				baseURL = baseURL[:i]
				break
			}
		}
	}
	workspaceRestConfig.Host = baseURL + "/clusters/" + parentWorkspace

	dynamicClient, err := dynamic.NewForConfig(workspaceRestConfig)
	if err != nil {
		return nil // Don't fail entirely, just skip this workspace
	}

	// List child workspaces
	workspaceList, err := dynamicClient.Resource(workspaceGVR).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil // Don't fail entirely, just skip
	}

	// Process each child workspace
	for _, item := range workspaceList.Items {
		childName := item.GetName()
		if childName == "" {
			continue
		}

		// Construct full workspace path
		fullPath := parentWorkspace + ":" + childName

		// Skip if already discovered
		if discovered[fullPath] {
			continue
		}

		discovered[fullPath] = true

		// Recursively discover children of this workspace
		_ = discoverWorkspacesRecursive(ctx, baseDynamicClient, baseRestConfig, fullPath, workspaceGVR, discovered)
	}

	return nil
}

func workspaceDescribe(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	workspaceName, ok := params.GetArguments()["workspace"].(string)
	if !ok {
		return api.NewToolCallResult("", fmt.Errorf("workspace parameter is required")), nil
	}

	dynamicClient := kubernetes.NewCore(params).DynamicClient()

	workspaceGVR := schema.GroupVersionResource{
		Group:    "tenancy.kcp.io",
		Version:  "v1alpha1",
		Resource: "workspaces",
	}

	workspace, err := dynamicClient.Resource(workspaceGVR).
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
