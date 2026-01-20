package kcp

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// WorkspaceGVR is the GroupVersionResource for kcp workspaces.
var WorkspaceGVR = schema.GroupVersionResource{
	Group:    "tenancy.kcp.io",
	Version:  "v1alpha1",
	Resource: "workspaces",
}

// ParseServerURL extracts the base server URL and workspace path from a kcp server URL.
// Example: "https://10.95.33.40:6443/clusters/root" -> ("https://10.95.33.40:6443", "root")
func ParseServerURL(serverURL string) (baseURL, workspace string) {
	clustersIndex := strings.Index(serverURL, "/clusters/")
	if clustersIndex == -1 {
		return serverURL, ""
	}

	baseURL = serverURL[:clustersIndex]
	workspacePath := serverURL[clustersIndex+len("/clusters/"):]
	workspace = strings.TrimSuffix(workspacePath, "/")

	return baseURL, workspace
}

// ExtractWorkspaceFromURL returns the workspace name from a full kcp URL.
func ExtractWorkspaceFromURL(serverURL string) string {
	_, workspace := ParseServerURL(serverURL)
	return workspace
}

// ConstructWorkspaceURL builds a full kcp server URL for a workspace.
func ConstructWorkspaceURL(baseURL, workspace string) string {
	return fmt.Sprintf("%s/clusters/%s", baseURL, workspace)
}

// DiscoverWorkspacesRecursive recursively discovers child workspaces starting from a parent workspace.
// It populates the discovered map with all found workspace paths.
// The function creates dynamic clients for each workspace and queries the kcp tenancy API.
func DiscoverWorkspacesRecursive(
	ctx context.Context,
	baseRestConfig *rest.Config,
	parentWorkspace string,
	discovered map[string]bool,
) error {
	// Parse base URL from the config
	baseURL, _ := ParseServerURL(baseRestConfig.Host)

	// Create a client for the parent workspace
	workspaceRestConfig := rest.CopyConfig(baseRestConfig)
	workspaceRestConfig.Host = ConstructWorkspaceURL(baseURL, parentWorkspace)

	dynamicClient, err := dynamic.NewForConfig(workspaceRestConfig)
	if err != nil {
		klog.V(3).Infof("Failed to create client for workspace %s: %v", parentWorkspace, err)
		return nil // Don't fail entirely, just skip this workspace
	}

	// List child workspaces
	workspaceList, err := dynamicClient.Resource(WorkspaceGVR).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.V(3).Infof("Failed to list workspaces in %s: %v", parentWorkspace, err)
		return nil // Don't fail entirely, just skip
	}

	// Process each child workspace
	for _, item := range workspaceList.Items {
		childName := item.GetName()
		if childName == "" {
			continue
		}

		// Construct full workspace path
		// Child workspace names are local to their parent, so we need to prepend the parent path
		fullPath := parentWorkspace + ":" + childName

		// Skip if already discovered
		if discovered[fullPath] {
			continue
		}

		discovered[fullPath] = true
		klog.V(3).Infof("Discovered workspace: %s", fullPath)

		// Recursively discover children of this workspace
		err = DiscoverWorkspacesRecursive(ctx, baseRestConfig, fullPath, discovered)
		if err != nil {
			klog.V(3).Infof("Failed to recurse into workspace %s: %v", fullPath, err)
			// Continue with other workspaces
		}
	}

	return nil
}

// DiscoverAllWorkspaces discovers all workspaces starting from a root workspace.
// It returns a slice of all discovered workspace paths.
func DiscoverAllWorkspaces(
	ctx context.Context,
	baseRestConfig *rest.Config,
	rootWorkspace string,
) ([]string, error) {
	// Start with the root workspace
	allWorkspaces := map[string]bool{
		rootWorkspace: true,
	}

	// Recursively discover workspaces starting from the root workspace
	err := DiscoverWorkspacesRecursive(ctx, baseRestConfig, rootWorkspace, allWorkspaces)
	if err != nil {
		return nil, fmt.Errorf("failed to discover workspaces: %w", err)
	}

	// Convert map to slice
	workspaces := make([]string, 0, len(allWorkspaces))
	for ws := range allWorkspaces {
		workspaces = append(workspaces, ws)
	}

	klog.V(2).Infof("Discovered %d workspaces via kcp API (including nested)", len(workspaces))
	return workspaces, nil
}
