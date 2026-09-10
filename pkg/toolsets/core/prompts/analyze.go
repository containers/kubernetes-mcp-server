package prompts

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
)

// podRestartFreqThreshold is the restart frequency (restarts per hour) above
// which a pod is flagged as having issues, even if it's currently running.
const podRestartFreqThreshold = 1.0

// podMinAge is the minimum pod age used when computing restart frequency,
// to avoid inflating the rate for very recently started pods.
const podMinAge = 10 * time.Minute

// nodeHealthReport holds the analysis of cluster node health.
// Nodes are classified into three mutually exclusive categories:
// readyNodes + len(nodesUnderPressure) + len(nodesWithIssues) == totalNodes.
type nodeHealthReport struct {
	totalNodes         int
	readyNodes         int         // Ready with no pressure conditions
	nodesUnderPressure []nodeIssue // Ready but with pressure conditions
	nodesWithIssues    []nodeIssue // NotReady or Unknown
}

// nodeIssue captures issues for a single node.
type nodeIssue struct {
	name   string
	status string
	issues []string
}

// podHealthReport holds the analysis of pod health across namespaces.
type podHealthReport struct {
	totalPods int
	issues    []podIssue
}

// podIssue captures pod issues aggregated for a single namespace.
type podIssue struct {
	namespace string
	issues    map[string][]string // issue type -> list of affected pod names
}

// workloadHealthReport holds the analysis of workload controller health.
type workloadHealthReport struct {
	// workloadType is the task name used for deterministic rendering (e.g. "deployments").
	workloadType string
	items        []workloadIssue
}

// workloadIssue captures unhealthy workloads for a single namespace.
type workloadIssue struct {
	namespace string
	unhealthy map[string]int32 // workload name -> number of missing replicas
}

// pvcHealthReport holds the analysis of PVC health.
type pvcHealthReport struct {
	items []pvcIssue
}

// pvcIssue captures PVC issues aggregated for a single namespace.
type pvcIssue struct {
	namespace string
	pending   []string // PVC names in Pending phase
	lost      []string // PVC names in Lost phase
}

// eventReport holds the analysis of recent warning/error events.
type eventReport struct {
	warnings int
	objects  []objectEvents
}

// objectEvents groups events for a single involved object within a namespace.
type objectEvents struct {
	key     string // "namespace/Kind/Name"
	entries []eventEntry
}

// eventEntry holds a single event's reason, count, and truncated message.
type eventEntry struct {
	reason  string
	count   int32
	message string
}

// clusterOperatorReport holds the analysis of OpenShift ClusterOperator health.
type clusterOperatorReport struct {
	operatorsWithIssues []operatorIssue
}

// operatorIssue captures issues for a single ClusterOperator.
type operatorIssue struct {
	name      string
	available string
	degraded  string
	issues    []string
}

// nsHealthData aggregates health issues for a single namespace.
type nsHealthData struct {
	podIssueCount int
	issues        map[string][]string      // issue category -> list of affected pod names
	workloads     map[string]workloadIssue // workload type (e.g. "deployments") -> aggregated issue
	pvcs          pvcIssue
}

// --- Analysis Task constructors (Layer 2 on tasks.Tasks) ---

// analyzeNodeHealthTask returns a Task that analyzes node health.
// It extracts the *v1.NodeList from the gather results internally.
func analyzeNodeHealthTask(gatherResults []tasks.TaskResult) *tasks.Task {
	return &tasks.Task{
		Name: taskNameNodes,
		Run: func(_ context.Context) (any, error) {
			nodeList, err := extractRequiredTaskOutput[*v1.NodeList](gatherResults, taskNameNodes)
			if err != nil {
				return nil, err
			}
			return analyzeNodeHealth(nodeList), nil
		},
	}
}

// analyzePodHealthTask returns a Task that analyzes pod health.
// It extracts the *v1.PodList from the gather results internally.
func analyzePodHealthTask(gatherResults []tasks.TaskResult) *tasks.Task {
	return &tasks.Task{
		Name: taskNamePods,
		Run: func(_ context.Context) (any, error) {
			podList, err := extractRequiredTaskOutput[*v1.PodList](gatherResults, taskNamePods)
			if err != nil {
				return nil, err
			}
			return analyzePodHealth(podList), nil
		},
	}
}

