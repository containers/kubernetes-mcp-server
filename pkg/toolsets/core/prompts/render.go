package prompts

import (
	"fmt"
	"sort"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
)

// reportLineLimit is the maximum number of data lines (namespace entries +
// event entries) rendered across the namespace-health and event sections of a
// health-check prompt. The budget is shared: events are allocated first (they
// carry higher diagnostic value), namespace health gets the remainder.
const reportLineLimit = 200

// maxNamespaceReportLines is the maximum number of namespace-health lines we
// reserve from the shared budget when events are competing for space. It is not
// a hard cap on namespace output: if there are fewer namespace entries than
// this value, the unused headroom is given to events, and if there are no
// events at all, namespaces may use the full reportLineLimit budget.
const maxNamespaceReportLines = 20

// countEventLines computes how many output lines the event section would
// consume without any truncation. Each objectEvents group produces
// 1 (header) + len(entries) lines.
func countEventLines(results []tasks.TaskResult) int {
	lines := 0
	for _, t := range results {
		if t.Err != nil || t.Output == nil {
			continue
		}

		if r, ok := t.Output.(*eventReport); ok {
			for _, obj := range r.objects {
				lines += 1 + len(obj.entries)
			}
		}
	}
	return lines
}

// isNamespaceHealthTaskResult reports whether a TaskResult belongs in the
// namespace health section, including error-only results that may not have a
// typed output due to analysis failure.
func isNamespaceHealthTaskResult(result tasks.TaskResult) bool {
	switch result.Name {
	case taskNamePods, taskNameDeployments, taskNameStatefulSets, taskNameDaemonSets, taskNamePVCs:
		return true
	}

	_, ok := result.Output.(*podHealthReport)
	if ok {
		return true
	}

	_, ok = result.Output.(*workloadHealthReport)
	if ok {
		return true
	}

	_, ok = result.Output.(*pvcHealthReport)
	return ok
}

// isEventTaskResult reports whether a TaskResult belongs in the event section,
// including wrapper and error-only results so event analysis failures remain
// visible during rendering.
func isEventTaskResult(result tasks.TaskResult) bool {
	if result.Name == taskNameEventsGather {
		return true
	}
	if strings.HasPrefix(result.Name, eventTaskPrefix) {
		return true
	}
	_, ok := result.Output.(*eventReport)
	if ok {
		return true
	}

	if result.Err == nil {
		return false
	}

	switch result.Name {
	case taskNameNodes, taskNamePods, taskNameDeployments, taskNameStatefulSets, taskNameDaemonSets, taskNamePVCs, taskNameClusterOperators:
		return false
	default:
		return true
	}
}

// countNamespaceHealthLines computes how many namespace summary lines would be
// rendered for the namespace health section without truncation.
func countNamespaceHealthLines(results []tasks.TaskResult) int {
	nsData := make(map[string]struct{})
	for _, t := range results {
		if t.Err != nil || t.Output == nil {
			continue
		}
		switch r := t.Output.(type) {
		case *podHealthReport:
			for _, pi := range r.issues {
				nsData[pi.namespace] = struct{}{}
			}
		case *workloadHealthReport:
			for _, wi := range r.items {
				nsData[wi.namespace] = struct{}{}
			}
		case *pvcHealthReport:
			for _, pvc := range r.items {
				nsData[pvc.namespace] = struct{}{}
			}
		}
	}
	return len(nsData)
}

// allocateReportBudgets splits the reportLineLimit between namespace and event health sections.
// Events are allocated first because they carry higher diagnostic value. Up to
// maxNamespaceReportLines are reserved for namespace health, capped by the
// actual number of namespace entries, so unused namespace headroom is given to
// events and namespaces may use the full budget when there are no events.
func allocateReportBudgets(nsResults []tasks.TaskResult, eventResults []tasks.TaskResult) (int, int) {
	nsCount := countNamespaceHealthLines(nsResults)
	if nsCount == 0 {
		return 0, min(countEventLines(eventResults), reportLineLimit)
	}

	maxEventBudget := reportLineLimit - min(nsCount, maxNamespaceReportLines)
	if maxEventBudget < 0 {
		maxEventBudget = 0
	}
	eventBudget := min(countEventLines(eventResults), maxEventBudget)
	nsBudget := reportLineLimit - eventBudget
	return nsBudget, eventBudget
}

