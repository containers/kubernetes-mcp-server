package core

import (
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// podHighRestartThreshold is the number of container restarts above which a pod
// is flagged as having issues, even if it's currently running without errors.
const podHighRestartThreshold = 5

// initHealthChecks initializes the cluster health check prompts
func initHealthChecks() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "cluster-health-check",
				Title:       "Cluster Health Check",
				Description: "Perform comprehensive health assessment of Kubernetes/OpenShift cluster",
				Arguments: []config.PromptArgument{
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
			RBAC: api.RBACBounded(
				api.RBACRequirement{
					Verbs:  []string{"list"},
					Target: api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "nodes"}},
				},
				api.RBACRequirement{
					Verbs:  []string{"get", "list"},
					Target: api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "namespaces"}},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "pods"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "apps", Resource: "deployments"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "apps", Resource: "statefulsets"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "apps", Resource: "daemonsets"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "persistentvolumeclaims"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs: []string{"list"},
					Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
						APIGroup: "config.openshift.io",
						Resource: "clusteroperators",
					}},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "events"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
			),
			Handler: clusterHealthCheckHandler,
		},
	}
}

// clusterHealthCheckHandler implements the cluster health check prompt
func clusterHealthCheckHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]
	checkEvents := args["check_events"] != "false" // default true

	logger := klogutil.FromContext(params.Context)
	logger.Info("Starting cluster health check...")

	// Check if namespace exists if specified
	namespaceWarning := ""
	requestedNamespace := namespace
	if namespace != "" {
		_, err := params.CoreV1().Namespaces().Get(params.Context, namespace, metav1.GetOptions{})
		if err != nil {
			// Namespace doesn't exist - show warning and proceed with cluster-wide check
			namespaceWarning = fmt.Sprintf("Namespace '%s' not found or not accessible. Showing cluster-wide information instead.", namespace)
			namespace = "" // Fall back to cluster-wide check
			klogutil.LogWarn(logger, "Namespace not found, performing cluster-wide health check",
				klogutil.Field("kubernetes.namespace.name", requestedNamespace),
			)
		} else {
			logger.Info("Performing health check for namespace", "kubernetes.namespace.name", namespace)
		}
	} else {
		logger.Info("Performing cluster-wide health check")
	}

	return api.NewPromptCallResult(
		"Cluster health diagnostic data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: buildHealthCheckPrompt(params, namespace, namespaceWarning, checkEvents),
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: healthCheckAssistantText,
				},
			},
		},
		nil,
	), nil
}

// healthCheckAssistantText counts toward the same result as the user prompt.
const healthCheckAssistantText = "I'll analyze the cluster health diagnostic data and provide a comprehensive assessment."

// clusterDiagnostics is the preamble data known before section lists run.
type clusterDiagnostics struct {
	CollectionTime   time.Time
	TotalNamespaces  int
	NamespaceScoped  bool
	TargetNamespace  string
	NamespaceWarning string
}