// analyzeDeploymentHealthTask returns a Task that analyzes deployment health.
// It extracts the *appsv1.DeploymentList from the gather results internally.
func analyzeDeploymentHealthTask(gatherResults []tasks.TaskResult) *tasks.Task {
	return &tasks.Task{
		Name: taskNameDeployments,
		Run: func(_ context.Context) (any, error) {
			deployList, err := extractRequiredTaskOutput[*appsv1.DeploymentList](gatherResults, taskNameDeployments)
			if err != nil {
				return nil, err
			}
			return analyzeDeploymentHealth(deployList), nil
		},
	}
}

// analyzeStatefulSetHealthTask returns a Task that analyzes statefulset health.
// It extracts the *appsv1.StatefulSetList from the gather results internally.
func analyzeStatefulSetHealthTask(gatherResults []tasks.TaskResult) *tasks.Task {
	return &tasks.Task{
		Name: taskNameStatefulSets,
		Run: func(_ context.Context) (any, error) {
			stsList, err := extractRequiredTaskOutput[*appsv1.StatefulSetList](gatherResults, taskNameStatefulSets)
			if err != nil {
				return nil, err
			}
			return analyzeStatefulSetHealth(stsList), nil
		},
	}
}

// analyzeDaemonSetHealthTask returns a Task that analyzes daemonset health.
// It extracts the *appsv1.DaemonSetList from the gather results internally.
func analyzeDaemonSetHealthTask(gatherResults []tasks.TaskResult) *tasks.Task {
	return &tasks.Task{
		Name: taskNameDaemonSets,
		Run: func(_ context.Context) (any, error) {
			dsList, err := extractRequiredTaskOutput[*appsv1.DaemonSetList](gatherResults, taskNameDaemonSets)
			if err != nil {
				return nil, err
			}
			return analyzeDaemonSetHealth(dsList), nil
		},
	}
}

// analyzePVCHealthTask returns a Task that analyzes PVC health.
// It extracts the *v1.PersistentVolumeClaimList from the gather results internally.
func analyzePVCHealthTask(gatherResults []tasks.TaskResult) *tasks.Task {
	return &tasks.Task{
		Name: taskNamePVCs,
		Run: func(_ context.Context) (any, error) {
			pvcList, err := extractRequiredTaskOutput[*v1.PersistentVolumeClaimList](gatherResults, taskNamePVCs)
			if err != nil {
				return nil, err
			}
			return analyzePVCHealth(pvcList), nil
		},
	}
}

// analyzeEventsTask returns a Task that analyzes events from a single namespace.
func analyzeEventsTask(gatherResult tasks.TaskResult, since time.Time) *tasks.Task {
	namespace := strings.TrimPrefix(gatherResult.Name, eventTaskPrefix)
	return &tasks.Task{
		Name: gatherResult.Name,
		Run: func(_ context.Context) (any, error) {
			if gatherResult.Err != nil {
				return nil, fmt.Errorf("failed to gather events for %s: %w", namespace, gatherResult.Err)
			}
			if gatherResult.Output == nil {
				return nil, fmt.Errorf("gather result for events in %s returned no output", namespace)
			}
			eventList, ok := gatherResult.Output.(*v1.EventList)
			if !ok {
				return nil, fmt.Errorf("unexpected gather output type for events in %s: %T", namespace, gatherResult.Output)
			}
			return analyzeEvents(namespace, eventList, since), nil
		},
	}
}

// analyzeClusterOperatorHealthTask returns a Task that analyzes ClusterOperator health.
// It extracts the unstructured output from the gather results internally.
func analyzeClusterOperatorHealthTask(gatherResults []tasks.TaskResult) *tasks.Task {
	return &tasks.Task{
		Name: taskNameClusterOperators,
		Run: func(_ context.Context) (any, error) {
			output, err := extractRequiredTaskOutputAny(gatherResults, taskNameClusterOperators)
			if err != nil {
				return nil, err
			}
			return analyzeClusterOperatorHealth(output), nil
		},
	}
}

// --- Pure analysis functions ---