// renderNodeHealth renders a nodeHealthReport as markdown.
func renderNodeHealth(report *nodeHealthReport) string {
	if report == nil || report.totalNodes == 0 {
		return "No nodes found"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Total:** %d | **Ready:** %d | **Under Pressure:** %d | **Issues:** %d\n",
		report.totalNodes, report.readyNodes, len(report.nodesUnderPressure), len(report.nodesWithIssues))

	if len(report.nodesUnderPressure) == 0 && len(report.nodesWithIssues) == 0 {
		sb.WriteString("*All nodes are healthy*")
		return sb.String()
	}

	var lines []string
	for _, ni := range report.nodesUnderPressure {
		lines = append(lines, fmt.Sprintf("- **%s** (Status: %s, under pressure)\n%s",
			ni.name, ni.status, " - "+strings.Join(ni.issues, "\n - ")))
	}

	for _, ni := range report.nodesWithIssues {
		lines = append(lines, fmt.Sprintf("- **%s** (Status: %s)\n%s",
			ni.name, ni.status, " - "+strings.Join(ni.issues, "\n - ")))
	}

	sb.WriteString(strings.Join(lines, "\n\n"))
	return sb.String()
}

// renderNamespaceHealthSummary renders the merged namespace health view from
// analysis task results. It expects the results to contain *podHealthReport,
// *workloadHealthReport, and *pvcHealthReport outputs.
// When detailed is true (namespace-scoped), pod names and workload names are
// included in the output. When false (cluster-wide), compact count-based
// summaries are used instead.
// maxLines limits the number of namespace entries rendered (each namespace
// produces one line). A value <= 0 means unlimited.
func renderNamespaceHealthSummary(results []tasks.TaskResult, detailed bool, maxLines int) string {
	nsData := make(map[string]*nsHealthData)
	var nsOrder []string
	getOrCreate := func(ns string) *nsHealthData {
		d, ok := nsData[ns]
		if !ok {
			d = &nsHealthData{issues: make(map[string][]string), workloads: make(map[string]workloadIssue)}
			nsData[ns] = d
			nsOrder = append(nsOrder, ns)
		}
		return d
	}

	var totalPods, totalPodIssues, totalUnhealthyWorkloads, totalPVCPending, totalPVCLost int
	var taskErrors []string

	for _, t := range results {
		if t.Err != nil {
			taskErrors = append(taskErrors, fmt.Sprintf("⚠️ Failed to analyze %s: %v", t.Name, t.Err))
			continue
		}
		if t.Output == nil {
			continue
		}

		switch r := t.Output.(type) {
		case *podHealthReport:
			totalPods = r.totalPods
			for _, pi := range r.issues {
				d := getOrCreate(pi.namespace)
				uniquePods := make(map[string]struct{})
				for issue, podNames := range pi.issues {
					d.issues[issue] = append(d.issues[issue], podNames...)
					for _, name := range podNames {
						uniquePods[name] = struct{}{}
					}
				}
				d.podIssueCount += len(uniquePods)
				totalPodIssues += len(uniquePods)
			}
		case *workloadHealthReport:
			for _, wi := range r.items {
				totalUnhealthyWorkloads += len(wi.unhealthy)
				d := getOrCreate(wi.namespace)
				w := d.workloads[r.workloadType]
				if w.unhealthy == nil {
					w.unhealthy = make(map[string]int32)
				}
				for name, missing := range wi.unhealthy {
					w.unhealthy[name] = missing
				}
				d.workloads[r.workloadType] = w
			}
		case *pvcHealthReport:
			for _, pi := range r.items {
				totalPVCPending += len(pi.pending)
				totalPVCLost += len(pi.lost)
				d := getOrCreate(pi.namespace)
				d.pvcs.pending = append(d.pvcs.pending, pi.pending...)
				d.pvcs.lost = append(d.pvcs.lost, pi.lost...)
			}
		}
	}

	// --- Render ---
	var sb strings.Builder
	if len(taskErrors) > 0 {
		sb.WriteString(strings.Join(taskErrors, "\n"))
		sb.WriteString("\n\n")
	}

	fmt.Fprintf(&sb, "**Total Pods:** %d | **Pods with Issues:** %d | **Unhealthy Workloads:** %d | **PVCs Pending:** %d | **PVCs Lost:** %d\n",
		totalPods, totalPodIssues, totalUnhealthyWorkloads, totalPVCPending, totalPVCLost)

	if len(nsData) == 0 {
		if len(taskErrors) > 0 {
			sb.WriteString("*No namespace health summary available because one or more analysis tasks failed*")
		} else {
			sb.WriteString("*No issues detected*")
		}
		return sb.String()
	}

	// Sort namespaces by total issue count (descending) for triage priority
	nsIssueCount := func(d *nsHealthData) int {
		c := d.podIssueCount + len(d.pvcs.pending) + len(d.pvcs.lost)
		for _, w := range d.workloads {
			c += len(w.unhealthy)
		}
		return c
	}

	sort.Slice(nsOrder, func(i, j int) bool {
		ci, cj := nsIssueCount(nsData[nsOrder[i]]), nsIssueCount(nsData[nsOrder[j]])
		if ci != cj {
			return ci > cj
		}
		return nsOrder[i] < nsOrder[j]
	})

	truncated := maxLines > 0 && len(nsOrder) > maxLines
	if truncated {
		fmt.Fprintf(&sb, "_Showing top %d namespaces by issue count (%d namespaces with issues total)_\n\n", maxLines, len(nsOrder))
		nsOrder = nsOrder[:maxLines]
	}

	var lines []string
	for _, ns := range nsOrder {
		d := nsData[ns]
		var parts []string

		if d.podIssueCount > 0 {
			var issueParts []string
			for issue, podNames := range d.issues {
				if detailed {
					sort.Strings(podNames)
					issueParts = append(issueParts, fmt.Sprintf("%s: %s", issue, strings.Join(podNames, ", ")))
				} else {
					issueParts = append(issueParts, fmt.Sprintf("%dx %s", len(podNames), issue))
				}
			}
			sort.Strings(issueParts)
			parts = append(parts, fmt.Sprintf("Pods: %d (%s)", d.podIssueCount, strings.Join(issueParts, ", ")))
		}

		// Render workload issues in deterministic order
		for _, wt := range []string{taskNameDeployments, taskNameStatefulSets, taskNameDaemonSets} {
			w, ok := d.workloads[wt]
			if !ok || len(w.unhealthy) == 0 {
				continue
			}

			if detailed {
				var wParts []string
				names := make([]string, 0, len(w.unhealthy))
				for name := range w.unhealthy {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					wParts = append(wParts, fmt.Sprintf("%s (%d replicas missing)", name, w.unhealthy[name]))
				}
				parts = append(parts, fmt.Sprintf("%s: %s", wt, strings.Join(wParts, ", ")))
			} else {
				totalMissing := int32(0)
				for _, m := range w.unhealthy {
					totalMissing += m
				}
				parts = append(parts, fmt.Sprintf("%s: %d unhealthy (%d replicas missing)", wt, len(w.unhealthy), totalMissing))
			}
		}

		pvcTotal := len(d.pvcs.pending) + len(d.pvcs.lost)
		if pvcTotal > 0 {
			var pvcParts []string
			if len(d.pvcs.pending) > 0 {
				if detailed {
					sort.Strings(d.pvcs.pending)
					pvcParts = append(pvcParts, fmt.Sprintf("pending: %s", strings.Join(d.pvcs.pending, ", ")))
				} else {
					pvcParts = append(pvcParts, fmt.Sprintf("%d pending", len(d.pvcs.pending)))
				}
			}
			if len(d.pvcs.lost) > 0 {
				if detailed {
					sort.Strings(d.pvcs.lost)
					pvcParts = append(pvcParts, fmt.Sprintf("lost: %s", strings.Join(d.pvcs.lost, ", ")))
				} else {
					pvcParts = append(pvcParts, fmt.Sprintf("%d lost", len(d.pvcs.lost)))
				}
			}
			parts = append(parts, fmt.Sprintf("PVC: %s", strings.Join(pvcParts, ", ")))
		}

		lines = append(lines, fmt.Sprintf("- **%s** - %s", ns, strings.Join(parts, " | ")))
	}

	sb.WriteString(strings.Join(lines, "\n"))
	return sb.String()
}

