package troubleshoot

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
)

func Tools(p api.FilteringProvider) []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name: "vm_troubleshoot",
				Description: fmt.Sprintf(
					"Diagnose %s VirtualMachine issues with automated root-cause detection. "+
						"Collects VM status, VMI status, volumes, DataVolume/PVC state, cloud-init configuration, pod state, logs, and events, "+
						"then runs heuristic checks to identify specific problems and suggest fixes. "+
						"Returns a 'Detected Issues' section with CRITICAL/WARNING findings and actionable remediation steps, followed by raw diagnostic data. "+
						"Use this tool FIRST whenever a user asks why a VM is not starting, stuck in Provisioning, crashlooping, failing to migrate, or exhibiting unexpected behavior. "+
						"Automatically detects: missing StorageClasses, invalid PVC specs, "+
						"dangerous cloud-init commands (shutdown/halt), "+
						"nodeSelector migration blockers, failed migrations, "+
						"and pod crashloops. "+
						"If the user asks to fix or remediate the issue, "+
						"use the Suggested Fixes from the report with "+
						"vm_lifecycle (restart) or resources_create_or_update.",
					defaults.ProductName()),
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the VirtualMachine to troubleshoot",
						},
						"name": {
							Type:        "string",
							Description: "The name of the VirtualMachine to troubleshoot",
						},
					},
					Required: []string{"namespace", "name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Troubleshoot",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			TargetCompatibilityFilters: []func() bool{
				kubevirt.HasVirtualMachine(p),
			},
			Handler: troubleshoot,
		},
	}
}

func troubleshoot(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	ctx := params.Context
	dynamicClient := params.DynamicClient()

	vmYaml, vm := fetchVMStatus(ctx, dynamicClient, namespace, name)
	vmiYaml, vmi := fetchVMIStatus(ctx, dynamicClient, namespace, name)
	volumesYaml := fetchVolumes(namespace, name, vm, vmi)
	dataVolumeYaml := fetchDataVolumeStatus(ctx, dynamicClient, namespace, vm)
	cloudInitYaml := extractCloudInit(vm, vmi)
	podYaml, podNames, podDiag := fetchVirtLauncherPod(ctx, dynamicClient, namespace, name)
	podLogsText := fetchVirtLauncherPodLogs(ctx, params.KubernetesClient, namespace, podNames)
	eventsYaml := fetchEvents(ctx, params.KubernetesClient, namespace, name, podNames)

	issuesSection := analyzeIssues(ctx, dynamicClient, namespace, name, vm, vmi, podDiag)

	report := fmt.Sprintf(`# VirtualMachine Diagnostic Report: %s/%s

%s

%s

%s

%s

%s

%s

%s

%s

%s
`, namespace, name, issuesSection, vmYaml, vmiYaml, volumesYaml, dataVolumeYaml, cloudInitYaml, podYaml, podLogsText, eventsYaml)

	return api.NewToolCallResult(report, nil), nil
}

func fetchVMStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (string, *unstructured.Unstructured) {
	vm, err := dynamicClient.Resource(kubevirt.VirtualMachineGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("## VirtualMachine\n\n*Error: %v*", err), nil
	}

	status, found, err := unstructured.NestedMap(vm.Object, "status")
	if err != nil {
		return fmt.Sprintf("## VirtualMachine\n\n*Error extracting status: %v*", err), vm
	}
	if !found {
		return fmt.Sprintf("## VirtualMachine: %s/%s\n\n*No status found (VM may not have been reconciled yet)*", namespace, name), vm
	}

	yamlStr, err := output.MarshalYaml(status)
	if err != nil {
		return fmt.Sprintf("## VirtualMachine\n\n*Error marshaling status: %v*", err), vm
	}

	return fmt.Sprintf("## VirtualMachine Status: %s/%s\n\n```yaml\n%s```", namespace, name, yamlStr), vm
}

func fetchVMIStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (string, *unstructured.Unstructured) {
	vmi, err := dynamicClient.Resource(kubevirt.VirtualMachineInstanceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("## VirtualMachineInstance\n\n*VMI not found: %v*\n\n(Expected if VM is stopped or stuck provisioning)", err), nil
	}

	status, found, err := unstructured.NestedMap(vmi.Object, "status")
	if err != nil {
		return fmt.Sprintf("## VirtualMachineInstance\n\n*Error extracting status: %v*", err), vmi
	}
	if !found {
		return fmt.Sprintf("## VirtualMachineInstance: %s/%s\n\n*No status found*", namespace, name), vmi
	}

	yamlStr, err := output.MarshalYaml(status)
	if err != nil {
		return fmt.Sprintf("## VirtualMachineInstance\n\n*Error marshaling status: %v*", err), vmi
	}

	return fmt.Sprintf("## VirtualMachineInstance Status: %s/%s\n\n```yaml\n%s```", namespace, name, yamlStr), vmi
}

func fetchVolumes(namespace, name string, vm, vmi *unstructured.Unstructured) string {
	var volumes []any
	var found bool
	var err error
	source := "VirtualMachine"

	if vm != nil {
		volumes, found, err = unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
		if err != nil {
			return "*Error extracting volumes from VirtualMachine: " + err.Error() + "*"
		}
	}

	if (!found || len(volumes) == 0) && vmi != nil {
		volumes, found, err = unstructured.NestedSlice(vmi.Object, "spec", "volumes")
		if err != nil {
			return "*Error extracting volumes from VirtualMachineInstance: " + err.Error() + "*"
		}
		if found && len(volumes) > 0 {
			source = "VirtualMachineInstance"
		}
	}

	if !found || len(volumes) == 0 {
		return "## Volumes\n\n*No volumes configured*"
	}

	sanitized := stripCloudInitBodies(volumes)

	yamlStr, err := output.MarshalYaml(sanitized)
	if err != nil {
		return "*Error marshaling volumes: " + err.Error() + "*"
	}

	return fmt.Sprintf("## Volumes (from %s: %s/%s)\n\n```yaml\n%s```", source, namespace, name, yamlStr)
}

func stripCloudInitBodies(volumes []any) []any {
	result := make([]any, 0, len(volumes))
	for _, vol := range volumes {
		volMap, ok := vol.(map[string]interface{})
		if !ok {
			result = append(result, vol)
			continue
		}
		clean := make(map[string]interface{}, len(volMap))
		for k, v := range volMap {
			if k == "cloudInitNoCloud" || k == "cloudInitConfigDrive" {
				ciMap, ok := v.(map[string]interface{})
				if !ok {
					clean[k] = v
					continue
				}
				stripped := make(map[string]interface{}, len(ciMap))
				for ck, cv := range ciMap {
					switch ck {
					case "userData", "networkData", "userDataBase64", "networkDataBase64":
						stripped[ck] = "<see Cloud-Init Configuration section>"
					default:
						stripped[ck] = cv
					}
				}
				clean[k] = stripped
			} else {
				clean[k] = v
			}
		}
		result = append(result, clean)
	}
	return result
}

