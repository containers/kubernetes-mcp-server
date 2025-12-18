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

// clusterHealthCheckHandler implements the cluster health check prompt
func clusterHealthCheckHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	// Parse arguments (GetArguments returns map[string]string for prompts)
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

	// Gather cluster diagnostics using the KubernetesClient interface
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
	namespaceList, err := kubernetes.NewCore(params).NamespacesList(params, api.ListOptions{})
	if err == nil {
		if items, ok := namespaceList.UnstructuredContent()["items"].([]interface{}); ok {
			diag.TotalNamespaces = len(items)
			klog.Infof("Found %d namespaces", diag.TotalNamespaces)
		}
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
	nodesWithIssues := []string{}

	for _, node := range nodeList.Items {
		nodeStatus := "Unknown"
		issues := []string{}

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

// gatherPodDiagnostics collects pod status using existing methods
func gatherPodDiagnostics(params api.PromptHandlerParams, namespace string) (string, error) {
	var podList interface{ UnstructuredContent() map[string]interface{} }
	var err error

	if namespace != "" {
		podList, err = kubernetes.NewCore(params).PodsListInNamespace(params, namespace, api.ListOptions{})
	} else {
		podList, err = kubernetes.NewCore(params).PodsListInAllNamespaces(params, api.ListOptions{})
	}

	if err != nil {
		return "", err
	}

	items, ok := podList.UnstructuredContent()["items"].([]interface{})
	if !ok {
		return "No pods found", nil
	}

	totalPods := len(items)
	problemPods := []string{}

	for _, item := range items {
		podMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		metadata, _ := podMap["metadata"].(map[string]interface{})
		name, _ := metadata["name"].(string)
		ns, _ := metadata["namespace"].(string)

		status, _ := podMap["status"].(map[string]interface{})
		phase, _ := status["phase"].(string)
		containerStatuses, _ := status["containerStatuses"].([]interface{})

		issues := []string{}
		restarts := int32(0)
		readyCount := 0
		totalContainers := len(containerStatuses)

		// Check container statuses
		for _, cs := range containerStatuses {
			csMap, _ := cs.(map[string]interface{})
			ready, _ := csMap["ready"].(bool)
			restartCount, _ := csMap["restartCount"].(float64)
			restarts += int32(restartCount)

			if ready {
				readyCount++
			}

			state, _ := csMap["state"].(map[string]interface{})
			if waiting, ok := state["waiting"].(map[string]interface{}); ok {
				reason, _ := waiting["reason"].(string)
				message, _ := waiting["message"].(string)
				if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					issues = append(issues, fmt.Sprintf("Container waiting: %s - %s", reason, message))
				}
			}
			if terminated, ok := state["terminated"].(map[string]interface{}); ok {
				reason, _ := terminated["reason"].(string)
				if reason == "Error" || reason == "OOMKilled" {
					issues = append(issues, fmt.Sprintf("Container terminated: %s", reason))
				}
			}
		}

		// Check pod phase
		if phase != "Running" && phase != "Succeeded" {
			issues = append(issues, fmt.Sprintf("Pod in %s phase", phase))
		}

		// Report pods with issues or high restart count
		if len(issues) > 0 || restarts > 5 {
			problemPods = append(problemPods, fmt.Sprintf("- **%s/%s** (Phase: %s, Ready: %d/%d, Restarts: %d)\n  - %s",
				ns, name, phase, readyCount, totalContainers, restarts, strings.Join(issues, "\n  - ")))
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

// gatherWorkloadDiagnostics collects workload controller status
func gatherWorkloadDiagnostics(params api.PromptHandlerParams, kind string, namespace string) (string, error) {
	gvk := &schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kind,
	}

	workloadList, err := kubernetes.NewCore(params).ResourcesList(params, gvk, namespace, api.ListOptions{})
	if err != nil {
		return "", err
	}

	items, ok := workloadList.UnstructuredContent()["items"].([]interface{})
	if !ok || len(items) == 0 {
		return fmt.Sprintf("No %ss found", kind), nil
	}

	workloadsWithIssues := []string{}

	for _, item := range items {
		workloadMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		metadata, _ := workloadMap["metadata"].(map[string]interface{})
		name, _ := metadata["name"].(string)
		ns, _ := metadata["namespace"].(string)

		status, _ := workloadMap["status"].(map[string]interface{})
		spec, _ := workloadMap["spec"].(map[string]interface{})
		issues := []string{}
		ready := "Unknown"

		switch kind {
		case "Deployment":
			replicas, _ := status["replicas"].(float64)
			readyReplicas, _ := status["readyReplicas"].(float64)
			unavailableReplicas, _ := status["unavailableReplicas"].(float64)

			ready = fmt.Sprintf("%d/%d", int(readyReplicas), int(replicas))

			if unavailableReplicas > 0 {
				issues = append(issues, fmt.Sprintf("%d replicas unavailable", int(unavailableReplicas)))
			}

		case "StatefulSet":
			specReplicas, _ := spec["replicas"].(float64)
			readyReplicas, _ := status["readyReplicas"].(float64)

			ready = fmt.Sprintf("%d/%d", int(readyReplicas), int(specReplicas))

			if readyReplicas < specReplicas {
				issues = append(issues, fmt.Sprintf("Only %d/%d replicas ready", int(readyReplicas), int(specReplicas)))
			}

		case "DaemonSet":
			desiredNumberScheduled, _ := status["desiredNumberScheduled"].(float64)
			numberReady, _ := status["numberReady"].(float64)
			numberUnavailable, _ := status["numberUnavailable"].(float64)

			ready = fmt.Sprintf("%d/%d", int(numberReady), int(desiredNumberScheduled))

			if numberUnavailable > 0 {
				issues = append(issues, fmt.Sprintf("%d pods unavailable", int(numberUnavailable)))
			}
		}

		if len(issues) > 0 {
			workloadsWithIssues = append(workloadsWithIssues, fmt.Sprintf("- **%s/%s** (Ready: %s)\n  - %s",
				ns, name, ready, strings.Join(issues, "\n  - ")))
		}
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

// gatherPVCDiagnostics collects PVC status
func gatherPVCDiagnostics(params api.PromptHandlerParams, namespace string) (string, error) {
	gvk := &schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "PersistentVolumeClaim",
	}

	pvcList, err := kubernetes.NewCore(params).ResourcesList(params, gvk, namespace, api.ListOptions{})
	if err != nil {
		return "", err
	}

	items, ok := pvcList.UnstructuredContent()["items"].([]interface{})
	if !ok || len(items) == 0 {
		return "No PVCs found", nil
	}

	pvcsWithIssues := []string{}

	for _, item := range items {
		pvcMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		metadata, _ := pvcMap["metadata"].(map[string]interface{})
		name, _ := metadata["name"].(string)
		ns, _ := metadata["namespace"].(string)

		status, _ := pvcMap["status"].(map[string]interface{})
		phase, _ := status["phase"].(string)

		if phase != "Bound" {
			pvcsWithIssues = append(pvcsWithIssues, fmt.Sprintf("- **%s/%s** (Status: %s)\n  - PVC not bound", ns, name, phase))
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

	operatorsWithIssues := []string{}

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
		issues := []string{}

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
	namespaces := []string{}

	if namespace != "" {
		namespaces = append(namespaces, namespace)
	} else {
		// Important namespaces
		namespaces = []string{"default", "kube-system"}

		// Add OpenShift namespaces
		nsList, err := kubernetes.NewCore(params).NamespacesList(params, api.ListOptions{})
		if err == nil {
			if items, ok := nsList.UnstructuredContent()["items"].([]interface{}); ok {
				for _, item := range items {
					nsMap, ok := item.(map[string]interface{})
					if !ok {
						continue
					}
					metadata, _ := nsMap["metadata"].(map[string]interface{})
					name, _ := metadata["name"].(string)
					if strings.HasPrefix(name, "openshift-") {
						namespaces = append(namespaces, name)
					}
				}
			}
		}
	}

	oneHourAgo := time.Now().Add(-1 * time.Hour)
	totalWarnings := 0
	totalErrors := 0
	recentEvents := []string{}

	for _, ns := range namespaces {
		eventList, err := params.CoreV1().Events(ns).List(params.Context, metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, event := range eventList.Items {
			// Only include Warning and Error events
			if event.Type != string(v1.EventTypeWarning) && event.Type != "Error" {
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

			if event.Type == string(v1.EventTypeWarning) {
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
