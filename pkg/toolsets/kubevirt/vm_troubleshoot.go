package kubevirt

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

// initVMTroubleshoot initializes the VM troubleshooting prompt
func initVMTroubleshoot() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "vm-troubleshoot",
				Title:       "VirtualMachine Troubleshoot",
				Description: "Generate a step-by-step troubleshooting guide for diagnosing VirtualMachine issues",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "The namespace of the VirtualMachine to troubleshoot",
						Required:    true,
					},
					{
						Name:        "name",
						Description: "The name of the VirtualMachine to troubleshoot",
						Required:    true,
					},
				},
			},
			Handler: vmTroubleshootHandler,
		},
	}
}

// vmTroubleshootHandler implements the VM troubleshooting prompt
func vmTroubleshootHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]
	name := args["name"]

	if namespace == "" {
		return nil, fmt.Errorf("namespace argument is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name argument is required")
	}

	ctx := params.Context
	if ctx == nil {
		ctx = context.Background()
	}

	dynamicClient := params.DynamicClient()

	// Fetch all relevant resources
	vmYaml, vm := fetchVirtualMachineStatus(ctx, dynamicClient, namespace, name)
	vmiYaml, vmi := fetchVirtualMachineInstanceStatus(ctx, dynamicClient, namespace, name)
	volumesYaml := fetchVirtualMachineVolumes(namespace, name, vm, vmi)
	podYaml, podNames := fetchVirtLauncherPod(ctx, dynamicClient, namespace, name)
	podLogsText := fetchVirtLauncherPodLogs(ctx, params.KubernetesClient, namespace, podNames)
	eventsYaml := fetchEvents(ctx, params.KubernetesClient, namespace, name)

	// Build the troubleshooting guide message with embedded resource data
	guideText := fmt.Sprintf(`# VirtualMachine Troubleshooting Guide

## VM: %s (namespace: %s)

Use this guide to diagnose issues with the VirtualMachine. The relevant resource data has been collected below.

---

## Step 1: VirtualMachine Status

Check the VirtualMachine status:
- printableStatus (should be "Running")
- ready (should be true)
- conditions for any errors

%s

---

## Step 2: VirtualMachineInstance Status

Check the VirtualMachineInstance status:
- phase (should be "Running")
- conditions for "Ready" condition

%s

---

## Step 3: VM Volumes

Review the VM volumes for any configuration issues:

%s

**Common volume types to check:**
- **containerDisk**: Verify the image URL is accessible and the image exists
- **cloudInitNoCloud / cloudInitConfigDrive**: Check userData and networkData for syntax errors
- **configMap**: Verify the referenced ConfigMap exists in the namespace
- **secret**: Verify the referenced Secret exists in the namespace
- **serviceAccount**: Check if the ServiceAccount exists and has proper permissions
- **emptyDisk**: Usually no issues, but check if capacity is specified correctly
- **hostDisk**: Verify the path exists on the host node and has correct permissions
- **downwardMetrics**: Usually no configuration issues
- **dataVolume**: Check that the DataVolume exists and status.phase is "Succeeded"
- **persistentVolumeClaim**: Check that the PVC exists and status.phase is "Bound"

---

## Step 4: virt-launcher Pod

Check the virt-launcher pod:
- Pod should be "Running" with all containers ready
- Check for container restart counts
- Review pod conditions for issues

%s

---

## Step 5: virt-launcher Pod Logs

Review the logs from the virt-launcher pod containers:
- Look for error messages or stack traces
- Check for VM boot issues or QEMU errors

%s

---

## Step 6: Events

Review events related to the VM and its components:
- Look for Warning events
- Check for scheduling, storage, or network issues

%s

---

## Troubleshooting Analysis

Based on the data above, analyze:

1. **VM State**: Is the VM in the expected state (Running/Stopped)?
2. **VMI State**: Does the VMI exist and is it Running?
3. **Volumes**: Are all referenced volumes (DataVolumes, PVCs, ConfigMaps, Secrets, container images) available?
4. **Pod Health**: Is the virt-launcher pod running without restarts?
5. **Events**: Are there any Warning events indicating problems?

---

## Fix the Issue

If you identified a problem, attempt to fix it:

1. **Missing Resources**: Create missing ConfigMaps, Secrets, or PVCs referenced by the VM
2. **Storage Issues**: Check DataVolume import status, fix storage class or access mode issues
3. **Image Pull Errors**: Verify container disk image URLs are correct and accessible
4. **Configuration Errors**: Fix any syntax errors in cloud-init userData or other configurations
5. **Resource Constraints**: Adjust VM resource requests if the pod cannot be scheduled
6. **Restart VM**: If the VM is stuck, try stopping and starting it again

Use the available Kubernetes tools to apply fixes (create resources, update configurations, restart VM).

---

## Report Findings

After completing troubleshooting and attempting fixes, report:
- **Status:** Running/Stopped/Failed/Provisioning
- **Root Cause:** Description or "None found"
- **Action Taken:** What was done to fix the issue (or "None" if no fix was needed/possible)
- **Result:** Whether the fix was successful or further action is needed
`, name, namespace, vmYaml, vmiYaml, volumesYaml, podYaml, podLogsText, eventsYaml)

	return api.NewPromptCallResult(
		"VirtualMachine troubleshooting guide generated",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: guideText,
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: "I'll analyze the collected data to diagnose the VirtualMachine issues systematically.",
				},
			},
		},
		nil,
	), nil
}