// buildHealthCheckPrompt gathers each section into one result budget. A section
// List does not start once the bytes already kept fill that budget. The
// assistant message is reserved up front because it counts toward the same
// result. See docs/specs/memory-bounds.md#retention.
func buildHealthCheckPrompt(params api.PromptHandlerParams, namespace, namespaceWarning string, checkEvents bool) string {
	diag := &clusterDiagnostics{
		CollectionTime:   time.Now(),
		NamespaceScoped:  namespace != "",
		TargetNamespace:  namespace,
		NamespaceWarning: namespaceWarning,
	}
	b := api.NewPromptBudget(params.Context, healthCheckAssistantText)
	logger := klogutil.FromContext(params.Context)

	var knownNS []v1.Namespace
	nsFetched := false
	if !b.Truncated() {
		logger.Info("Counting namespaces...")
		namespaceList, err := params.CoreV1().Namespaces().List(params.Context, metav1.ListOptions{})
		if err == nil {
			nsFetched = true
			knownNS = namespaceList.Items
			diag.TotalNamespaces = len(knownNS)
			logger.Info("Found namespaces", "kubernetes.namespaces.count", diag.TotalNamespaces)
		}
	}

	writeHealthCheckPreamble(b, diag)

	if body := beginSection(b, "## 1. Nodes\n\n"); body != nil {
		logger.Info("Collecting node diagnostics...")
		err := gatherNodeDiagnostics(params, body)
		if err == nil {
			commitSection(b, "## 1. Nodes\n\n", body)
			logger.Info("Node diagnostics collected")
		} else {
			klogutil.LogWarn(logger, "Failed to collect node diagnostics", klogutil.Err(err))
		}
	}

	if body := beginSection(b, "## 2. Cluster Operators (OpenShift)\n\n"); body != nil {
		logger.Info("Checking for cluster operators (OpenShift)...")
		err := gatherClusterOperatorDiagnostics(params, body)
		if err == nil {
			commitSection(b, "## 2. Cluster Operators (OpenShift)\n\n", body)
			logger.Info("Cluster operator diagnostics collected")
		}
	}

	if body := beginSection(b, "## 3. Pods\n\n"); body != nil {
		logger.Info("Collecting pod diagnostics...")
		err := gatherPodDiagnostics(params, namespace, body)
		if err == nil {
			commitSection(b, "## 3. Pods\n\n", body)
			logger.Info("Pod diagnostics collected")
		} else {
			klogutil.LogWarn(logger, "Failed to collect pod diagnostics", klogutil.Err(err))
		}
	}

	writeWorkloadSections(b, params, namespace)

	if body := beginSection(b, "## 5. Persistent Volume Claims\n\n"); body != nil {
		logger.Info("Collecting PVC diagnostics...")
		err := gatherPVCDiagnostics(params, namespace, body)
		if err == nil {
			commitSection(b, "## 5. Persistent Volume Claims\n\n", body)
			logger.Info("PVC diagnostics collected")
		} else {
			klogutil.LogWarn(logger, "Failed to collect PVC diagnostics", klogutil.Err(err))
		}
	}

	header := "## 6. Recent Events (Last Hour)\n\n"
	if checkEvents && b.Fits(len(header)) {
		logger.Info("Collecting recent events...")
		body := b.Scratch(len(header) + len("\n\n"))
		writeEventBody(params, namespace, nsFetched, knownNS, body)
		if b.CommitScratch(header, "\n\n", body) {
			logger.Info("Event diagnostics collected")
		}
	}

	if !b.Truncated() {
		b.WriteString("---\n\n")
		b.WriteString("**Please analyze the above diagnostic data and provide your comprehensive health assessment.**\n")
	}
	logger.Info("Cluster health check data collection completed")
	return b.String()
}

func beginSection(b *api.BuilderWithBudget, header string) *api.BuilderWithBudget {
	if b == nil || !b.Fits(len(header)) {
		return nil
	}
	body := b.Scratch(len(header) + len("\n\n"))
	if body.Truncated() {
		return nil
	}
	return body
}

func commitSection(b *api.BuilderWithBudget, header string, body *api.BuilderWithBudget) {
	b.CommitScratch(header, "\n\n", body)
}

// sectionSummaryReserve leaves room for a short totals line in front of the
// issue blocks. Callers also reserve ResultTruncationNotice so copying a cut
// scratch back does not append a second notice.
const sectionSummaryReserve = 80

func appendIssueBlock(lines *api.BuilderWithBudget, block string) {
	if lines == nil || lines.Truncated() || block == "" {
		return
	}
	if lines.Len() > 0 {
		lines.WriteString("\n\n")
		if lines.Truncated() {
			return
		}
	}
	lines.WriteString(block)
}

func writeSummaryAndBlocks(b, lines *api.BuilderWithBudget, summary, emptyNote string) {
	b.WriteString(summary)
	if b.Truncated() {
		return
	}
	if lines == nil || lines.Len() == 0 {
		b.WriteString(emptyNote)
		return
	}
	b.WriteString(lines.String())
}

func writeWorkloadSections(b *api.BuilderWithBudget, params api.PromptHandlerParams, namespace string) {
	logger := klogutil.FromContext(params.Context)
	wroteHeader := false
	writeOne := func(title, kind, logName string) {
		header := "### " + title + "\n\n"
		need := header
		reserve := len(header) + len("\n\n")
		if !wroteHeader {
			need = "## 4. Workload Controllers\n\n" + header
			reserve += len("## 4. Workload Controllers\n\n")
		}
		if !b.Fits(len(need)) {
			return
		}
		body := b.Scratch(reserve)
		if body.Truncated() {
			return
		}
		logger.Info("Collecting " + logName + " diagnostics...")
		err := gatherWorkloadDiagnostics(params, kind, namespace, body)
		if err != nil {
			klogutil.LogWarn(logger, "Failed to collect "+logName+" diagnostics", klogutil.Err(err))
			return
		}
		if body.Len() == 0 {
			return
		}
		if !wroteHeader {
			b.WriteString("## 4. Workload Controllers\n\n")
			wroteHeader = true
		}
		b.CommitScratch(header, "\n\n", body)
		logger.Info(title + " diagnostics collected")
	}
	writeOne("Deployments", "Deployment", "deployment")
	writeOne("StatefulSets", "StatefulSet", "statefulset")
	writeOne("DaemonSets", "DaemonSet", "daemonset")
}

