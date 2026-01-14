package kiali

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// WorkloadLogs returns logs for a specific workload's pods in a namespace.
// This method first gets workload details to find associated pods, then retrieves logs for each pod.
// Parameters:
//   - namespace: the namespace containing the workload
//   - workload: the name of the workload
//   - container: container name (optional, will be auto-detected if not provided)
//   - duration: time duration (e.g., "5m", "1h") - optional
//   - maxLines: maximum number of lines to return - optional
func (k *Kiali) WorkloadLogs(ctx context.Context, namespace string, workload string, container string, duration string, maxLines string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if workload == "" {
		return "", fmt.Errorf("workload name is required")
	}
	// Container is optional - will be auto-detected if not provided

	// First, get workload details to find associated pods
	workloadDetails, err := k.WorkloadDetails(ctx, namespace, workload)
	if err != nil {
		return "", fmt.Errorf("failed to get workload details: %w", err)
	}

	// Parse the workload details JSON to extract pod names and containers
	var workloadData struct {
		Pods []struct {
			Name       string `json:"name"`
			Containers []struct {
				Name string `json:"name"`
			} `json:"containers"`
		} `json:"pods"`
	}

	if err := json.Unmarshal([]byte(workloadDetails), &workloadData); err != nil {
		return "", fmt.Errorf("failed to parse workload details: %w", err)
	}

	if len(workloadData.Pods) == 0 {
		return "", fmt.Errorf("no pods found for workload %s in namespace %s", workload, namespace)
	}

	// Collect logs from all pods
	var allLogs []string
	for _, pod := range workloadData.Pods {
		// Auto-detect container if not provided
		podContainer := container
		if podContainer == "" {
			// Find the main application container (not istio-proxy or istio-init)
			for _, c := range pod.Containers {
				if c.Name != "istio-proxy" && c.Name != "istio-init" {
					podContainer = c.Name
					break
				}
			}
			// If no app container found, use the first container
			if podContainer == "" && len(pod.Containers) > 0 {
				podContainer = pod.Containers[0].Name
			}
		}

		if podContainer == "" {
			allLogs = append(allLogs, fmt.Sprintf("Error: No container found for pod %s", pod.Name))
			continue
		}

		podLogs, err := k.PodLogs(ctx, namespace, pod.Name, podContainer, workload, duration, maxLines)
		if err != nil {
			// Log the error but continue with other pods
			allLogs = append(allLogs, fmt.Sprintf("Error getting logs for pod %s: %v", pod.Name, err))
			continue
		}
		if podLogs != "" {
			allLogs = append(allLogs, fmt.Sprintf("=== Pod: %s (Container: %s) ===\n%s", pod.Name, podContainer, podLogs))
		}
	}

	if len(allLogs) == 0 {
		return "", fmt.Errorf("no logs found for workload %s in namespace %s", workload, namespace)
	}

	return strings.Join(allLogs, "\n\n"), nil
}

// PodLogs returns logs for a specific pod using the Kiali API endpoint.
// Parameters:
//   - namespace: the namespace containing the pod
//   - podName: the name of the pod
//   - container: container name (optional, will be auto-detected if not provided)
//   - workload: workload name (optional)
//   - duration: time duration (e.g., "5m", "1h") - optional
//   - maxLines: maximum number of lines to return - optional
func (k *Kiali) PodLogs(ctx context.Context, namespace string, podName string, container string, workload string, duration string, maxLines string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if podName == "" {
		return "", fmt.Errorf("pod name is required")
	}
	// Container is optional - will be auto-detected if not provided
	podContainer := container
	if podContainer == "" {
		// Get pod details to find containers
		podDetails, err := k.executeRequest(ctx, http.MethodGet, fmt.Sprintf(PodDetailsEndpoint, url.PathEscape(namespace), url.PathEscape(podName)), "", nil)
		if err != nil {
			return "", fmt.Errorf("failed to get pod details: %w", err)
		}

		// Parse pod details to extract container names
		var podData struct {
			Containers []struct {
				Name string `json:"name"`
			} `json:"containers"`
		}

		if err := json.Unmarshal([]byte(podDetails), &podData); err != nil {
			return "", fmt.Errorf("failed to parse pod details: %w", err)
		}

		// Find the main application container (not istio-proxy or istio-init)
		for _, c := range podData.Containers {
			if c.Name != "istio-proxy" && c.Name != "istio-init" {
				podContainer = c.Name
				break
			}
		}
		// If no app container found, use the first container
		if podContainer == "" && len(podData.Containers) > 0 {
			podContainer = podData.Containers[0].Name
		}

		if podContainer == "" {
			return "", fmt.Errorf("no container found for pod %s in namespace %s", podName, namespace)
		}
	}

	endpoint := fmt.Sprintf(PodsLogsEndpoint,
		url.PathEscape(namespace), url.PathEscape(podName))

	// Add query parameters
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()

	// Required parameters
	q.Set("container", podContainer)

	// Optional parameters
	if workload != "" {
		q.Set("workload", workload)
	}
	if duration != "" {
		q.Set("duration", duration)
	}
	if maxLines != "" {
		q.Set("maxLines", maxLines)
	}

	u.RawQuery = q.Encode()
	endpoint = u.String()

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}