// analyzeNodeHealth examines nodes and returns a health report.
// Each node is placed into exactly one of three buckets:
// ready (no issues), under pressure (Ready but with pressure conditions),
// or issues (NotReady/Unknown).
func analyzeNodeHealth(nodeList *v1.NodeList) *nodeHealthReport {
	if nodeList == nil {
		return &nodeHealthReport{}
	}
	report := &nodeHealthReport{totalNodes: len(nodeList.Items)}
	for _, node := range nodeList.Items {
		nodeStatus := "Unknown"
		isReady := false
		var pressures []string
		var issues []string

		for _, cond := range node.Status.Conditions {
			if cond.Type == v1.NodeReady {
				if cond.Status == v1.ConditionTrue {
					nodeStatus = "Ready"
					isReady = true
				} else {
					nodeStatus = "NotReady"
					issues = append(issues, fmt.Sprintf("Not ready: %s", cond.Message))
				}
			} else if cond.Status == v1.ConditionTrue {
				pressures = append(pressures, fmt.Sprintf("%s: %s", cond.Type, cond.Message))
			}
		}

		switch {
		case isReady && len(pressures) > 0:
			report.nodesUnderPressure = append(report.nodesUnderPressure, nodeIssue{
				name:   node.Name,
				status: nodeStatus,
				issues: pressures,
			})
		case !isReady:
			report.nodesWithIssues = append(report.nodesWithIssues, nodeIssue{
				name:   node.Name,
				status: nodeStatus,
				issues: append(issues, pressures...),
			})
		default:
			report.readyNodes++
		}
	}
	return report
}

// analyzePodHealth examines pods and returns a health report.
// Issues are aggregated per namespace: each issue type maps to the list of
// affected pod names so that namespace-scoped reports can show names while
// cluster-wide reports derive counts via len().
func analyzePodHealth(podList *v1.PodList) *podHealthReport {
	if podList == nil {
		return &podHealthReport{}
	}
	report := &podHealthReport{totalPods: len(podList.Items)}
	nsIssues := make(map[string]map[string][]string) // ns -> issue type -> pod names
	for i := range podList.Items {
		pod := &podList.Items[i]
		issues := collectPodIssues(pod)
		if len(issues) > 0 {
			if nsIssues[pod.Namespace] == nil {
				nsIssues[pod.Namespace] = make(map[string][]string)
			}
			for _, issue := range issues {
				nsIssues[pod.Namespace][issue] = append(nsIssues[pod.Namespace][issue], pod.Name)
			}
		}
	}
	for ns, issues := range nsIssues {
		report.issues = append(report.issues, podIssue{
			namespace: ns,
			issues:    issues,
		})
	}
	return report
}

// benignWaitingReasons contains container waiting reasons that indicate normal
// startup or initialization and should not be flagged as issues. Using a small
// allow-list makes the health check resilient to new failure reasons added by
// Kubernetes or CRI implementations over time.
var benignWaitingReasons = map[string]struct{}{
	"ContainerCreating": {},
	"PodInitializing":   {},
}

// collectContainerStateIssues inspects a single container status for
// problematic states and appends deduplicated issue strings to issues.
// The prefix distinguishes init containers from regular containers.
// Rather than enumerating every possible failure reason, it flags any waiting
// reason that is not known to be benign and any terminated state that did not
// exit cleanly (reason "Completed" with exit code 0).
func collectContainerStateIssues(cs v1.ContainerStatus, prefix string, issueSet map[string]struct{}, issues *[]string) {
	if cs.State.Waiting != nil {
		reason := cs.State.Waiting.Reason
		if _, benign := benignWaitingReasons[reason]; !benign {
			addContainerIssue(prefix+" waiting: "+reason, issueSet, issues)
		}
	}

	if cs.State.Terminated != nil {
		reason := cs.State.Terminated.Reason
		exitCode := cs.State.Terminated.ExitCode
		// A clean exit is reason "Completed" with exit code 0. Anything else
		// (non-zero exit, empty reason, or a failure reason) is an issue.
		if reason != "Completed" || exitCode != 0 {
			msg := prefix + " terminated: " + reason
			if reason == "" {
				msg = fmt.Sprintf("%s terminated: exit code %d", prefix, exitCode)
			}
			addContainerIssue(msg, issueSet, issues)
		}
	}
}

// addContainerIssue adds msg to issues if it has not already been recorded.
func addContainerIssue(msg string, issueSet map[string]struct{}, issues *[]string) {
	if _, dup := issueSet[msg]; !dup {
		issueSet[msg] = struct{}{}
		*issues = append(*issues, msg)
	}
}

