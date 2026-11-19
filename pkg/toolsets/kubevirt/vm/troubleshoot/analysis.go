package troubleshoot

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type severity int

const (
	severityCritical severity = iota
	severityWarning
	severityInfo
)

func (s severity) String() string {
	switch s {
	case severityCritical:
		return "CRITICAL"
	case severityWarning:
		return "WARNING"
	default:
		return "INFO"
	}
}

type issue struct {
	Severity severity
	Message  string
	Fix      string
}

func analyzeIssues(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string, vm, vmi *unstructured.Unstructured, podDiagnostics []map[string]interface{}) string {
	var issues []issue

	issues = append(issues, checkVMConditions(vm)...)

	scIssues := checkStorageClass(ctx, dynamicClient, vm)
	issues = append(issues, scIssues...)
	issues = append(issues, checkDataVolumeErrors(vm, scIssues)...)

	issues = append(issues, checkCloudInitCommands(vm, vmi)...)
	issues = append(issues, checkNodeSelector(vm, vmi)...)
	issues = append(issues, checkPodHealth(podDiagnostics)...)
	issues = append(issues, checkMigrationStatus(ctx, dynamicClient, namespace, name)...)

	if len(issues) == 0 {
		return "## Detected Issues\n\n*No issues automatically detected. Review the raw diagnostic data below for manual analysis.*"
	}

	var result strings.Builder
	result.WriteString("## Detected Issues\n\n")
	for _, iss := range issues {
		fmt.Fprintf(&result, "- **%s**: %s\n", iss.Severity, iss.Message)
	}

	var fixes []string
	for _, iss := range issues {
		if iss.Fix != "" {
			fixes = append(fixes, iss.Fix)
		}
	}
	if len(fixes) > 0 {
		result.WriteString("\n## Suggested Fixes\n\n")
		for i, fix := range fixes {
			fmt.Fprintf(&result, "%d. %s\n", i+1, fix)
		}
	}

	return result.String()
}

func checkVMConditions(vm *unstructured.Unstructured) []issue {
	if vm == nil {
		return nil
	}

	var issues []issue

	conditions, _, _ := unstructured.NestedSlice(vm.Object, "status", "conditions")
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := condMap["type"].(string)
		status, _ := condMap["status"].(string)
		reason, _ := condMap["reason"].(string)
		message, _ := condMap["message"].(string)

		if condType != "Ready" || status != "False" {
			continue
		}

		switch reason {
		case "VMINotExists":
			issues = append(issues, issue{
				Severity: severityWarning,
				Message:  "VM condition Ready=False with reason VMINotExists. The VirtualMachineInstance has not been created.",
			})
		default:
			if message != "" && (strings.Contains(message, "Guest") || strings.Contains(strings.ToLower(message), "error")) {
				issues = append(issues, issue{
					Severity: severityWarning,
					Message:  fmt.Sprintf("VM condition Ready=False: %s", message),
				})
			}
		}
	}

	printableStatus, _, _ := unstructured.NestedString(vm.Object, "status", "printableStatus")
	switch printableStatus {
	case "Provisioning":
		issues = append(issues, issue{
			Severity: severityWarning,
			Message:  "VM is stuck in Provisioning state. This typically means a DataVolume or PVC cannot be created.",
		})
	case "CrashLoopBackOff":
		issues = append(issues, issue{
			Severity: severityCritical,
			Message:  "VM is in CrashLoopBackOff state. The guest OS or cloud-init is causing repeated failures.",
			Fix:      "Check the cloud-init userData for commands that shut down or crash the guest (e.g., shutdown -h now, exit 1).",
		})
	case "ErrImagePull", "ImagePullBackOff":
		issues = append(issues, issue{
			Severity: severityCritical,
			Message:  fmt.Sprintf("VM is in %s state. The container disk image cannot be pulled.", printableStatus),
			Fix:      "Verify the containerDisk image reference exists and is accessible from the cluster.",
		})
	}

	return issues
}

func checkDataVolumeErrors(vm *unstructured.Unstructured, scIssues []issue) []issue {
	if vm == nil {
		return nil
	}

	hasMissingSC := false
	for _, sci := range scIssues {
		if sci.Severity == severityCritical {
			hasMissingSC = true
			break
		}
	}

	var issues []issue

	volumeStatuses, _, _ := unstructured.NestedSlice(vm.Object, "status", "volumeSnapshotStatuses")
	for _, vs := range volumeStatuses {
		vsMap, ok := vs.(map[string]interface{})
		if !ok {
			continue
		}
		volName, _ := vsMap["name"].(string)
		reason, _ := vsMap["reason"].(string)
		if reason != "" && strings.Contains(reason, "not found") {
			if hasMissingSC {
				continue
			}
			issues = append(issues, issue{
				Severity: severityCritical,
				Message:  fmt.Sprintf("Volume %q: PVC not found. The backing storage does not exist.", volName),
				Fix:      fmt.Sprintf("Ensure the PVC or DataVolume for volume %q is created and in Bound state.", volName),
			})
		}
	}

	return issues
}

