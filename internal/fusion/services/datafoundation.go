package services

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/internal/fusion/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DataFoundationService provides Data Foundation (ODF/OCS) operations
type DataFoundationService struct {
	client *clients.KubernetesClient
}

// NewDataFoundationService creates a new Data Foundation service
func NewDataFoundationService(client *clients.KubernetesClient) *DataFoundationService {
	return &DataFoundationService{
		client: client,
	}
}

// DataFoundationStatus represents the status of Data Foundation
type DataFoundationStatus struct {
	ComponentStatus
	Namespace      string   `json:"namespace,omitempty"`
	StorageClasses []string `json:"storageClasses,omitempty"`
	CephHealth     string   `json:"cephHealth,omitempty"`
}

// GetStatus retrieves Data Foundation status
func (s *DataFoundationService) GetStatus(ctx context.Context, clusterClient *clients.ClusterClient) (*DataFoundationStatus, error) {
	status := &DataFoundationStatus{}

	// Check for ODF namespace
	odfNamespaces := []string{"openshift-storage", "openshift-data-foundation"}
	var foundNamespace string
	for _, ns := range odfNamespaces {
		if CheckNamespaceExists(ctx, clusterClient, ns) {
			foundNamespace = ns
			break
		}
	}

	if foundNamespace == "" {
		*status = DataFoundationStatus{
			ComponentStatus: NotInstalledStatus("ODF/OCS namespace not found"),
		}
		return status, nil
	}

	status.Namespace = foundNamespace
	status.Installed = true

	// Check for ODF operator pods
	podCount, err := CheckPodsInNamespace(ctx, clusterClient, foundNamespace, "app=odf-operator")
	if err == nil && podCount > 0 {
		status.Ready = true
		status.Message = fmt.Sprintf("ODF operator running with %d pods", podCount)
	} else {
		status.Ready = false
		status.Message = "ODF operator not found or not ready"
	}

	// Get ODF storage classes
	scList, err := clusterClient.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err == nil {
		odfProvisioners := []string{
			"openshift-storage.rbd.csi.ceph.com",
			"openshift-storage.cephfs.csi.ceph.com",
			"ocs-storagecluster-ceph-rbd",
			"ocs-storagecluster-cephfs",
		}
		for _, sc := range scList.Items {
			for _, prov := range odfProvisioners {
				if sc.Provisioner == prov {
					status.StorageClasses = append(status.StorageClasses, sc.Name)
					break
				}
			}
		}
	}

	// Try to get Ceph health (best effort)
	cephGVR := schema.GroupVersionResource{
		Group:    "ceph.rook.io",
		Version:  "v1",
		Resource: "cephclusters",
	}
	if CheckCRDExists(ctx, clusterClient, cephGVR) {
		status.CephHealth = "CRD exists (detailed health check not implemented)"
	}

	return status, nil
}

// Made with Bob