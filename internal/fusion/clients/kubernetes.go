package clients

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubernetesClient wraps the standard Kubernetes client for Fusion-specific operations
type KubernetesClient struct {
	client api.KubernetesClient
}

// NewKubernetesClient creates a new Fusion Kubernetes client wrapper
func NewKubernetesClient(client api.KubernetesClient) *KubernetesClient {
	return &KubernetesClient{
		client: client,
	}
}

// ListStorageClasses retrieves all storage classes in the cluster
func (c *KubernetesClient) ListStorageClasses(ctx context.Context) (*storagev1.StorageClassList, error) {
	return c.client.Clientset().StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
}

// ListPVCs retrieves all PVCs in a given namespace
func (c *KubernetesClient) ListPVCs(ctx context.Context, namespace string) (interface{}, error) {
	if namespace == "" {
		namespace = metav1.NamespaceAll
	}
	return c.client.Clientset().CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
}

// Made with Bob