func checkStorageClass(ctx context.Context, dynamicClient dynamic.Interface, vm *unstructured.Unstructured) []issue {
	if vm == nil || dynamicClient == nil {
		return nil
	}

	dvTemplates, found, err := unstructured.NestedSlice(vm.Object, "spec", "dataVolumeTemplates")
	if err != nil || !found || len(dvTemplates) == 0 {
		return nil
	}

	var issues []issue
	for _, dvTemplate := range dvTemplates {
		dvMap, ok := dvTemplate.(map[string]interface{})
		if !ok {
			continue
		}
		metadata, _ := dvMap["metadata"].(map[string]interface{})
		dvName, _ := metadata["name"].(string)

		spec, _ := dvMap["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}

		scName := extractStorageClassName(spec)
		if scName == "" {
			continue
		}

		_, err := dynamicClient.Resource(kubevirt.StorageClassGVR).Get(ctx, scName, metav1.GetOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				issues = append(issues, issue{
					Severity: severityWarning,
					Message:  fmt.Sprintf("Unable to verify StorageClass %q for DataVolume %q: %v", scName, dvName, err),
				})
				continue
			}
			availableSCs := listAvailableStorageClasses(ctx, dynamicClient)
			issues = append(issues, issue{
				Severity: severityCritical,
				Message:  fmt.Sprintf("StorageClass %q referenced by DataVolume %q does not exist on this cluster. Available StorageClasses: %s.", scName, dvName, availableSCs),
				Fix:      fmt.Sprintf("Change the DataVolume storageClassName from %q to an existing StorageClass (e.g., %s).", scName, availableSCs),
			})
		}
	}

	return issues
}

func extractStorageClassName(dvSpec map[string]interface{}) string {
	if storage, ok := dvSpec["storage"].(map[string]interface{}); ok {
		if sc, ok := storage["storageClassName"].(string); ok && sc != "" {
			return sc
		}
	}
	if pvc, ok := dvSpec["pvc"].(map[string]interface{}); ok {
		if sc, ok := pvc["storageClassName"].(string); ok && sc != "" {
			return sc
		}
	}
	return ""
}

func listAvailableStorageClasses(ctx context.Context, dynamicClient dynamic.Interface) string {
	scList, err := dynamicClient.Resource(kubevirt.StorageClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil || len(scList.Items) == 0 {
		return "(none found)"
	}

	var names []string
	for _, sc := range scList.Items {
		name := sc.GetName()
		annotations := sc.GetAnnotations()
		if annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			name += " (default)"
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

func checkCloudInitCommands(vm, vmi *unstructured.Unstructured) []issue {
	var volumes []interface{}
	if vm != nil {
		volumes, _, _ = unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	}
	if len(volumes) == 0 && vmi != nil {
		volumes, _, _ = unstructured.NestedSlice(vmi.Object, "spec", "volumes")
	}
	if len(volumes) == 0 {
		return nil
	}

	runStrategy := ""
	if vm != nil {
		runStrategy, _, _ = unstructured.NestedString(vm.Object, "spec", "runStrategy")
	}

	var issues []issue

	for _, vol := range volumes {
		volMap, ok := vol.(map[string]interface{})
		if !ok {
			continue
		}

		for _, ciKey := range []string{"cloudInitNoCloud", "cloudInitConfigDrive"} {
			ciData, exists := volMap[ciKey].(map[string]interface{})
			if !exists {
				continue
			}

			userData, _ := ciData["userData"].(string)
			if userData == "" {
				continue
			}

			for _, cmd := range detectDangerousCloudInitCommands(userData) {
				msg := fmt.Sprintf("Cloud-init userData contains %q command that will shut down or restart the guest immediately after boot.", cmd)
				if runStrategy == "Always" || runStrategy == "" {
					msg += " Combined with runStrategy Always, this causes a crashloop."
				}
				fix := fmt.Sprintf("Remove the %q command from cloud-init userData, or change runStrategy to Manual/Halted if the shutdown is intentional.", cmd)
				issues = append(issues, issue{
					Severity: severityCritical,
					Message:  msg,
					Fix:      fix,
				})
			}
		}
	}

	return issues
}

func detectDangerousCloudInitCommands(userData string) []string {
	lines := strings.Split(userData, "\n")
	var matches []string
	seen := map[string]bool{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if isEchoOrPrintLine(trimmed) {
			continue
		}
		if isConfigKeyLine(trimmed) {
			continue
		}
		if cmd := matchDangerousCommand(trimmed); cmd != "" && !seen[cmd] {
			seen[cmd] = true
			matches = append(matches, cmd)
		}
	}
	return matches
}

func isEchoOrPrintLine(line string) bool {
	lower := strings.ToLower(line)
	prefixes := []string{"echo ", "echo'", "echo\"", "printf ", "- echo ", "- printf "}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) || strings.HasPrefix(strings.TrimPrefix(lower, "- "), p) {
			return true
		}
	}
	return false
}

func isConfigKeyLine(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, ":") && !strings.HasPrefix(lower, "-") && !strings.HasPrefix(lower, "[") {
		parts := strings.SplitN(lower, ":", 2)
		key := strings.TrimSpace(parts[0])
		if len(key) > 0 && !strings.Contains(key, " ") {
			return true
		}
	}
	return false
}

