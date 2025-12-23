package core

import (
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// podHighRestartThreshold is the number of container restarts above which a pod
// is flagged as having issues, even if it's currently running without errors.
const podHighRestartThreshold = 5

// initHealthChecks initializes the cluster health check prompts
func initHealthChecks() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "cluster-health-check",
				Title:       "Cluster Health Check",
				Description: "Perform comprehensive health assessment of Kubernetes/OpenShift cluster",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Optional namespace to limit health check scope (default: all namespaces)",
						Required:    false,
					},
					{
						Name:        "check_events",
						Description: "Include recent warning/error events (true/false, default: true)",
						Required:    false,
					},
				},
			},
			Handler: clusterHealthCheckHandler,
		},
	}
}

// clusterHealthCheckHandler implements the cluster health check prompt
func clusterHealthCheckHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]
	checkEvents := args["check_events"] != "false" // default true

	klog.Info("Starting cluster health check...")

	// Check if namespace exists if specified
	namespaceWarning := ""
	requestedNamespace := namespace
	if namespace != "" {
		_, err := params.CoreV1().Namespaces().Get(params.Context, namespace, metav1.GetOptions{})
		if err != nil {
			// Namespace doesn't exist - show warning and proceed with cluster-wide check
			namespaceWarning = fmt.Sprintf("Namespace '%s' not found or not accessible. Showing cluster-wide information instead.", namespace)
			namespace = "" // Fall back to cluster-wide check
			klog.Warningf("Namespace '%s' not found, performing cluster-wide health check", requestedNamespace)
		} else {
			klog.Infof("Performing health check for namespace: %s", namespace)
		}
	} else {
		klog.Info("Performing cluster-wide health check")
	}

	diagnostics, err := gatherClusterDiagnostics(params, namespace, checkEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to gather cluster diagnostics: %w", err)
	}

	// Set namespace warning and requested namespace for display
	diagnostics.NamespaceWarning = namespaceWarning
	if requestedNamespace != "" && namespaceWarning != "" {
		diagnostics.TargetNamespace = requestedNamespace
		diagnostics.NamespaceScoped = false // Changed to cluster-wide due to error
	}

	// Format diagnostic data for LLM analysis
	promptText := formatHealthCheckPrompt(diagnostics)

	return api.NewPromptCallResult(
		"Cluster health diagnostic data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: promptText,
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: "I'll analyze the cluster health diagnostic data and provide a comprehensive assessment.",
				},
			},
		},
		nil,
	), nil
}

// clusterDiagnostics contains all diagnostic data gathered from the cluster
type clusterDiagnostics struct {
	Nodes            string
	Pods             string
	Deployments      string
	StatefulSets     string
	DaemonSets       string
	PVCs             string
	ClusterOperators string
	Events           string
	CollectionTime   time.Time
	TotalNamespaces  int
	NamespaceScoped  bool
	TargetNamespace  string
	NamespaceWarning string
}