// isInitStuck reports whether a Pending pod is scheduled to a node with init
// containers that have not all completed and no init-container-level issues were
// already captured. This distinguishes pods stuck in the init phase from pods
// that are unschedulable or have explicit init failures (e.g. Error, OOMKilled).
func isInitStuck(pod *v1.Pod, currentIssues []string) bool {
	if pod.Status.Phase != v1.PodPending {
		return false
	}
	if pod.Spec.NodeName == "" {
		return false
	}
	if len(pod.Spec.InitContainers) == 0 {
		return false
	}
	// If collectContainerStateIssues already found init container problems
	// (e.g. terminated with Error), defer to those more specific messages.
	for _, issue := range currentIssues {
		if strings.HasPrefix(issue, "Init container") {
			return false
		}
	}
	ready, total := initProgress(pod)
	return ready < total
}

// initProgress returns the number of completed init containers and the total.
func initProgress(pod *v1.Pod) (int, int) {
	total := len(pod.Spec.InitContainers)
	ready := 0
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.Ready || (cs.State.Terminated != nil && cs.State.Terminated.Reason == "Completed" && cs.State.Terminated.ExitCode == 0) {
			ready++
		}
	}
	return ready, total
}

// collectPodIssues examines a pod's container statuses and phase and returns
// a deduplicated list of issue descriptions. Returns nil if the pod is healthy.
func collectPodIssues(pod *v1.Pod) []string {
	var issues []string
	restarts := int32(0)

	// Check container statuses (deduplicate identical issues across containers)
	issueSet := make(map[string]struct{})
	for _, cs := range pod.Status.InitContainerStatuses {
		restarts += cs.RestartCount
		collectContainerStateIssues(cs, "Init container", issueSet, &issues)
	}
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
		collectContainerStateIssues(cs, "Container", issueSet, &issues)
	}

	// Check pod phase
	if pod.Status.Phase != v1.PodRunning && pod.Status.Phase != v1.PodSucceeded {
		if isInitStuck(pod, issues) {
			ready, total := initProgress(pod)
			issues = append(issues, fmt.Sprintf("Init containers not ready: %d/%d", ready, total))
		} else {
			issues = append(issues, fmt.Sprintf("Pod in %s phase", pod.Status.Phase))
		}
	}

	// Flag pods with high restart frequency even if no container issues
	if restarts > 0 && len(issues) == 0 {
		startTime := pod.CreationTimestamp.Time
		if pod.Status.StartTime != nil {
			startTime = pod.Status.StartTime.Time
		}
		age := time.Since(startTime)
		if age < podMinAge {
			age = podMinAge
		}
		if float64(restarts)/age.Hours() > podRestartFreqThreshold {
			issues = append(issues, "High restart frequency")
		}
	}

	return issues
}

// analyzeDeploymentHealth examines deployments and returns a health report.
func analyzeDeploymentHealth(deployList *appsv1.DeploymentList) *workloadHealthReport {
	if deployList == nil {
		return &workloadHealthReport{workloadType: taskNameDeployments}
	}
	report := &workloadHealthReport{workloadType: taskNameDeployments}
	for _, d := range deployList.Items {
		if d.Status.UnavailableReplicas > 0 {
			report.items = append(report.items, workloadIssue{
				namespace: d.Namespace,
				unhealthy: map[string]int32{d.Name: d.Status.UnavailableReplicas},
			})
		}
	}
	return report
}

// analyzeStatefulSetHealth examines statefulsets and returns a health report.
func analyzeStatefulSetHealth(stsList *appsv1.StatefulSetList) *workloadHealthReport {
	if stsList == nil {
		return &workloadHealthReport{workloadType: taskNameStatefulSets}
	}
	report := &workloadHealthReport{workloadType: taskNameStatefulSets}
	for _, sts := range stsList.Items {
		specReplicas := int32(1)
		if sts.Spec.Replicas != nil {
			specReplicas = *sts.Spec.Replicas
		}
		if sts.Status.ReadyReplicas < specReplicas {
			report.items = append(report.items, workloadIssue{
				namespace: sts.Namespace,
				unhealthy: map[string]int32{sts.Name: specReplicas - sts.Status.ReadyReplicas},
			})
		}
	}
	return report
}

// analyzeDaemonSetHealth examines daemonsets and returns a health report.
func analyzeDaemonSetHealth(dsList *appsv1.DaemonSetList) *workloadHealthReport {
	if dsList == nil {
		return &workloadHealthReport{workloadType: taskNameDaemonSets}
	}
	report := &workloadHealthReport{workloadType: taskNameDaemonSets}
	for _, ds := range dsList.Items {
		if ds.Status.NumberUnavailable > 0 {
			report.items = append(report.items, workloadIssue{
				namespace: ds.Namespace,
				unhealthy: map[string]int32{ds.Name: ds.Status.NumberUnavailable},
			})
		}
	}
	return report
}