// gatherNodeDiagnostics collects node status using CoreV1 clientset.
// Issue lines share b. Further lines are not formatted once that budget is
// full. Totals still count the rest of the already-fetched list.
func gatherNodeDiagnostics(params api.PromptHandlerParams, b *api.BuilderWithBudget) error {
	if b == nil || b.Truncated() {
		return nil
	}
	nodeList, err := params.CoreV1().Nodes().List(params.Context, metav1.ListOptions{})
	if err != nil {
		return err
	}

	if len(nodeList.Items) == 0 {
		b.WriteString("No nodes found")
		return nil
	}

	lines := b.Scratch(sectionSummaryReserve + len(api.ResultTruncationNotice))
	totalNodes := len(nodeList.Items)
	healthyNodes := 0
	withIssues := 0

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
		if len(issues) == 0 {
			continue
		}
		withIssues++
		if lines.Truncated() {
			continue
		}
		appendIssueBlock(lines, fmt.Sprintf("- **%s** (Status: %s)\n%s", node.Name, nodeStatus, "  - "+strings.Join(issues, "\n  - ")))
	}

	note := ""
	if withIssues == 0 {
		note = "*All nodes are healthy*"
	}
	writeSummaryAndBlocks(b, lines, fmt.Sprintf("**Total:** %d | **Healthy:** %d\n\n", totalNodes, healthyNodes), note)
	return nil
}

// gatherPodDiagnostics collects pod status using CoreV1 clientset.
// Issue lines share b. Further lines are not formatted once that budget is
// full. Totals still count the rest of the already-fetched list.
func gatherPodDiagnostics(params api.PromptHandlerParams, namespace string, b *api.BuilderWithBudget) error {
	if b == nil || b.Truncated() {
		return nil
	}
	podList, err := params.CoreV1().Pods(namespace).List(params.Context, metav1.ListOptions{})
	if err != nil {
		return err
	}

	if len(podList.Items) == 0 {
		b.WriteString("No pods found")
		return nil
	}

	lines := b.Scratch(sectionSummaryReserve + len(api.ResultTruncationNotice))
	totalPods := len(podList.Items)
	withIssues := 0

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
		if len(issues) == 0 && restarts <= podHighRestartThreshold {
			continue
		}
		withIssues++
		if lines.Truncated() {
			continue
		}
		appendIssueBlock(lines, fmt.Sprintf("- **%s/%s** (Phase: %s, Ready: %d/%d, Restarts: %d)\n  - %s",
			pod.Namespace, pod.Name, pod.Status.Phase, readyCount, totalContainers, restarts, strings.Join(issues, "\n  - ")))
	}

	note := ""
	if withIssues == 0 {
		note = "*No pod issues detected*"
	}
	writeSummaryAndBlocks(b, lines, fmt.Sprintf("**Total:** %d | **With Issues:** %d\n\n", totalPods, withIssues), note)
	return nil
}