// gatherClusterDiagnostics collects comprehensive diagnostic data from the cluster
func gatherClusterDiagnostics(params api.PromptHandlerParams, namespace string, checkEvents bool) (*clusterDiagnostics, error) {
	diag := &clusterDiagnostics{
		CollectionTime:  time.Now(),
		NamespaceScoped: namespace != "",
		TargetNamespace: namespace,
	}

	// Gather node diagnostics using ResourcesList
	klog.Info("Collecting node diagnostics...")
	nodeDiag, err := gatherNodeDiagnostics(params)
	if err == nil {
		diag.Nodes = nodeDiag
		klog.Info("Node diagnostics collected")
	} else {
		klog.Warningf("Failed to collect node diagnostics: %v", err)
	}

	// Gather pod diagnostics
	klog.Info("Collecting pod diagnostics...")
	podDiag, err := gatherPodDiagnostics(params, namespace)
	if err == nil {
		diag.Pods = podDiag
		klog.Info("Pod diagnostics collected")
	} else {
		klog.Warningf("Failed to collect pod diagnostics: %v", err)
	}

	// Gather workload diagnostics
	klog.Info("Collecting deployment diagnostics...")
	deployDiag, err := gatherWorkloadDiagnostics(params, "Deployment", namespace)
	if err == nil {
		diag.Deployments = deployDiag
		klog.Info("Deployment diagnostics collected")
	} else {
		klog.Warningf("Failed to collect deployment diagnostics: %v", err)
	}

	klog.Info("Collecting statefulset diagnostics...")
	stsDiag, err := gatherWorkloadDiagnostics(params, "StatefulSet", namespace)
	if err == nil {
		diag.StatefulSets = stsDiag
		klog.Info("StatefulSet diagnostics collected")
	} else {
		klog.Warningf("Failed to collect statefulset diagnostics: %v", err)
	}

	klog.Info("Collecting daemonset diagnostics...")
	dsDiag, err := gatherWorkloadDiagnostics(params, "DaemonSet", namespace)
	if err == nil {
		diag.DaemonSets = dsDiag
		klog.Info("DaemonSet diagnostics collected")
	} else {
		klog.Warningf("Failed to collect daemonset diagnostics: %v", err)
	}

	// Gather PVC diagnostics
	klog.Info("Collecting PVC diagnostics...")
	pvcDiag, err := gatherPVCDiagnostics(params, namespace)
	if err == nil {
		diag.PVCs = pvcDiag
		klog.Info("PVC diagnostics collected")
	} else {
		klog.Warningf("Failed to collect PVC diagnostics: %v", err)
	}

	// Gather cluster operator diagnostics (OpenShift only)
	klog.Info("Checking for cluster operators (OpenShift)...")
	operatorDiag, err := gatherClusterOperatorDiagnostics(params)
	if err == nil {
		diag.ClusterOperators = operatorDiag
		klog.Info("Cluster operator diagnostics collected")
	}

	// Gather recent events if requested
	if checkEvents {
		klog.Info("Collecting recent events...")
		eventDiag, err := gatherEventDiagnostics(params, namespace)
		if err == nil {
			diag.Events = eventDiag
			klog.Info("Event diagnostics collected")
		} else {
			klog.Warningf("Failed to collect event diagnostics: %v", err)
		}
	}

	// Count namespaces
	klog.Info("Counting namespaces...")
	namespaceList, err := params.CoreV1().Namespaces().List(params.Context, metav1.ListOptions{})
	if err == nil {
		diag.TotalNamespaces = len(namespaceList.Items)
		klog.Infof("Found %d namespaces", diag.TotalNamespaces)
	}

	klog.Info("Cluster health check data collection completed")
	return diag, nil
}

