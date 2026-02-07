package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/fusion/clients"
	"github.com/containers/kubernetes-mcp-server/internal/fusion/targeting"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// ClusterOperation represents an operation to execute on a cluster
type ClusterOperation func(ctx context.Context, client *clients.ClusterClient) (interface{}, error)

// ExecuteOnClusters executes an operation across multiple clusters based on target
func ExecuteOnClusters(ctx context.Context, registry *clients.Registry, target targeting.Target, operation ClusterOperation) *targeting.Result {
	result := targeting.NewResult(target)

	// Get cluster names based on target type
	clusterNames, err := target.ResolveClusterNames(registry)
	if err != nil {
		result.Summary.Error = err.Error()
		return result
	}

	// Set timeout
	timeout := 30 * time.Second
	if target.Timeout > 0 {
		timeout = time.Duration(target.Timeout) * time.Second
	}

	// Execute on each cluster concurrently
	var wg sync.WaitGroup
	resultChan := make(chan targeting.ClusterResult, len(clusterNames))

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			// Create context with timeout
			opCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Get cluster client
			client, err := registry.GetClient(name)
			if err != nil {
				resultChan <- targeting.ClusterResult{
					Cluster: name,
					Success: false,
					Error:   fmt.Sprintf("failed to get client: %v", err),
				}
				return
			}

			// Execute operation
			data, err := operation(opCtx, client)
			if err != nil {
				resultChan <- targeting.ClusterResult{
					Cluster: name,
					Success: false,
					Error:   err.Error(),
				}
				return
			}

			// Marshal data to JSON
			jsonData, err := json.Marshal(data)
			if err != nil {
				resultChan <- targeting.ClusterResult{
					Cluster: name,
					Success: false,
					Error:   fmt.Sprintf("failed to marshal data: %v", err),
				}
				return
			}

			resultChan <- targeting.ClusterResult{
				Cluster: name,
				Success: true,
				Data:    json.RawMessage(jsonData),
			}
		}(clusterName)
	}

	// Wait for all operations to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for clusterResult := range resultChan {
		result.AddClusterResult(clusterResult.Cluster, clusterResult.Data, 
			func() error {
				if !clusterResult.Success {
					return fmt.Errorf("%s", clusterResult.Error)
				}
				return nil
			}())
	}

	return result
}

// CheckCRDExists checks if a CRD exists in the cluster
func CheckCRDExists(ctx context.Context, client *clients.ClusterClient, gvr schema.GroupVersionResource) bool {
	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(client.Config)
	
	_, apiResourceList, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return false
	}

	for _, list := range apiResourceList {
		if list.GroupVersion == gvr.GroupVersion().String() {
			for _, resource := range list.APIResources {
				if resource.Name == gvr.Resource {
					return true
				}
			}
		}
	}

	return false
}

// CheckNamespaceExists checks if a namespace exists
func CheckNamespaceExists(ctx context.Context, client *clients.ClusterClient, namespace string) bool {
	_, err := client.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	return err == nil
}

// CheckPodsInNamespace checks if there are pods in a namespace with a label selector
func CheckPodsInNamespace(ctx context.Context, client *clients.ClusterClient, namespace, labelSelector string) (int, error) {
	pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return 0, err
	}
	return len(pods.Items), nil
}

// ComponentStatus represents the status of a component
type ComponentStatus struct {
	Installed bool   `json:"installed"`
	Ready     bool   `json:"ready,omitempty"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message,omitempty"`
}

// NotInstalledStatus returns a status indicating component is not installed
func NotInstalledStatus(message string) ComponentStatus {
	return ComponentStatus{
		Installed: false,
		Message:   message,
	}
}

// InstalledStatus returns a status indicating component is installed
func InstalledStatus(ready bool, version, message string) ComponentStatus {
	return ComponentStatus{
		Installed: true,
		Ready:     ready,
		Version:   version,
		Message:   message,
	}
}

// Made with Bob