// gatherWorkloadDiagnostics collects workload controller status using AppsV1 clientset.
// Issue lines share b. Further lines are not formatted once that budget is
// full. Totals still count the rest of the already-fetched list.
func gatherWorkloadDiagnostics(params api.PromptHandlerParams, kind, namespace string, b *api.BuilderWithBudget) error {
	if b == nil || b.Truncated() {
		return nil
	}
	lines := b.Scratch(sectionSummaryReserve + len(api.ResultTruncationNotice))
	withIssues := 0
	record := func(block string) {
		withIssues++
		if lines.Truncated() {
			return
		}
		appendIssueBlock(lines, block)
	}

	switch kind {
	case "Deployment":
		deploymentList, err := params.AppsV1().Deployments(namespace).List(params.Context, metav1.ListOptions{})
		if err != nil {
			return err
		}
		if len(deploymentList.Items) == 0 {
			b.WriteString("No Deployments found")
			return nil
		}

		for _, deployment := range deploymentList.Items {
			var issues []string
			ready := fmt.Sprintf("%d/%d", deployment.Status.ReadyReplicas, deployment.Status.Replicas)

			if deployment.Status.UnavailableReplicas > 0 {
				issues = append(issues, fmt.Sprintf("%d replicas unavailable", deployment.Status.UnavailableReplicas))
			}

			if len(issues) > 0 {
				record(fmt.Sprintf("- **%s/%s** (Ready: %s)\n  - %s",
					deployment.Namespace, deployment.Name, ready, strings.Join(issues, "\n  - ")))
			}
		}

	case "StatefulSet":
		statefulSetList, err := params.AppsV1().StatefulSets(namespace).List(params.Context, metav1.ListOptions{})
		if err != nil {
			return err
		}
		if len(statefulSetList.Items) == 0 {
			b.WriteString("No StatefulSets found")
			return nil
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
				record(fmt.Sprintf("- **%s/%s** (Ready: %s)\n  - %s",
					sts.Namespace, sts.Name, ready, strings.Join(issues, "\n  - ")))
			}
		}

	case "DaemonSet":
		daemonSetList, err := params.AppsV1().DaemonSets(namespace).List(params.Context, metav1.ListOptions{})
		if err != nil {
			return err
		}
		if len(daemonSetList.Items) == 0 {
			b.WriteString("No DaemonSets found")
			return nil
		}

		for _, ds := range daemonSetList.Items {
			var issues []string
			ready := fmt.Sprintf("%d/%d", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)

			if ds.Status.NumberUnavailable > 0 {
				issues = append(issues, fmt.Sprintf("%d pods unavailable", ds.Status.NumberUnavailable))
			}

			if len(issues) > 0 {
				record(fmt.Sprintf("- **%s/%s** (Ready: %s)\n  - %s",
					ds.Namespace, ds.Name, ready, strings.Join(issues, "\n  - ")))
			}
		}

	default:
		return fmt.Errorf("unsupported workload kind: %s", kind)
	}

	note := ""
	if withIssues == 0 {
		note = fmt.Sprintf("*No %s issues detected*", kind)
	}
	writeSummaryAndBlocks(b, lines, fmt.Sprintf("**%ss with Issues:** %d\n\n", kind, withIssues), note)
	return nil
}

// gatherPVCDiagnostics collects PVC status using CoreV1 clientset.
// Issue lines share b. Further lines are not formatted once that budget is
// full. Totals still count the rest of the already-fetched list.
func gatherPVCDiagnostics(params api.PromptHandlerParams, namespace string, b *api.BuilderWithBudget) error {
	if b == nil || b.Truncated() {
		return nil
	}
	pvcList, err := params.CoreV1().PersistentVolumeClaims(namespace).List(params.Context, metav1.ListOptions{})
	if err != nil {
		return err
	}

	if len(pvcList.Items) == 0 {
		b.WriteString("No PVCs found")
		return nil
	}

	lines := b.Scratch(sectionSummaryReserve + len(api.ResultTruncationNotice))
	withIssues := 0

	for _, pvc := range pvcList.Items {
		if pvc.Status.Phase == v1.ClaimBound {
			continue
		}
		withIssues++
		if lines.Truncated() {
			continue
		}
		appendIssueBlock(lines, fmt.Sprintf("- **%s/%s** (Status: %s)\n  - PVC not bound",
			pvc.Namespace, pvc.Name, pvc.Status.Phase))
	}

	note := ""
	if withIssues == 0 {
		note = "*No PVC issues detected*"
	}
	writeSummaryAndBlocks(b, lines, fmt.Sprintf("**PVCs with Issues:** %d\n\n", withIssues), note)
	return nil
}

// gatherClusterOperatorDiagnostics collects ClusterOperator status (OpenShift only).
// Issue lines share b. Further lines are not formatted once that budget is
// full. Totals still count the rest of the already-fetched list.
func gatherClusterOperatorDiagnostics(params api.PromptHandlerParams, b *api.BuilderWithBudget) error {
	if b == nil || b.Truncated() {
		return nil
	}
	gvk := &schema.GroupVersionKind{
		Group:   "config.openshift.io",
		Version: "v1",
		Kind:    "ClusterOperator",
	}

	operatorList, err := kubernetes.NewCore(params).ResourcesList(params, gvk, "", api.ListOptions{})
	if err != nil {
		// Not an OpenShift cluster
		return err
	}

	items, ok := operatorList.UnstructuredContent()["items"].([]interface{})
	if !ok || len(items) == 0 {
		b.WriteString("No cluster operators found")
		return nil
	}

	lines := b.Scratch(sectionSummaryReserve + len(api.ResultTruncationNotice))
	withIssues := 0

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

		if len(issues) == 0 {
			continue
		}
		withIssues++
		if lines.Truncated() {
			continue
		}
		appendIssueBlock(lines, fmt.Sprintf("- **%s** (Available: %s, Degraded: %s)\n  - %s",
			name, available, degraded, strings.Join(issues, "\n  - ")))
	}

	note := ""
	if withIssues == 0 {
		note = "*All cluster operators are healthy*"
	}
	writeSummaryAndBlocks(b, lines, fmt.Sprintf("**Operators with Issues:** %d\n\n", withIssues), note)
	return nil
}