func fetchVirtLauncherPod(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (string, []string, []map[string]interface{}) {
	labelSelector := fmt.Sprintf("kubevirt.io=virt-launcher,vm.kubevirt.io/name=%s", name)
	podList, err := dynamicClient.Resource(kubevirt.PodGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Sprintf("## virt-launcher Pod\n\n*Error listing pods: %v*", err), nil, nil
	}

	if len(podList.Items) == 0 {
		return "## virt-launcher Pod\n\n*No virt-launcher pod found (VM may be stopped or not yet scheduled)*", nil, nil
	}

	var result strings.Builder
	var podNames []string
	var allDiags []map[string]interface{}
	result.WriteString("## virt-launcher Pod\n\n")
	for _, pod := range podList.Items {
		podNames = append(podNames, pod.GetName())
		summary := extractPodDiagnostics(&pod)
		allDiags = append(allDiags, summary)
		yamlStr, err := output.MarshalYaml(summary)
		if err != nil {
			fmt.Fprintf(&result, "*Error marshaling pod %s: %v*\n\n", pod.GetName(), err)
			continue
		}
		fmt.Fprintf(&result, "### %s\n\n```yaml\n%s```\n\n", pod.GetName(), yamlStr)
	}

	return result.String(), podNames, allDiags
}

func extractPodDiagnostics(pod *unstructured.Unstructured) map[string]interface{} {
	diag := map[string]interface{}{}

	phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
	diag["phase"] = phase

	nodeName, _, _ := unstructured.NestedString(pod.Object, "spec", "nodeName")
	if nodeName != "" {
		diag["nodeName"] = nodeName
	}

	conditions, _, _ := unstructured.NestedSlice(pod.Object, "status", "conditions")
	if len(conditions) > 0 {
		diag["conditions"] = conditions
	}

	containerStatuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
	if len(containerStatuses) > 0 {
		var summaries []map[string]interface{}
		for _, cs := range containerStatuses {
			csMap, ok := cs.(map[string]interface{})
			if !ok {
				continue
			}
			summary := map[string]interface{}{
				"name":  csMap["name"],
				"ready": csMap["ready"],
			}
			if rc, ok := csMap["restartCount"]; ok {
				summary["restartCount"] = rc
			}
			if state, ok := csMap["state"]; ok {
				summary["state"] = state
			}
			if lastState, ok := csMap["lastTerminationState"]; ok {
				stateMap, _ := lastState.(map[string]interface{})
				if len(stateMap) > 0 {
					summary["lastTerminationState"] = lastState
				}
			}
			summaries = append(summaries, summary)
		}
		diag["containerStatuses"] = summaries
	}

	nodeSelector, _, _ := unstructured.NestedMap(pod.Object, "spec", "nodeSelector")
	if len(nodeSelector) > 0 {
		diag["nodeSelector"] = nodeSelector
	}

	return diag
}

func fetchVirtLauncherPodLogs(ctx context.Context, client api.KubernetesClient, namespace string, podNames []string) string {
	if len(podNames) == 0 {
		return "## virt-launcher Pod Logs\n\n*No pod found — no logs available*"
	}

	core := kubernetes.NewCore(client)
	var result strings.Builder
	result.WriteString("## virt-launcher Pod Logs\n\n")

	containerName := "compute"
	for _, podName := range podNames {
		logs, err := core.PodsLog(ctx, namespace, podName, containerName, false, 50)
		if err != nil {
			fmt.Fprintf(&result, "### %s\n\n*Error fetching logs: %v*\n\n", podName, err)
			continue
		}
		fmt.Fprintf(&result, "### %s (container: %s)\n\n```\n%s\n```\n\n", podName, containerName, redactLogSecrets(logs))
	}

	return result.String()
}

func fetchEvents(ctx context.Context, client api.KubernetesClient, namespace, vmName string, podNames []string) string {
	core := kubernetes.NewCore(client)

	objectNames := []string{vmName}
	objectNames = append(objectNames, podNames...)

	var relatedEvents []map[string]any
	var eventErrors []string
	for _, objName := range objectNames {
		events, err := core.EventsList(ctx, namespace, api.ListOptions{
			ListOptions: metav1.ListOptions{FieldSelector: "involvedObject.name=" + objName},
		})
		if err != nil {
			eventErrors = append(eventErrors, fmt.Sprintf("Error fetching events for %s: %v", objName, err))
			continue
		}
		relatedEvents = append(relatedEvents, events...)
	}

	if len(relatedEvents) == 0 && len(eventErrors) == 0 {
		return "## Events\n\n*No events found related to this VM*"
	}

	var result strings.Builder
	result.WriteString("## Events")
	if len(eventErrors) > 0 {
		result.WriteString("\n\n")
		for _, e := range eventErrors {
			fmt.Fprintf(&result, "*%s*\n", e)
		}
	}

	if len(relatedEvents) > 0 {
		yamlStr, err := output.MarshalYaml(relatedEvents)
		if err != nil {
			fmt.Fprintf(&result, "\n\n*Error marshaling events: %v*", err)
		} else {
			fmt.Fprintf(&result, "\n\n```yaml\n%s```", yamlStr)
		}
	}

	return result.String()
}

func fetchDataVolumeStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace string, vm *unstructured.Unstructured) string {
	if vm == nil {
		return "## DataVolume/PVC Status\n\n*No VM available to extract DataVolume references*"
	}

	dvTemplates, found, err := unstructured.NestedSlice(vm.Object, "spec", "dataVolumeTemplates")
	if err != nil || !found || len(dvTemplates) == 0 {
		return "## DataVolume/PVC Status\n\n*No dataVolumeTemplates defined in VM spec*"
	}

	var result strings.Builder
	result.WriteString("## DataVolume/PVC Status\n\n")

	for _, dvTemplate := range dvTemplates {
		dvMap, ok := dvTemplate.(map[string]interface{})
		if !ok {
			continue
		}
		metadata, _ := dvMap["metadata"].(map[string]interface{})
		dvName, _ := metadata["name"].(string)
		if dvName == "" {
			continue
		}

		dv, err := dynamicClient.Resource(kubevirt.DataVolumeGVR).Namespace(namespace).Get(ctx, dvName, metav1.GetOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				fmt.Fprintf(&result, "### %s\n\n*Error fetching DataVolume: %v*\n\n", dvName, err)
				continue
			}
			fmt.Fprintf(&result, "### %s\n\n*DataVolume not found*\n\n", dvName)
			pvc, pvcErr := dynamicClient.Resource(kubevirt.PersistentVolumeClaimGVR).Namespace(namespace).Get(ctx, dvName, metav1.GetOptions{})
			if pvcErr != nil {
				fmt.Fprintf(&result, "*PVC also not found: %v*\n\n", pvcErr)
			} else {
				pvcStatus, _, _ := unstructured.NestedMap(pvc.Object, "status")
				if pvcStatus != nil {
					yamlStr, marshalErr := output.MarshalYaml(pvcStatus)
					if marshalErr != nil {
						fmt.Fprintf(&result, "*Error marshaling PVC status: %v*\n\n", marshalErr)
					} else {
						fmt.Fprintf(&result, "PVC exists with status:\n```yaml\n%s```\n\n", yamlStr)
					}
				}
			}
			continue
		}

		dvStatus, found, _ := unstructured.NestedMap(dv.Object, "status")
		if !found {
			fmt.Fprintf(&result, "### %s\n\n*DataVolume exists but has no status*\n\n", dvName)
			continue
		}

		yamlStr, err := output.MarshalYaml(dvStatus)
		if err != nil {
			fmt.Fprintf(&result, "### %s\n\n*Error marshaling DataVolume status: %v*\n\n", dvName, err)
			continue
		}

		dvSpec, _, _ := unstructured.NestedMap(dv.Object, "spec")
		storageClass := ""
		if dvSpec != nil {
			sc, _, _ := unstructured.NestedString(dvSpec, "storage", "storageClassName")
			if sc != "" {
				storageClass = sc
			}
		}

		fmt.Fprintf(&result, "### %s", dvName)
		if storageClass != "" {
			fmt.Fprintf(&result, " (storageClass: %s)", storageClass)
		}
		fmt.Fprintf(&result, "\n\n```yaml\n%s```\n\n", yamlStr)
	}

	return result.String()
}