// renderEvents renders event reports as markdown.
// maxLines limits the number of data lines rendered (each object group
// consumes 1 + len(entries) lines). A value <= 0 means unlimited.
func renderEvents(results []tasks.TaskResult, maxLines int) string {
	totalWarnings := 0
	var allObjects []objectEvents
	var taskErrors []string

	for _, t := range results {
		if t.Err != nil {
			taskErrors = append(taskErrors, fmt.Sprintf("⚠️ Failed to analyze events for %s: %v", strings.TrimPrefix(t.Name, eventTaskPrefix), t.Err))
			continue
		}
		if t.Output == nil {
			continue
		}

		if r, ok := t.Output.(*eventReport); ok {
			totalWarnings += r.warnings
			allObjects = append(allObjects, r.objects...)
		}
	}

	// Truncate object groups to fit within the line budget
	linesUsed := 0
	truncIdx := len(allObjects)
	if maxLines > 0 {
		for i, obj := range allObjects {
			need := 1 + len(obj.entries)
			if linesUsed+need > maxLines {
				truncIdx = i
				break
			}
			linesUsed += need
		}
	}

	eventsTruncated := truncIdx < len(allObjects)
	if eventsTruncated {
		allObjects = allObjects[:truncIdx]
	}

	var sb strings.Builder
	if len(taskErrors) > 0 {
		sb.WriteString(strings.Join(taskErrors, "\n"))
		sb.WriteString("\n\n")
	}

	fmt.Fprintf(&sb, "**Warnings:** %d\n", totalWarnings)
	if eventsTruncated {
		fmt.Fprintf(&sb, "_Event list truncated (%d involved object(s) shown)_\n\n", len(allObjects))
	}

	if len(allObjects) > 0 {
		var lines []string
		for _, obj := range allObjects {
			var entryLines []string
			for _, e := range obj.entries {
				entryLines = append(entryLines, fmt.Sprintf("  - %s (Count: %d): %s", e.reason, e.count, e.message))
			}
			lines = append(lines, fmt.Sprintf("- **%s**\n%s", obj.key, strings.Join(entryLines, "\n")))
		}
		sb.WriteString(strings.Join(lines, "\n"))
	} else {
		switch {
		case len(taskErrors) > 0:
			sb.WriteString("*No event details available because one or more event analysis tasks failed*")
		case totalWarnings > 0:
			sb.WriteString("*Warning events are present but do not fit in the available event budget. Consider checking the cluster event log directly.*")
		default:
			sb.WriteString("*No recent warning events*")
		}
	}

	return sb.String()
}