func matchDangerousCommand(line string) string {
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)

	stripped := strings.TrimPrefix(lower, "- ")
	stripped = strings.Trim(stripped, "[]\"' ")

	switch {
	case strings.HasPrefix(stripped, "shutdown -r") ||
		strings.HasPrefix(stripped, "/sbin/shutdown -r"):
		return "reboot"
	case stripped == "shutdown" || stripped == "shutdown -h now" ||
		stripped == "/sbin/shutdown" || strings.HasPrefix(stripped, "/sbin/shutdown ") ||
		strings.HasPrefix(stripped, "shutdown -h") ||
		strings.HasPrefix(stripped, "shutdown -P") || strings.HasPrefix(stripped, "shutdown -p"):
		return "shutdown"
	case stripped == "poweroff" || stripped == "/sbin/poweroff" ||
		stripped == "systemctl poweroff":
		return "poweroff"
	case stripped == "halt" || stripped == "/sbin/halt" ||
		stripped == "systemctl halt":
		return "halt"
	case stripped == "reboot" || stripped == "/sbin/reboot" ||
		stripped == "systemctl reboot":
		return "reboot"
	case stripped == "init 0" || stripped == "/sbin/init 0":
		return "init 0"
	case stripped == "init 6" || stripped == "/sbin/init 6":
		return "init 6"
	}

	if strings.Contains(lower, "[") {
		firstElem := extractFirstArrayElement(lower)
		switch firstElem {
		case "shutdown":
			return "shutdown"
		case "poweroff", "/sbin/poweroff":
			return "poweroff"
		case "halt", "/sbin/halt":
			return "halt"
		case "reboot", "/sbin/reboot":
			return "reboot"
		case "systemctl":
			start := strings.Index(lower, "[")
			if start != -1 {
				inner := lower[start+1:]
				if end := strings.Index(inner, "]"); end != -1 {
					inner = inner[:end]
				}
				elems := strings.Split(inner, ",")
				if len(elems) >= 2 {
					subCmd := strings.Trim(strings.TrimSpace(elems[1]), "\"' ")
					switch subCmd {
					case "poweroff":
						return "poweroff"
					case "halt":
						return "halt"
					case "reboot":
						return "reboot"
					}
				}
			}
		}
	}

	return ""
}

func extractFirstArrayElement(line string) string {
	start := strings.Index(line, "[")
	if start == -1 {
		return ""
	}
	inner := line[start+1:]
	end := strings.Index(inner, "]")
	if end != -1 {
		inner = inner[:end]
	}
	parts := strings.SplitN(inner, ",", 2)
	elem := strings.TrimSpace(parts[0])
	elem = strings.Trim(elem, "\"' ")
	return strings.ToLower(elem)
}

func checkNodeSelector(vm, vmi *unstructured.Unstructured) []issue {
	var nodeSelector map[string]interface{}

	if vm != nil {
		nodeSelector, _, _ = unstructured.NestedMap(vm.Object, "spec", "template", "spec", "nodeSelector")
	}
	if len(nodeSelector) == 0 && vmi != nil {
		nodeSelector, _, _ = unstructured.NestedMap(vmi.Object, "spec", "nodeSelector")
	}

	if len(nodeSelector) == 0 {
		return nil
	}

	for key, val := range nodeSelector {
		if key == "kubernetes.io/hostname" {
			hostname, _ := val.(string)
			return []issue{{
				Severity: severityWarning,
				Message:  fmt.Sprintf("VM is pinned to a specific node via nodeSelector (kubernetes.io/hostname=%s). This prevents live migration to other nodes.", hostname),
				Fix:      "Remove the kubernetes.io/hostname nodeSelector to allow migration, or use a broader label selector that matches multiple nodes.",
			}}
		}
	}

	var labels []string
	for k, v := range nodeSelector {
		labels = append(labels, fmt.Sprintf("%s=%v", k, v))
	}
	return []issue{{
		Severity: severityInfo,
		Message:  fmt.Sprintf("VM has nodeSelector constraints: %s. This may limit scheduling and migration options.", strings.Join(labels, ", ")),
	}}
}