// gatherNodeDiagnostics collects node status using CoreV1 clientset
func gatherNodeDiagnostics(params api.PromptHandlerParams) (string, error) {
	nodeList, err := params.CoreV1().Nodes().List(params.Context, metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	if len(nodeList.Items) == 0 {
		return "No nodes found", nil
	}

	var sb strings.Builder
	totalNodes := len(nodeList.Items)
	healthyNodes := 0
	var nodesWithIssues []string

	for _, node := range nodeList.Items {
		nodeStatus := "Unknown"
		var issues []string

		// Parse node conditions
		for _, cond := range node.Status.Conditions {
			if cond.Type == v1.NodeReady {
				if cond.Status == v1.ConditionTrue {
					nodeStatus = "Ready"
					healthyNodes++
				} else {
					nodeStatus = "NotReady"
					issues = append(issues, fmt.Sprintf("Not ready: %s", cond.Message))
				}
			} else if cond.Status == v1.ConditionTrue {
				// Pressure conditions
				issues = append(issues, fmt.Sprintf("%s: %s", cond.Type, cond.Message))
			}
		}

		// Only report nodes with issues
		if len(issues) > 0 {
			nodesWithIssues = append(nodesWithIssues, fmt.Sprintf("- **%s** (Status: %s)\n%s", node.Name, nodeStatus, "  - "+strings.Join(issues, "\n  - ")))
		}
	}

	sb.WriteString(fmt.Sprintf("**Total:** %d | **Healthy:** %d\n\n", totalNodes, healthyNodes))
	if len(nodesWithIssues) > 0 {
		sb.WriteString(strings.Join(nodesWithIssues, "\n\n"))
	} else {
		sb.WriteString("*All nodes are healthy*")
	}

	return sb.String(), nil
}

// gatherPodDiagnostics collects pod status using CoreV1 clientset
func gatherPodDiagnostics(params api.PromptHandlerParams, namespace string) (string, error) {
	podList, err := params.CoreV1().Pods(namespace).List(params.Context, metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	if len(podList.Items) == 0 {
		return "No pods found", nil
	}

	totalPods := len(podList.Items)
	var problemPods []string

	for _, pod := range podList.Items {
		var issues []string
		restarts := int32(0)
		readyCount := 0
		totalContainers := len(pod.Status.ContainerStatuses)

		// Check container statuses
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
			restarts += cs.RestartCount

			// Check waiting state
			if cs.State.Waiting != nil {
				reason := cs.State.Waiting.Reason
				if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					issues = append(issues, fmt.Sprintf("Container waiting: %s - %s", reason, cs.State.Waiting.Message))
				}
			}

			// Check terminated state
			if cs.State.Terminated != nil {
				reason := cs.State.Terminated.Reason
				if reason == "Error" || reason == "OOMKilled" {
					issues = append(issues, fmt.Sprintf("Container terminated: %s", reason))
				}
			}
		}

		// Check pod phase
		if pod.Status.Phase != v1.PodRunning && pod.Status.Phase != v1.PodSucceeded {
			issues = append(issues, fmt.Sprintf("Pod in %s phase", pod.Status.Phase))
		}

		// Report pods with issues or high restart count
		if len(issues) > 0 || restarts > podHighRestartThreshold {
			problemPods = append(problemPods, fmt.Sprintf("- **%s/%s** (Phase: %s, Ready: %d/%d, Restarts: %d)\n  - %s",
				pod.Namespace, pod.Name, pod.Status.Phase, readyCount, totalContainers, restarts, strings.Join(issues, "\n  - ")))
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Total:** %d | **With Issues:** %d\n\n", totalPods, len(problemPods)))
	if len(problemPods) > 0 {
		sb.WriteString(strings.Join(problemPods, "\n\n"))
	} else {
		sb.WriteString("*No pod issues detected*")
	}

	return sb.String(), nil
}

// gatherWorkloadDiagnostics collects workload controller status using AppsV1 clientset
func gatherWorkloadDiagnostics(params api.PromptHandlerParams, kind string, namespace string) (string, error) {
	var workloadsWithIssues []string

	switch kind {
	case "Deployment":
		deploymentList, err := params.AppsV1().Deployments(namespace).List(params.Context, metav1.ListOptions{})
		if err != nil {
			return "", err
		}
		if len(deploymentList.Items) == 0 {
			return "No Deployments found", nil
		}

		for _, deployment := range deploymentList.Items {
			var issues []string
			ready := fmt.Sprintf("%d/%d", deployment.Status.ReadyReplicas, deployment.Status.Replicas)

			if deployment.Status.UnavailableReplicas > 0 {
				issues = append(issues, fmt.Sprintf("%d replicas unavailable", deployment.Status.UnavailableReplicas))
			}

			if len(issues) > 0 {
				workloadsWithIssues = append(workloadsWithIssues, fmt.Sprintf("- **%s/%s** (Ready: %s)\n  - %s",
					deployment.Namespace, deployment.Name, ready, strings.Join(issues, "\n  - ")))
			}
		}

	case "StatefulSet":
		statefulSetList, err := params.AppsV1().StatefulSets(namespace).List(params.Context, metav1.ListOptions{})
		if err != nil {
			return "", err
		}
		if len(statefulSetList.Items) == 0 {
			return "No StatefulSets found", nil
		}

		for _, sts := range statefulSetList.Items {
			var issues []string
			specReplicas := int32(1)
			if sts.Spec.Replicas != nil {
				specReplicas = *sts.Spec.Replicas
			}
			ready := fmt.Sprintf("%d/%d", sts.Status.ReadyReplicas, specReplicas)

			if sts.Status.ReadyReplicas < specReplicas {
				issues = append(issues, fmt.Sprintf("Only %d/%d replicas ready", sts.Status.ReadyReplicas, specReplicas))
			}

			if len(issues) > 0 {
				workloadsWithIssues = append(workloadsWithIssues, fmt.Sprintf("- **%s/%s** (Ready: %s)\n  - %s",
					sts.Namespace, sts.Name, ready, strings.Join(issues, "\n  - ")))
			}
		}

	case "DaemonSet":
		daemonSetList, err := params.AppsV1().DaemonSets(namespace).List(params.Context, metav1.ListOptions{})
		if err != nil {
			return "", err
		}
		if len(daemonSetList.Items) == 0 {
			return "No DaemonSets found", nil
		}

		for _, ds := range daemonSetList.Items {
			var issues []string
			ready := fmt.Sprintf("%d/%d", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)

			if ds.Status.NumberUnavailable > 0 {
				issues = append(issues, fmt.Sprintf("%d pods unavailable", ds.Status.NumberUnavailable))
			}

			if len(issues) > 0 {
				workloadsWithIssues = append(workloadsWithIssues, fmt.Sprintf("- **%s/%s** (Ready: %s)\n  - %s",
					ds.Namespace, ds.Name, ready, strings.Join(issues, "\n  - ")))
			}
		}

	default:
		return "", fmt.Errorf("unsupported workload kind: %s", kind)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%ss with Issues:** %d\n\n", kind, len(workloadsWithIssues)))
	if len(workloadsWithIssues) > 0 {
		sb.WriteString(strings.Join(workloadsWithIssues, "\n\n"))
	} else {
		sb.WriteString(fmt.Sprintf("*No %s issues detected*", kind))
	}

	return sb.String(), nil
}

// gatherPVCDiagnostics collects PVC status using CoreV1 clientset
func gatherPVCDiagnostics(params api.PromptHandlerParams, namespace string) (string, error) {
	pvcList, err := params.CoreV1().PersistentVolumeClaims(namespace).List(params.Context, metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	if len(pvcList.Items) == 0 {
		return "No PVCs found", nil
	}

	var pvcsWithIssues []string

	for _, pvc := range pvcList.Items {
		if pvc.Status.Phase != v1.ClaimBound {
			pvcsWithIssues = append(pvcsWithIssues, fmt.Sprintf("- **%s/%s** (Status: %s)\n  - PVC not bound",
				pvc.Namespace, pvc.Name, pvc.Status.Phase))
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**PVCs with Issues:** %d\n\n", len(pvcsWithIssues)))
	if len(pvcsWithIssues) > 0 {
		sb.WriteString(strings.Join(pvcsWithIssues, "\n\n"))
	} else {
		sb.WriteString("*No PVC issues detected*")
	}

	return sb.String(), nil
}

// gatherClusterOperatorDiagnostics collects ClusterOperator status (OpenShift only)
func gatherClusterOperatorDiagnostics(params api.PromptHandlerParams) (string, error) {
	gvk := &schema.GroupVersionKind{
		Group:   "config.openshift.io",
		Version: "v1",
		Kind:    "ClusterOperator",
	}

	operatorList, err := kubernetes.NewCore(params).ResourcesList(params, gvk, "", api.ListOptions{})
	if err != nil {
		// Not an OpenShift cluster
		return "", err
	}

	items, ok := operatorList.UnstructuredContent()["items"].([]interface{})
	if !ok || len(items) == 0 {
		return "No cluster operators found", nil
	}

	var operatorsWithIssues []string

	for _, item := range items {
		opMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		metadata, _ := opMap["metadata"].(map[string]interface{})
		name, _ := metadata["name"].(string)

		status, _ := opMap["status"].(map[string]interface{})
		conditions, _ := status["conditions"].([]interface{})

		available := "Unknown"
		degraded := "Unknown"
		var issues []string

		for _, cond := range conditions {
			condMap, _ := cond.(map[string]interface{})
			condType, _ := condMap["type"].(string)
			condStatus, _ := condMap["status"].(string)
			message, _ := condMap["message"].(string)

			switch condType {
			case "Available":
				available = condStatus
				if condStatus != "True" {
					issues = append(issues, fmt.Sprintf("Not available: %s", message))
				}
			case "Degraded":
				degraded = condStatus
				if condStatus == "True" {
					issues = append(issues, fmt.Sprintf("Degraded: %s", message))
				}
			}
		}

		if len(issues) > 0 {
			operatorsWithIssues = append(operatorsWithIssues, fmt.Sprintf("- **%s** (Available: %s, Degraded: %s)\n  - %s",
				name, available, degraded, strings.Join(issues, "\n  - ")))
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Operators with Issues:** %d\n\n", len(operatorsWithIssues)))
	if len(operatorsWithIssues) > 0 {
		sb.WriteString(strings.Join(operatorsWithIssues, "\n\n"))
	} else {
		sb.WriteString("*All cluster operators are healthy*")
	}

	return sb.String(), nil
}

// gatherEventDiagnostics collects recent warning and error events
func gatherEventDiagnostics(params api.PromptHandlerParams, namespace string) (string, error) {
	var namespaces []string

	if namespace != "" {
		namespaces = append(namespaces, namespace)
	} else {
		// Important namespaces
		namespaces = []string{"default", "kube-system"}

		// Add OpenShift namespaces using typed clientset
		nsList, err := params.CoreV1().Namespaces().List(params.Context, metav1.ListOptions{})
		if err == nil {
			for _, ns := range nsList.Items {
				if strings.HasPrefix(ns.Name, "openshift-") {
					namespaces = append(namespaces, ns.Name)
				}
			}
		}
	}

	oneHourAgo := time.Now().Add(-1 * time.Hour)
	totalWarnings := 0
	totalErrors := 0
	var recentEvents []string

	for _, ns := range namespaces {
		eventList, err := params.CoreV1().Events(ns).List(params.Context, metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, event := range eventList.Items {
			// Only include Warning and Error events
			if event.Type != v1.EventTypeWarning && event.Type != "Error" {
				continue
			}

			// Check timestamp
			lastSeenTime := event.LastTimestamp.Time
			if lastSeenTime.IsZero() {
				lastSeenTime = event.EventTime.Time
			}
			if lastSeenTime.Before(oneHourAgo) {
				continue
			}

			if event.Type == v1.EventTypeWarning {
				totalWarnings++
			} else {
				totalErrors++
			}

			// Limit message length
			message := event.Message
			if len(message) > 150 {
				message = message[:150] + "..."
			}

			recentEvents = append(recentEvents, fmt.Sprintf("- **%s/%s** in `%s` (%s, Count: %d)\n  - %s",
				event.InvolvedObject.Kind, event.InvolvedObject.Name, ns, event.Reason, event.Count, message))
		}
	}

	// Limit to 20 most recent events
	if len(recentEvents) > 20 {
		recentEvents = recentEvents[:20]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Warnings:** %d | **Errors:** %d\n\n", totalWarnings, totalErrors))
	if len(recentEvents) > 0 {
		sb.WriteString(strings.Join(recentEvents, "\n\n"))
	} else {
		sb.WriteString("*No recent warning/error events*")
	}

	return sb.String(), nil
}

// formatHealthCheckPrompt formats diagnostic data into a prompt for LLM analysis
func formatHealthCheckPrompt(diag *clusterDiagnostics) string {
	var sb strings.Builder

	sb.WriteString("# Cluster Health Check Diagnostic Data\n\n")
	sb.WriteString(fmt.Sprintf("**Collection Time:** %s\n", diag.CollectionTime.Format(time.RFC3339)))

	// Show namespace warning prominently if present
	if diag.NamespaceWarning != "" {
		sb.WriteString("\n")
		sb.WriteString("⚠️  **WARNING:** " + diag.NamespaceWarning + "\n")
		sb.WriteString("\n")
		sb.WriteString("**Note:** Please verify the namespace name and try again if you want namespace-specific diagnostics.\n")
	}

	if diag.NamespaceScoped {
		sb.WriteString(fmt.Sprintf("**Scope:** Namespace `%s`\n", diag.TargetNamespace))
	} else {
		sb.WriteString(fmt.Sprintf("**Scope:** All namespaces (Total: %d)\n", diag.TotalNamespaces))
	}
	sb.WriteString("\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Analyze the following cluster diagnostic data and provide:\n")
	sb.WriteString("1. **Overall Health Status**: Healthy, Warning, or Critical\n")
	sb.WriteString("2. **Critical Issues**: Issues requiring immediate attention\n")
	sb.WriteString("3. **Warnings**: Non-critical issues that should be addressed\n")
	sb.WriteString("4. **Recommendations**: Suggested actions to improve cluster health\n")
	sb.WriteString("5. **Summary**: Brief overview of findings by component\n\n")

	sb.WriteString("---\n\n")

	if diag.Nodes != "" {
		sb.WriteString("## 1. Nodes\n\n")
		sb.WriteString(diag.Nodes)
		sb.WriteString("\n\n")
	}

	if diag.ClusterOperators != "" {
		sb.WriteString("## 2. Cluster Operators (OpenShift)\n\n")
		sb.WriteString(diag.ClusterOperators)
		sb.WriteString("\n\n")
	}

	if diag.Pods != "" {
		sb.WriteString("## 3. Pods\n\n")
		sb.WriteString(diag.Pods)
		sb.WriteString("\n\n")
	}

	if diag.Deployments != "" || diag.StatefulSets != "" || diag.DaemonSets != "" {
		sb.WriteString("## 4. Workload Controllers\n\n")
		if diag.Deployments != "" {
			sb.WriteString("### Deployments\n\n")
			sb.WriteString(diag.Deployments)
			sb.WriteString("\n\n")
		}
		if diag.StatefulSets != "" {
			sb.WriteString("### StatefulSets\n\n")
			sb.WriteString(diag.StatefulSets)
			sb.WriteString("\n\n")
		}
		if diag.DaemonSets != "" {
			sb.WriteString("### DaemonSets\n\n")
			sb.WriteString(diag.DaemonSets)
			sb.WriteString("\n\n")
		}
	}

	if diag.PVCs != "" {
		sb.WriteString("## 5. Persistent Volume Claims\n\n")
		sb.WriteString(diag.PVCs)
		sb.WriteString("\n\n")
	}

	if diag.Events != "" {
		sb.WriteString("## 6. Recent Events (Last Hour)\n\n")
		sb.WriteString(diag.Events)
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString("**Please analyze the above diagnostic data and provide your comprehensive health assessment.**\n")

	return sb.String()
}