// formatPromptHeader formats the common header for health check prompts.
func formatPromptHeader(title string, collectionTime time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", title)
	fmt.Fprintf(&sb, "**Collection Time:** %s\n", collectionTime.Format(time.RFC3339))
	fmt.Fprintf(&sb, "**Elapsed Time:** %s\n", time.Since(collectionTime))
	return sb.String()
}

// formatPromptInstructions formats the "Your Task" section.
func formatPromptInstructions(scope string) string {
	var sb strings.Builder
	sb.WriteString("## Your Task\n\n")
	fmt.Fprintf(&sb, "Analyze the following %s diagnostic data and provide:\n", scope)
	sb.WriteString("1. **Overall Health Status:** Healthy, Warning, or Critical\n")
	sb.WriteString("2. **Critical Issues:** Issues requiring immediate attention\n")
	sb.WriteString("3. **Warnings:** Non-critical issues that should be addressed\n")
	sb.WriteString("4. **Recommendations:** Suggested actions to improve health\n")
	sb.WriteString("5. **Summary:** Brief overview of findings by component\n\n")
	sb.WriteString("---\n\n")
	return sb.String()
}

// formatPromptFooter formats the closing request for the LLM.
func formatPromptFooter(assessment string) string {
	return fmt.Sprintf("---\n\n**Please analyze the above diagnostic data and provide your %s health assessment.**\n", assessment)
}