// fetchVirtualMachineStatus fetches the VirtualMachine resource and returns its status as YAML
// Also returns the VM object for reuse by other functions
func fetchVirtualMachineStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (string, *unstructured.Unstructured) {
	vm, err := dynamicClient.Resource(kubevirt.VirtualMachineGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("### VirtualMachine Status\n\n*Error fetching VirtualMachine: %v*", err), nil
	}

	status, found, err := unstructured.NestedMap(vm.Object, "status")
	if err != nil {
		return fmt.Sprintf("### VirtualMachine Status\n\n*Error extracting status: %v*", err), vm
	}
	if !found {
		return fmt.Sprintf("### VirtualMachine Status: %s/%s\n\n*No status found*", namespace, name), vm
	}

	yamlStr, err := output.MarshalYaml(status)
	if err != nil {
		return fmt.Sprintf("### VirtualMachine Status\n\n*Error marshaling VirtualMachine status: %v*", err), vm
	}

	return fmt.Sprintf("### VirtualMachine Status: %s/%s\n\n```yaml\n%s```", namespace, name, yamlStr), vm
}

// fetchVirtualMachineVolumes extracts volumes from the VM or VMI objects
// It first tries to get volumes from VM, and falls back to VMI if VM is nil
func fetchVirtualMachineVolumes(namespace, name string, vm, vmi *unstructured.Unstructured) string {
	var volumes []any
	var found bool
	var err error
	source := "VirtualMachine"

	// First try to get volumes from VM
	if vm != nil {
		volumes, found, err = unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
		if err != nil {
			return "*Error extracting volumes from VirtualMachine: " + err.Error() + "*"
		}

	}

	// If VM doesn't exist or has no volumes, try to get from VMI
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
		return "*No volumes configured*"
	}

	yamlStr, err := output.MarshalYaml(volumes)
	if err != nil {
		return "*Error marshaling volumes: " + err.Error() + "*"
	}

	return fmt.Sprintf("### Configured Volumes (from %s: %s/%s)\n\n```yaml\n%s```", source, namespace, name, yamlStr)
}

// fetchVirtualMachineInstanceStatus fetches the VMI resource and returns its status as YAML
// Also returns the VMI object for reuse by other functions
func fetchVirtualMachineInstanceStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (string, *unstructured.Unstructured) {
	vmi, err := dynamicClient.Resource(kubevirt.VirtualMachineInstanceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("### VirtualMachineInstance Status\n\n*VirtualMachineInstance not found or error: %v*\n\n(This may be expected if the VM is stopped)", err), nil
	}

	status, found, err := unstructured.NestedMap(vmi.Object, "status")
	if err != nil {
		return fmt.Sprintf("### VirtualMachineInstance Status\n\n*Error extracting status: %v*", err), vmi
	}
	if !found {
		return fmt.Sprintf("### VirtualMachineInstance Status: %s/%s\n\n*No status found*", namespace, name), vmi
	}

	yamlStr, err := output.MarshalYaml(status)
	if err != nil {
		return fmt.Sprintf("### VirtualMachineInstance Status\n\n*Error marshaling VirtualMachineInstance status: %v*", err), vmi
	}

	return fmt.Sprintf("### VirtualMachineInstance Status: %s/%s\n\n```yaml\n%s```", namespace, name, yamlStr), vmi
}

