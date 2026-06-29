package troubleshoot

import (
	"context"
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

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name: "vm_troubleshoot",
				Description: fmt.Sprintf(
					"Diagnose %s VirtualMachine issues by collecting VM status, VMI status, volumes, DataVolume/PVC state, cloud-init configuration, virt-launcher pod state, pod logs, and related events. "+
						"Returns a structured diagnostic report with root-cause data that goes beyond what alerts provide. "+
						"Use this tool FIRST whenever a user asks why a VM is not starting, stuck in Provisioning, crashlooping, failing to migrate, or exhibiting unexpected behavior. "+
						"Identifies issues such as missing StorageClasses, invalid PVC specs, misconfigured cloud-init, and scheduling constraints.",
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
	if ctx == nil {
		ctx = context.Background()
	}

	dynamicClient := params.DynamicClient()

	vmYaml, vm := fetchVMStatus(ctx, dynamicClient, namespace, name)
	vmiYaml, vmi := fetchVMIStatus(ctx, dynamicClient, namespace, name)
	volumesYaml := fetchVolumes(namespace, name, vm, vmi)
	dataVolumeYaml := fetchDataVolumeStatus(ctx, dynamicClient, namespace, vm)
	cloudInitYaml := extractCloudInit(vm, vmi)
	podYaml, podNames := fetchVirtLauncherPod(ctx, dynamicClient, namespace, name)
	podLogsText := fetchVirtLauncherPodLogs(ctx, params.KubernetesClient, namespace, podNames)
	eventsYaml := fetchEvents(ctx, params.KubernetesClient, namespace, name, podNames)

	report := fmt.Sprintf(`# VirtualMachine Diagnostic Report: %s/%s

%s

%s

%s

%s

%s

%s

%s

%s
`, namespace, name, vmYaml, vmiYaml, volumesYaml, dataVolumeYaml, cloudInitYaml, podYaml, podLogsText, eventsYaml)

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

	yamlStr, err := output.MarshalYaml(volumes)
	if err != nil {
		return "*Error marshaling volumes: " + err.Error() + "*"
	}

	return fmt.Sprintf("## Volumes (from %s: %s/%s)\n\n```yaml\n%s```", source, namespace, name, yamlStr)
}

func fetchVirtLauncherPod(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (string, []string) {
	labelSelector := fmt.Sprintf("kubevirt.io=virt-launcher,vm.kubevirt.io/name=%s", name)
	podList, err := dynamicClient.Resource(kubevirt.PodGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Sprintf("## virt-launcher Pod\n\n*Error listing pods: %v*", err), nil
	}

	if len(podList.Items) == 0 {
		return "## virt-launcher Pod\n\n*No virt-launcher pod found (VM may be stopped or not yet scheduled)*", nil
	}

	var result strings.Builder
	var podNames []string
	result.WriteString("## virt-launcher Pod\n\n")
	for _, pod := range podList.Items {
		podNames = append(podNames, pod.GetName())
		summary := extractPodDiagnostics(&pod)
		yamlStr, err := output.MarshalYaml(summary)
		if err != nil {
			fmt.Fprintf(&result, "*Error marshaling pod %s: %v*\n\n", pod.GetName(), err)
			continue
		}
		fmt.Fprintf(&result, "### %s\n\n```yaml\n%s```\n\n", pod.GetName(), yamlStr)
	}

	return result.String(), podNames
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
		fmt.Fprintf(&result, "### %s (container: %s)\n\n```\n%s\n```\n\n", podName, containerName, logs)
	}

	return result.String()
}

func fetchEvents(ctx context.Context, client api.KubernetesClient, namespace, vmName string, podNames []string) string {
	core := kubernetes.NewCore(client)

	objectNames := []string{vmName}
	objectNames = append(objectNames, podNames...)

	var relatedEvents []map[string]any
	for _, objName := range objectNames {
		events, err := core.EventsList(ctx, namespace, api.ListOptions{
			ListOptions: metav1.ListOptions{FieldSelector: "involvedObject.name=" + objName},
		})
		if err != nil {
			continue
		}
		relatedEvents = append(relatedEvents, events...)
	}

	if len(relatedEvents) == 0 {
		return "## Events\n\n*No events found related to this VM*"
	}

	yamlStr, err := output.MarshalYaml(relatedEvents)
	if err != nil {
		return fmt.Sprintf("## Events\n\n*Error marshaling events: %v*", err)
	}

	return fmt.Sprintf("## Events (related to %s)\n\n```yaml\n%s```", vmName, yamlStr)
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
					yamlStr, _ := output.MarshalYaml(pvcStatus)
					fmt.Fprintf(&result, "PVC exists with status:\n```yaml\n%s```\n\n", yamlStr)
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

			userData, _ := ciData["userData"].(string)
			if userData != "" {
				fmt.Fprintf(&result, "```yaml\n%s\n```\n\n", redactCloudInitSensitiveFields(userData))
			}
			networkData, _ := ciData["networkData"].(string)
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

var sensitiveYAMLKeys = []string{
	"password",
	"passwd",
	"ssh_authorized_keys",
	"ca-cert",
	"client-cert",
	"client-key",
	"token",
}

var sensitiveLinePatterns = []string{
	"ssh-rsa ",
	"ssh-ed25519 ",
	"ecdsa-sha2-",
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
			if valuePart == "|" || valuePart == ">" || valuePart == "|+" || valuePart == "|-" {
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

func leadingSpaces(s string) int {
	for i, c := range s {
		if c != ' ' && c != '\t' {
			return i
		}
	}
	return len(s)
}