// renderNodeSection renders the Nodes section from analysis results.
func renderNodeSection(results []tasks.TaskResult) string {
	var sb strings.Builder
	sb.WriteString("## Nodes\n\n")
	if nodeResult, ok := extractTaskResult(results, taskNameNodes); ok && nodeResult.Err != nil {
		fmt.Fprintf(&sb, "⚠️ Failed to analyze nodes: %v", nodeResult.Err)
	} else {
		nodeReport, _ := extractTaskOutput[*nodeHealthReport](results, taskNameNodes)
		sb.WriteString(renderNodeHealth(nodeReport))
	}
	sb.WriteString("\n\n")
	return sb.String()
}

// renderClusterOperatorSection renders the OpenShift Cluster Operators section.
func renderClusterOperatorSection(results []tasks.TaskResult) string {
	var sb strings.Builder
	sb.WriteString("## Cluster Operators (OpenShift)\n\n")
	if coResult, ok := extractTaskResult(results, taskNameClusterOperators); ok && coResult.Err != nil {
		fmt.Fprintf(&sb, "⚠️ Failed to analyze cluster operators: %v", coResult.Err)
	} else {
		coReport, _ := extractTaskOutput[*clusterOperatorReport](results, taskNameClusterOperators)
		sb.WriteString(renderClusterOperators(coReport))
	}
	sb.WriteString("\n\n")
	return sb.String()
}

// partitionHealthResults splits analyze results into namespace-health and event
// result slices.
func partitionHealthResults(results []tasks.TaskResult) (nsHealth []tasks.TaskResult, events []tasks.TaskResult) {
	for _, r := range results {
		if isNamespaceHealthTaskResult(r) {
			nsHealth = append(nsHealth, r)
		}
		if isEventTaskResult(r) {
			events = append(events, r)
		}
	}
	return nsHealth, events
}

// renderHealthSummarySection renders a namespace/workload health summary section.
func renderHealthSummarySection(title string, results []tasks.TaskResult, detailed bool, budget int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", title)
	sb.WriteString(renderNamespaceHealthSummary(results, detailed, budget))
	sb.WriteString("\n\n")
	return sb.String()
}

// renderEventSection renders the events section when checkEvents is enabled.
func renderEventSection(title string, results []tasks.TaskResult, budget int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", title)
	sb.WriteString(renderEvents(results, budget))
	sb.WriteString("\n\n")
	return sb.String()
}

// formatNamespaceLookupError renders the warning block for a failed namespace lookup.
func formatNamespaceLookupError(namespace string, nsLookupErr error) string {
	var sb strings.Builder
	sb.WriteString("\n")
	switch {
	case apierrors.IsNotFound(nsLookupErr):
		fmt.Fprintf(&sb, "⚠️ **WARNING:** Namespace '%s' not found.\n", namespace)
		sb.WriteString("\nNote: Please verify the namespace name and try again.\n")
	case apierrors.IsForbidden(nsLookupErr):
		fmt.Fprintf(&sb, "⚠️ **WARNING:** Access denied for namespace '%s': %v\n", namespace, nsLookupErr)
		sb.WriteString("\nNote: Check RBAC permissions for this namespace.\n")
	case apierrors.IsUnauthorized(nsLookupErr):
		fmt.Fprintf(&sb, "⚠️ **WARNING:** Authentication failed while looking up namespace '%s': %v\n", namespace, nsLookupErr)
		sb.WriteString("\nNote: Check cluster credentials and try again.\n")
	default:
		fmt.Fprintf(&sb, "⚠️ **WARNING:** Failed to verify namespace '%s': %v\n", namespace, nsLookupErr)
		sb.WriteString("\nNote: The cluster may be unreachable or experiencing issues.\n")
	}
	return sb.String()
}

// formatNamespaceLookupNoData renders the "no diagnostic data" message after a
// failed namespace lookup.
func formatNamespaceLookupNoData(nsLookupErr error) string {
	if apierrors.IsNotFound(nsLookupErr) {
		return "**No diagnostic data available - namespace not found.**\n"
	}
	return "**No diagnostic data available - namespace lookup failed.**\n"
}

