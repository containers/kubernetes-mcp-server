package services

import (
	"context"
	"fmt"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/fusion/clients"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// BackupService provides backup and restore operations
type BackupService struct {
	client *clients.KubernetesClient
}

// NewBackupService creates a new backup service
func NewBackupService(client *clients.KubernetesClient) *BackupService {
	return &BackupService{
		client: client,
	}
}

// BackupJob represents a backup job
type BackupJob struct {
	Name       string    `json:"name"`
	Namespace  string    `json:"namespace"`
	Status     string    `json:"status"`
	StartTime  time.Time `json:"startTime,omitempty"`
	Completion time.Time `json:"completionTime,omitempty"`
	Age        string    `json:"age"`
}

// BackupJobsList represents a list of backup jobs
type BackupJobsList struct {
	ComponentStatus
	Jobs []BackupJob `json:"jobs,omitempty"`
}

// ListJobs lists backup jobs
func (s *BackupService) ListJobs(ctx context.Context, clusterClient *clients.ClusterClient) (*BackupJobsList, error) {
	result := &BackupJobsList{
		Jobs: []BackupJob{},
	}

	// Check for OADP namespace (OpenShift API for Data Protection)
	oadpNamespace := "openshift-adp"
	if !CheckNamespaceExists(ctx, clusterClient, oadpNamespace) {
		result.ComponentStatus = NotInstalledStatus("OADP namespace not found")
		return result, nil
	}

	result.Installed = true

	// Check for Velero CRD (OADP uses Velero)
	veleroGVR := schema.GroupVersionResource{
		Group:    "velero.io",
		Version:  "v1",
		Resource: "backups",
	}

	if !CheckCRDExists(ctx, clusterClient, veleroGVR) {
		result.Ready = false
		result.Message = "Velero CRDs not found"
		return result, nil
	}

	result.Ready = true

	// List backup jobs (using standard Kubernetes Jobs as fallback)
	jobs, err := clusterClient.Clientset.BatchV1().Jobs(oadpNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=backup",
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to list jobs: %v", err)
		return result, nil
	}

	// Convert to BackupJob format
	for _, job := range jobs.Items {
		backupJob := s.convertJob(&job)
		result.Jobs = append(result.Jobs, backupJob)
	}

	result.Message = fmt.Sprintf("Found %d backup jobs", len(result.Jobs))
	return result, nil
}

// convertJob converts a Kubernetes Job to BackupJob
func (s *BackupService) convertJob(job *batchv1.Job) BackupJob {
	status := "Unknown"
	if job.Status.Succeeded > 0 {
		status = "Completed"
	} else if job.Status.Failed > 0 {
		status = "Failed"
	} else if job.Status.Active > 0 {
		status = "Running"
	}

	age := time.Since(job.CreationTimestamp.Time).Round(time.Second).String()

	backupJob := BackupJob{
		Name:      job.Name,
		Namespace: job.Namespace,
		Status:    status,
		Age:       age,
	}

	if job.Status.StartTime != nil {
		backupJob.StartTime = job.Status.StartTime.Time
	}
	if job.Status.CompletionTime != nil {
		backupJob.Completion = job.Status.CompletionTime.Time
	}

	return backupJob
}

// Made with Bob