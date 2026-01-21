package kcp

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Kubernetes defines the minimal interface required for kcp operations.
type Kubernetes interface {
	RESTConfig() *rest.Config
	DynamicClient() dynamic.Interface
}

// Kcp provides operations for interacting with kcp workspaces.
type Kcp struct {
	kubernetes Kubernetes
}

// NewKcp creates a new Kcp instance.
func NewKcp(kubernetes Kubernetes) *Kcp {
	return &Kcp{kubernetes: kubernetes}
}

// ListWorkspaces returns all available kcp workspaces discovered recursively from the current workspace.
func (k *Kcp) ListWorkspaces(ctx context.Context) ([]string, error) {
	restConfig := k.kubernetes.RESTConfig()

	// Determine current workspace from server URL
	currentWorkspace := ExtractWorkspaceFromURL(restConfig.Host)
	if currentWorkspace == "" {
		currentWorkspace = "root"
	}

	// Discover all workspaces recursively
	return DiscoverAllWorkspaces(ctx, restConfig, currentWorkspace)
}

// DescribeWorkspace returns detailed information about a specific kcp workspace.
func (k *Kcp) DescribeWorkspace(ctx context.Context, name string) (map[string]any, error) {
	dynamicClient := k.kubernetes.DynamicClient()

	workspace, err := dynamicClient.Resource(WorkspaceGVR).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	return workspace.Object, nil
}