// formatClusterHealthPrompt formats the full cluster health check prompt output
// from analysis results.
func formatClusterHealthPrompt(
	collectionTime time.Time,
	clusterContext string,
	totalNamespaces int,
	analyzeResults []tasks.TaskResult,
	isOpenShift bool,
	checkEvents bool,
	nsListErr error,
) string {
	cluster := "unknown"
	if clusterContext != "" {
		cluster = clusterContext
	}

	var sb strings.Builder
	sb.WriteString(formatPromptHeader(fmt.Sprintf("Cluster Health Check Diagnostic Data for %s", cluster), collectionTime))
	fmt.Fprintf(&sb, "**Scope:** All namespaces for workload data; system namespaces for events (Total: %d)\n\n", totalNamespaces)
	if nsListErr != nil {
		fmt.Fprintf(&sb, "⚠️ **WARNING:** Failed to list namespaces: %v\n", nsListErr)
		sb.WriteString("Event gathering was skipped because the namespace list is unavailable.\n")
	}
	sb.WriteString(formatPromptInstructions("cluster"))

	sb.WriteString(renderNodeSection(analyzeResults))
	if isOpenShift {
		sb.WriteString(renderClusterOperatorSection(analyzeResults))
	}

	nsHealthResults, eventResults := partitionHealthResults(analyzeResults)
	nsBudget, eventBudget := allocateReportBudgets(nsHealthResults, eventResults)

	sb.WriteString(renderHealthSummarySection("Namespace Health", nsHealthResults, false, nsBudget))
	if checkEvents {
		sb.WriteString(renderEventSection("Recent System Events (Last Hour)", eventResults, eventBudget))
	}

	sb.WriteString(formatPromptFooter("comprehensive"))
	return sb.String()
}

// formatNamespaceHealthPrompt formats the full namespace health check prompt output.
func formatNamespaceHealthPrompt(
	collectionTime time.Time,
	clusterContext string,
	namespace string,
	analyzeResults []tasks.TaskResult,
	checkEvents bool,
	nsLookupErr error,
) string {
	cluster := "unknown"
	if clusterContext != "" {
		cluster = clusterContext
	}

	var sb strings.Builder
	sb.WriteString(formatPromptHeader(fmt.Sprintf("Namespace Health Check Diagnostic Data. Cluster: %s; Namespace: %s", cluster, namespace), collectionTime))
	if nsLookupErr != nil {
		sb.WriteString(formatNamespaceLookupError(namespace, nsLookupErr))
	}
	sb.WriteString("\n")
	sb.WriteString(formatPromptInstructions("namespace"))

	if apierrors.IsNotFound(nsLookupErr) {
		sb.WriteString(formatNamespaceLookupNoData(nsLookupErr))
		return sb.String()
	}

	nsHealthResults, eventResults := partitionHealthResults(analyzeResults)
	nsBudget, eventBudget := allocateReportBudgets(nsHealthResults, eventResults)

	sb.WriteString(renderHealthSummarySection("Workload Health", nsHealthResults, true, nsBudget))
	if checkEvents {
		sb.WriteString(renderEventSection("Recent Events (Last Hour)", eventResults, eventBudget))
	}

	sb.WriteString(formatPromptFooter("namespace"))
	return sb.String()
}

// renderClusterOperators renders a clusterOperatorReport as markdown.
func renderClusterOperators(report *clusterOperatorReport) string {
	if report == nil {
		return "No cluster operators found"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Operators with Issues:** %d\n", len(report.operatorsWithIssues))
	if len(report.operatorsWithIssues) > 0 {
		var lines []string
		for _, op := range report.operatorsWithIssues {
			lines = append(lines, fmt.Sprintf("- **%s** (Available: %s, Degraded: %s)\n - %s",
				op.name, op.available, op.degraded, strings.Join(op.issues, "\n - ")))
		}
		sb.WriteString(strings.Join(lines, "\n\n"))
	} else {
		sb.WriteString("*All cluster operators are healthy*")
	}

	return sb.String()
}