func extractCloudInit(vm, vmi *unstructured.Unstructured) string {
	var volumes []interface{}
	if vm != nil {
		volumes, _, _ = unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	}
	if len(volumes) == 0 && vmi != nil {
		volumes, _, _ = unstructured.NestedSlice(vmi.Object, "spec", "volumes")
	}
	if len(volumes) == 0 {
		return "## Cloud-Init Configuration\n\n*No volumes found*"
	}

	var result strings.Builder
	found := false

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
			found = true
			if result.Len() == 0 {
				result.WriteString("## Cloud-Init Configuration\n\n")
			}

			volName, _ := volMap["name"].(string)
			fmt.Fprintf(&result, "### %s (type: %s)\n\n", volName, ciKey)

			userData := cloudInitData(ciData, "userData", "userDataBase64")
			if userData != "" {
				fmt.Fprintf(&result, "```yaml\n%s\n```\n\n", redactCloudInitSensitiveFields(userData))
			}
			networkData := cloudInitData(ciData, "networkData", "networkDataBase64")
			if networkData != "" {
				fmt.Fprintf(&result, "**networkData:**\n```yaml\n%s\n```\n\n", redactCloudInitSensitiveFields(networkData))
			}
			if userData == "" && networkData == "" {
				secretRef, _ := ciData["userDataSecretRef"].(map[string]interface{})
				if secretRef != nil {
					fmt.Fprintf(&result, "*userData from Secret: %v*\n\n", secretRef["name"])
				}
			}
		}
	}

	if !found {
		return "## Cloud-Init Configuration\n\n*No cloud-init volumes configured*"
	}
	return result.String()
}