// fetchVirtLauncherPod fetches the virt-launcher pod for the VM and returns it as YAML
// Also returns the list of pod names for log fetching
func fetchVirtLauncherPod(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (string, []string) {
	// Use label selector to find virt-launcher pod for this VM
	labelSelector := fmt.Sprintf("kubevirt.io=virt-launcher,vm.kubevirt.io/name=%s", name)
	podList, err := dynamicClient.Resource(kubevirt.PodGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Sprintf("### virt-launcher Pod\n\n*Error listing pods: %v*", err), nil
	}

	if len(podList.Items) == 0 {
		return "### virt-launcher Pod\n\n*No virt-launcher pod found*\n\n(This is expected if the VM is stopped or the VMI hasn't been scheduled yet)", nil
	}

	var result strings.Builder
	var podNames []string
	result.WriteString("### virt-launcher Pod\n\n")
	for _, pod := range podList.Items {
		podNames = append(podNames, pod.GetName())
		yamlStr, err := output.MarshalYaml(&pod)
		if err != nil {
			result.WriteString(fmt.Sprintf("*Error marshaling pod %s: %v*\n\n", pod.GetName(), err))
			continue
		}
		result.WriteString(fmt.Sprintf("#### %s\n\n```yaml\n%s```\n\n", pod.GetName(), yamlStr))
	}

	return result.String(), podNames
}

// fetchVirtLauncherPodLogs fetches logs from the virt-launcher pod containers
func fetchVirtLauncherPodLogs(ctx context.Context, client api.KubernetesClient, namespace string, podNames []string) string {
	if len(podNames) == 0 {
		return "### virt-launcher Pod Logs\n\n*No virt-launcher pod found - no logs available*"
	}

	core := kubernetes.NewCore(client)
	var result strings.Builder
	result.WriteString("### virt-launcher Pod Logs\n\n")

	containerName := "compute"
	for _, podName := range podNames {
		result.WriteString(fmt.Sprintf("#### Pod: %s\n\n", podName))

		// Fetch last 50 lines of logs
		logs, err := core.PodsLog(ctx, namespace, podName, containerName, false, 50)
		if err != nil {
			return fmt.Sprintf("### virt-launcher Pod Logs\n\n*Error fetching logs: %v*", err)
		}

		result.WriteString(fmt.Sprintf("**Container: %s**\n\n```\n%s\n```\n\n", containerName, logs))
	}

	return result.String()
}

// fetchEvents fetches events related to the VM and returns them formatted
func fetchEvents(ctx context.Context, client api.KubernetesClient, namespace, vmName string) string {
	core := kubernetes.NewCore(client)
	eventMap, err := core.EventsList(ctx, namespace)
	if err != nil {
		return fmt.Sprintf("### Events\n\n*Error listing events: %v*", err)
	}

	if len(eventMap) == 0 {
		return "### Events\n\n*No events found in namespace*"
	}

	// Filter events related to the VM
	var relatedEvents []map[string]any
	for _, event := range eventMap {
		involvedObj, ok := event["InvolvedObject"].(map[string]string)
		if !ok {
			continue
		}
		objName := involvedObj["Name"]
		objKind := involvedObj["Kind"]

		// Include events for VM, VMI
		if objName == vmName && (objKind == "VirtualMachine" || objKind == "VirtualMachineInstance") {
			relatedEvents = append(relatedEvents, event)
			continue
		}

		// Include events for pods with VM name prefix
		if strings.HasPrefix(objName, vmName+"-") || strings.HasPrefix(objName, "virt-launcher-"+vmName) {
			relatedEvents = append(relatedEvents, event)
		}
	}

	if len(relatedEvents) == 0 {
		return "### Events\n\n*No events found related to this VM*"
	}

	yamlStr, err := output.MarshalYaml(relatedEvents)
	if err != nil {
		return fmt.Sprintf("### Events\n\n*Error marshaling events: %v*", err)
	}

	return fmt.Sprintf("### Events (related to %s)\n\n```yaml\n%s```", vmName, yamlStr)
}