// analyzePVCHealth examines PVCs and returns a health report.
// Issues are aggregated per namespace with Pending and Lost phases tracked
// separately so the renderer can surface Lost PVCs as a more severe condition.
func analyzePVCHealth(pvcList *v1.PersistentVolumeClaimList) *pvcHealthReport {
	if pvcList == nil {
		return &pvcHealthReport{}
	}
	report := &pvcHealthReport{}
	nsMap := make(map[string]*pvcIssue)
	for _, pvc := range pvcList.Items {
		switch pvc.Status.Phase {
		case v1.ClaimPending:
			pi := getOrCreatePVCIssue(nsMap, pvc.Namespace)
			pi.pending = append(pi.pending, pvc.Name)
		case v1.ClaimLost:
			pi := getOrCreatePVCIssue(nsMap, pvc.Namespace)
			pi.lost = append(pi.lost, pvc.Name)
		}
	}
	for _, pi := range nsMap {
		report.items = append(report.items, *pi)
	}
	return report
}

func getOrCreatePVCIssue(m map[string]*pvcIssue, ns string) *pvcIssue {
	if pi, ok := m[ns]; ok {
		return pi
	}
	pi := &pvcIssue{namespace: ns}
	m[ns] = pi
	return pi
}

// analyzeEvents examines events from a single namespace and returns an event report.
func analyzeEvents(namespace string, eventList *v1.EventList, since time.Time) *eventReport {
	if eventList == nil {
		return &eventReport{}
	}
	report := &eventReport{}
	objectIndex := make(map[string]int) // key -> index in report.objects

	for _, event := range eventList.Items {
		// Check timestamp - prefer Series.LastObservedTime for server-side
		// aggregation events, fall back to top-level fields otherwise.
		var lastSeenTime time.Time
		if event.Series != nil {
			lastSeenTime = event.Series.LastObservedTime.Time
		}
		if lastSeenTime.IsZero() {
			lastSeenTime = event.LastTimestamp.Time
		}
		if lastSeenTime.IsZero() {
			lastSeenTime = event.EventTime.Time
		}
		if lastSeenTime.Before(since) {
			continue
		}

		report.warnings++

		// Collapse multi-line messages and limit length
		message := strings.ReplaceAll(event.Message, "\n", "; ")
		// Truncate long messages (use runes so multi-byte UTF-8 characters are
		// not split in the middle).
		if utf8.RuneCountInString(message) > 150 {
			message = string([]rune(message)[:150]) + "..."
		}

		key := namespace + "/" + event.InvolvedObject.Kind + "/" + event.InvolvedObject.Name
		idx, exists := objectIndex[key]
		if !exists {
			idx = len(report.objects)
			objectIndex[key] = idx
			report.objects = append(report.objects, objectEvents{key: key})
		}

		// Prefer Series.Count for server-side aggregated events.
		count := event.Count
		if event.Series != nil && event.Series.Count > count {
			count = event.Series.Count
		}

		report.objects[idx].entries = append(report.objects[idx].entries, eventEntry{
			reason:  event.Reason,
			count:   count,
			message: message,
		})
	}
	return report
}

// analyzeClusterOperatorHealth examines ClusterOperator unstructured output
// and returns a health report.
func analyzeClusterOperatorHealth(output any) *clusterOperatorReport {
	report := &clusterOperatorReport{}
	if output == nil {
		return report
	}

	type unstructuredContenter interface {
		UnstructuredContent() map[string]any
	}
	uc, ok := output.(unstructuredContenter)
	if !ok {
		return report
	}

	items, ok := uc.UnstructuredContent()["items"].([]any)
	if !ok {
		return report
	}

	for _, item := range items {
		opMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		metadata, _ := opMap["metadata"].(map[string]any)
		name, _ := metadata["name"].(string)

		status, _ := opMap["status"].(map[string]any)
		conditions, _ := status["conditions"].([]any)

		available := "Unknown"
		degraded := "Unknown"
		var issues []string

		for _, cond := range conditions {
			condMap, _ := cond.(map[string]any)
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
			report.operatorsWithIssues = append(report.operatorsWithIssues, operatorIssue{
				name:      name,
				available: available,
				degraded:  degraded,
				issues:    issues,
			})
		}
	}
	return report
}
