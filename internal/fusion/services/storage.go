package services

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/internal/fusion/clients"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageService provides storage-related operations for IBM Fusion
type StorageService struct {
	client *clients.KubernetesClient
}

// NewStorageService creates a new storage service
func NewStorageService(client *clients.KubernetesClient) *StorageService {
	return &StorageService{
		client: client,
	}
}

// StorageClassInfo contains information about a storage class
type StorageClassInfo struct {
	Name        string `json:"name"`
	Provisioner string `json:"provisioner"`
	IsDefault   bool   `json:"isDefault"`
}

// PVCStats contains statistics about PVCs
type PVCStats struct {
	Bound   int `json:"bound"`
	Pending int `json:"pending"`
	Lost    int `json:"lost"`
	Total   int `json:"total"`
}

// StorageSummary contains a summary of storage status
type StorageSummary struct {
	StorageClasses []StorageClassInfo `json:"storageClasses"`
	PVCStats       PVCStats           `json:"pvcStats"`
	ODFInstalled   bool               `json:"odfInstalled"`
}

// GetStorageSummary retrieves a comprehensive storage summary
func (s *StorageService) GetStorageSummary(ctx context.Context) (*StorageSummary, error) {
	summary := &StorageSummary{
		StorageClasses: []StorageClassInfo{},
		PVCStats:       PVCStats{},
		ODFInstalled:   false,
	}

	// Get storage classes
	scList, err := s.client.ListStorageClasses(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage classes: %w", err)
	}

	summary.StorageClasses = s.extractStorageClassInfo(scList)

	// Get PVC statistics
	pvcList, err := s.client.ListPVCs(ctx, metav1.NamespaceAll)
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs: %w", err)
	}

	summary.PVCStats = s.calculatePVCStats(pvcList)

	// Check for ODF/OCS installation (non-failing check)
	summary.ODFInstalled = s.checkODFInstalled(scList)

	return summary, nil
}

// extractStorageClassInfo extracts relevant info from storage classes
func (s *StorageService) extractStorageClassInfo(scList *storagev1.StorageClassList) []StorageClassInfo {
	info := make([]StorageClassInfo, 0, len(scList.Items))
	for _, sc := range scList.Items {
		isDefault := false
		if sc.Annotations != nil {
			if val, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]; ok && val == "true" {
				isDefault = true
			}
		}
		info = append(info, StorageClassInfo{
			Name:        sc.Name,
			Provisioner: sc.Provisioner,
			IsDefault:   isDefault,
		})
	}
	return info
}

// calculatePVCStats calculates statistics from PVC list
func (s *StorageService) calculatePVCStats(pvcList interface{}) PVCStats {
	stats := PVCStats{}

	// Type assert to PVC list
	if list, ok := pvcList.(*corev1.PersistentVolumeClaimList); ok {
		stats.Total = len(list.Items)
		for _, pvc := range list.Items {
			switch pvc.Status.Phase {
			case corev1.ClaimBound:
				stats.Bound++
			case corev1.ClaimPending:
				stats.Pending++
			case corev1.ClaimLost:
				stats.Lost++
			}
		}
	}

	return stats
}

// checkODFInstalled checks if ODF/OCS is installed by looking for known provisioners
func (s *StorageService) checkODFInstalled(scList *storagev1.StorageClassList) bool {
	odfProvisioners := []string{
		"openshift-storage.rbd.csi.ceph.com",
		"openshift-storage.cephfs.csi.ceph.com",
		"ocs-storagecluster-ceph-rbd",
		"ocs-storagecluster-cephfs",
	}

	for _, sc := range scList.Items {
		for _, odfProv := range odfProvisioners {
			if sc.Provisioner == odfProv {
				return true
			}
		}
	}
	return false
}

// Made with Bob