// healthCheckMaxEvents is how many warning and error lines the health-check
// prompt keeps. The result budget is the memory bound; this is the display cap.
// TODO: Config knob?
const healthCheckMaxEvents = 20

// gatherEventDiagnostics collects recent warning and error events.
// Lines share the caller's result budget. The next namespace List does not
// start once healthCheckMaxEvents lines are kept or that budget is exhausted.
// Totals keep counting events already fetched. See
// docs/specs/memory-bounds.md#retention.
func gatherEventDiagnostics(params api.PromptHandlerParams, namespace string) (string, error) {
	b := api.NewBuilderWithBudget(api.ResultLimit(params.Context))
	writeEventBody(params, namespace, false, nil, b)
	return b.String(), nil
}

// eventTotalsReserve is room for "**Warnings:** %d | **Errors:** %d\n\n" so
// the totals line stays in front of the event lines.
const eventTotalsReserve = 80

func writeEventBody(params api.PromptHandlerParams, namespace string, nsFetched bool, knownNS []v1.Namespace, b *api.BuilderWithBudget) {
	if b == nil || b.Truncated() {
		return
	}
	lines := b.Scratch(eventTotalsReserve)
	if lines.Truncated() {
		return
	}

	namespaces := []string{namespace}
	if namespace == "" {
		namespaces = []string{"default", "kube-system"}
		items := knownNS
		if !nsFetched {
			nsList, err := params.CoreV1().Namespaces().List(params.Context, metav1.ListOptions{})
			if err == nil {
				items = nsList.Items
			}
		}
		for _, ns := range items {
			if strings.HasPrefix(ns.Name, "openshift-") {
				namespaces = append(namespaces, ns.Name)
			}
		}
	}

	oneHourAgo := time.Now().Add(-1 * time.Hour)
	totalWarnings := 0
	totalErrors := 0
	retained := 0

	for _, ns := range namespaces {
		if retained >= healthCheckMaxEvents || lines.Truncated() {
			break
		}
		eventList, err := params.CoreV1().Events(ns).List(params.Context, metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, event := range eventList.Items {
			if event.Type != v1.EventTypeWarning && event.Type != "Error" {
				continue
			}

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
			if retained >= healthCheckMaxEvents || lines.Truncated() {
				continue
			}

			message := event.Message
			if len(message) > 150 {
				message = message[:150] + "..."
			}

			if lines.WriteString(fmt.Sprintf("- **%s/%s** in `%s` (%s, Count: %d)\n  - %s\n\n",
				event.InvolvedObject.Kind, event.InvolvedObject.Name, ns, event.Reason, event.Count, message)) {
				continue
			}
			retained++
		}
	}

	b.WriteString(fmt.Sprintf("**Warnings:** %d | **Errors:** %d\n\n", totalWarnings, totalErrors))
	if b.Truncated() {
		return
	}
	if lines.Len() == 0 {
		b.WriteString("*No recent warning/error events*")
		return
	}
	b.WriteString(lines.String())
}

func writeHealthCheckPreamble(b *api.BuilderWithBudget, diag *clusterDiagnostics) {
	b.WriteString("# Cluster Health Check Diagnostic Data\n\n")
	b.WriteString(fmt.Sprintf("**Collection Time:** %s\n", diag.CollectionTime.Format(time.RFC3339)))

	if diag.NamespaceWarning != "" {
		b.WriteString("\n")
		b.WriteString("⚠️  **WARNING:** " + diag.NamespaceWarning + "\n")
		b.WriteString("\n")
		b.WriteString("**Note:** Please verify the namespace name and try again if you want namespace-specific diagnostics.\n")
	}

	if diag.NamespaceScoped {
		b.WriteString(fmt.Sprintf("**Scope:** Namespace `%s`\n", diag.TargetNamespace))
	} else {
		b.WriteString(fmt.Sprintf("**Scope:** All namespaces (Total: %d)\n", diag.TotalNamespaces))
	}
	b.WriteString("\n")

	b.WriteString("## Your Task\n\n")
	b.WriteString("Analyze the following cluster diagnostic data and provide:\n")
	b.WriteString("1. **Overall Health Status**: Healthy, Warning, or Critical\n")
	b.WriteString("2. **Critical Issues**: Issues requiring immediate attention\n")
	b.WriteString("3. **Warnings**: Non-critical issues that should be addressed\n")
	b.WriteString("4. **Recommendations**: Suggested actions to improve cluster health\n")
	b.WriteString("5. **Summary**: Brief overview of findings by component\n\n")
	b.WriteString("---\n\n")
}