// cloudInitData returns the cloud-init payload for a field, preferring the
// plain-text key and falling back to the base64-encoded variant. This keeps the
// content diagnosable while ensuring it always flows through redaction (the raw
// volume dump strips both variants, see stripCloudInitBodies).
func cloudInitData(ciData map[string]interface{}, plainKey, base64Key string) string {
	if plain, _ := ciData[plainKey].(string); plain != "" {
		return plain
	}
	encoded, _ := ciData[base64Key].(string)
	if encoded == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return ""
	}
	return string(decoded)
}

var sensitiveYAMLKeys = []string{
	"password",
	"passwd",
	"plain_text_passwd",
	"hashed_passwd",
	"ssh_authorized_keys",
	"ssh_keys",
	"ca-cert",
	"client-cert",
	"client-key",
	"token",
	"chpasswd",
	"write_files",
	"rsa_private",
	"rsa_public",
	"dsa_private",
	"dsa_public",
	"ecdsa_private",
	"ecdsa_public",
	"ed25519_private",
	"ed25519_public",
}

var sensitiveLinePatterns = []string{
	"ssh-rsa ",
	"ssh-ed25519 ",
	"ecdsa-sha2-",
	"ssh-dss ",
	"sk-ssh-ed25519@openssh.com ",
	"sk-ecdsa-sha2-",
	"-----begin",
}

func redactCloudInitSensitiveFields(userData string) string {
	lines := strings.Split(userData, "\n")
	var result []string
	redactIndent := -1

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := leadingSpaces(line)

		if redactIndent >= 0 {
			if len(trimmed) == 0 || indent > redactIndent {
				continue
			}
			redactIndent = -1
		}

		if isSensitiveKeyLine(trimmed) {
			key := strings.SplitN(trimmed, ":", 2)[0]
			valuePart := strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1])
			if valuePart == "" || isBlockScalarIndicator(valuePart) {
				redactIndent = indent
			}
			result = append(result, strings.Repeat(" ", indent)+key+": <REDACTED>")
			continue
		}

		if isSensitiveValueLine(trimmed) {
			result = append(result, strings.Repeat(" ", indent)+"<REDACTED>")
			continue
		}

		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func isSensitiveKeyLine(trimmed string) bool {
	lower := strings.ToLower(trimmed)
	for _, key := range sensitiveYAMLKeys {
		if strings.HasPrefix(lower, key+":") {
			return true
		}
	}
	return false
}

func isSensitiveValueLine(trimmed string) bool {
	lower := strings.ToLower(trimmed)
	for _, pat := range sensitiveLinePatterns {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

var logSecretPatterns = []string{
	"bearer ",
	"authorization:",
	"authorization=",
	"-u ", "--user ",
	"-----begin",
	"ssh-rsa ", "ssh-ed25519 ", "ssh-dss ",
	"password=", "passwd=", "token=",
}

func redactLogSecrets(logs string) string {
	lines := strings.Split(logs, "\n")
	var result []string
	for _, line := range lines {
		lower := strings.ToLower(line)
		redacted := false
		for _, pat := range logSecretPatterns {
			if strings.Contains(lower, pat) {
				idx := strings.Index(lower, pat)
				result = append(result, line[:idx+len(pat)]+"<REDACTED>")
				redacted = true
				break
			}
		}
		if !redacted {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func leadingSpaces(s string) int {
	for i, c := range s {
		if c != ' ' && c != '\t' {
			return i
		}
	}
	return len(s)
}

func isBlockScalarIndicator(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] != '|' && s[0] != '>' {
		return false
	}
	for _, c := range s[1:] {
		if c != '+' && c != '-' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