func checkPodHealth(podDiagnostics []map[string]interface{}) []issue {
	if len(podDiagnostics) == 0 {
		return nil
	}

	var issues []issue

	for _, diag := range podDiagnostics {
		phase, _ := diag["phase"].(string)
		if phase == "Pending" {
			issues = append(issues, issue{
				Severity: severityWarning,
				Message:  "virt-launcher pod is in Pending state. The pod has not been scheduled yet (possible resource constraints or nodeSelector mismatch).",
				Fix:      "Check node resources and scheduling constraints. Ensure at least one node satisfies the pod's resource requests and node selectors.",
			})
		}

		containerStatuses, _ := diag["containerStatuses"].([]map[string]interface{})
		for _, cs := range containerStatuses {
			restartCount := toInt64(cs["restartCount"])
			if restartCount > 3 {
				containerName, _ := cs["name"].(string)
				issues = append(issues, issue{
					Severity: severityCritical,
					Message:  fmt.Sprintf("Container %q in virt-launcher pod has restarted %d times, indicating a crashloop.", containerName, restartCount),
				})
			}

			state, ok := cs["state"].(map[string]interface{})
			if !ok {
				continue
			}
			if waiting, ok := state["waiting"].(map[string]interface{}); ok {
				reason, _ := waiting["reason"].(string)
				if reason == "CrashLoopBackOff" || reason == "ErrImagePull" || reason == "ImagePullBackOff" {
					message, _ := waiting["message"].(string)
					issues = append(issues, issue{
						Severity: severityCritical,
						Message:  fmt.Sprintf("virt-launcher container is in %s state: %s", reason, message),
					})
				}
			}
			if terminated, ok := state["terminated"].(map[string]interface{}); ok {
				reason, _ := terminated["reason"].(string)
				if reason == "OOMKilled" {
					issues = append(issues, issue{
						Severity: severityCritical,
						Message:  "virt-launcher container was OOMKilled. The VM requires more memory than allocated.",
						Fix:      "Increase the memory resource limits for the VM or reduce the guest memory requirements.",
					})
				}
			}
		}
	}

	return issues
}

const maxReportedFailedMigrations = 3

func checkMigrationStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) []issue {
	if dynamicClient == nil {
		return nil
	}

	vmimList, err := dynamicClient.Resource(kubevirt.VirtualMachineInstanceMigrationGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubevirt.io/vmi-name=%s", name),
	})
	if err != nil || len(vmimList.Items) == 0 {
		return nil
	}

	var failed []unstructured.Unstructured
	for _, vmim := range vmimList.Items {
		vmiName, _, _ := unstructured.NestedString(vmim.Object, "spec", "vmiName")
		if vmiName != name {
			continue
		}
		phase, _, _ := unstructured.NestedString(vmim.Object, "status", "phase")
		if phase == "Failed" {
			failed = append(failed, vmim)
		}
	}

	if len(failed) == 0 {
		return nil
	}

	sort.Slice(failed, func(i, j int) bool {
		ti := failed[i].GetCreationTimestamp().Time
		tj := failed[j].GetCreationTimestamp().Time
		return ti.After(tj)
	})

	var issues []issue
	now := time.Now()
	limit := maxReportedFailedMigrations
	if len(failed) < limit {
		limit = len(failed)
	}

	for _, vmim := range failed[:limit] {
		migrationName := vmim.GetName()
		conditions, _, _ := unstructured.NestedSlice(vmim.Object, "status", "conditions")
		failureReason := extractMigrationFailureReason(conditions)

		msg := fmt.Sprintf("VirtualMachineInstanceMigration %q failed", migrationName)
		if created := vmim.GetCreationTimestamp(); !created.IsZero() {
			msg += fmt.Sprintf(" (%s ago)", formatDuration(now.Sub(created.Time)))
		}
		msg += "."
		if failureReason != "" {
			msg += " Reason: " + failureReason
		}
		issues = append(issues, issue{
			Severity: severityCritical,
			Message:  msg,
			Fix:      "Check nodeSelector constraints, resource availability on target nodes, and migration policies. Remove hostname-specific nodeSelector to allow migration.",
		})
	}

	if len(failed) > maxReportedFailedMigrations {
		issues = append(issues, issue{
			Severity: severityInfo,
			Message:  fmt.Sprintf("%d additional older failed migrations not shown.", len(failed)-maxReportedFailedMigrations),
		})
	}

	return issues
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func extractMigrationFailureReason(conditions []interface{}) string {
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := condMap["type"].(string)
		status, _ := condMap["status"].(string)
		if condType == "Failure" && status == "True" {
			message, _ := condMap["message"].(string)
			return message
		}
	}
	return ""
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int32:
		return int64(n)
	default:
		return 0
	}
}
