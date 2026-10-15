package kubevirt

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
)

// initVMTroubleshoot initializes the VM troubleshooting prompt
func initVMTroubleshoot() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "vm-troubleshoot",
				Title:       fmt.Sprintf("%s VirtualMachine Troubleshoot", defaults.ProductName()),
				Description: fmt.Sprintf("Generate a step-by-step troubleshooting guide for diagnosing %s VirtualMachine issues", defaults.ProductName()),
				Arguments: []config.PromptArgument{
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
			RBAC: api.RBACBounded(
				api.RBACRequirement{
					Verbs:        []string{"get"},
					Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "kubevirt.io", Resource: "virtualmachines"}},
					Namespace:    &api.RBACNamespace{Argument: "namespace"},
					ResourceName: &api.RBACResourceName{Argument: "name"},
				},
				api.RBACRequirement{
					Verbs:        []string{"get"},
					Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "kubevirt.io", Resource: "virtualmachineinstances"}},
					Namespace:    &api.RBACNamespace{Argument: "namespace"},
					ResourceName: &api.RBACResourceName{Argument: "name"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "pods"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"get"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "pods", Subresource: "log"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "events"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
			),
			Handler: vmTroubleshootHandler,
		},
	}
}

const vmTroubleshootAssistantText = "I'll analyze the collected data to diagnose the VirtualMachine issues systematically."

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

	b := api.NewPromptBudget(ctx, vmTroubleshootAssistantText)
	writeVMTroubleshootGuide(ctx, params.KubernetesClient, params.DynamicClient(), namespace, name, b)
	return vmTroubleshootResult(b.String(), true), nil
}

// writeVMTroubleshootGuide writes the guide into one prompt budget. Each
// fetch waits until the previous section still fits. Pod YAML, pod logs, and
// events are written as they are seen; the next pod log or event list does
// not start once the budget is spent.
func writeVMTroubleshootGuide(ctx context.Context, client api.KubernetesClient, dynamicClient dynamic.Interface, namespace, name string, b *api.BuilderWithBudget) {
	product := defaults.ProductName()
	var vm, vmi *unstructured.Unstructured
	var podNames []string
	_ = b.Sections(
		api.Text(fmt.Sprintf(`# %s VirtualMachine Troubleshooting Guide

## VM: %s (namespace: %s)

Use this guide to diagnose issues with the %s VirtualMachine. The relevant resource data has been collected below.

---

## Step 1: VirtualMachine Status

Check the VirtualMachine status:
- printableStatus (should be "Running")
- ready (should be true)
- conditions for any errors

`, product, name, namespace, product)),
		func(b *api.BuilderWithBudget) error {
			vmYaml, obj := fetchVirtualMachineStatus(ctx, dynamicClient, namespace, name)
			vm = obj
			b.WriteString(vmYaml)
			return nil
		},
		api.Text(`

---

## Step 2: VirtualMachineInstance Status

Check the VirtualMachineInstance status:
- phase (should be "Running")
- conditions for "Ready" condition

`),
		func(b *api.BuilderWithBudget) error {
			vmiYaml, obj := fetchVirtualMachineInstanceStatus(ctx, dynamicClient, namespace, name)
			vmi = obj
			b.WriteString(vmiYaml)
			return nil
		},
		api.Text(`

---

## Step 3: VM Volumes

Review the VM volumes for any configuration issues:

`),
		func(b *api.BuilderWithBudget) error {
			b.WriteString(fetchVirtualMachineVolumes(namespace, name, vm, vmi))
			return nil
		},
		api.Text(`

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

`),
		func(b *api.BuilderWithBudget) error {
			podNames = fetchVirtLauncherPod(ctx, dynamicClient, namespace, name, b)
			return nil
		},
		api.Text(`

---

## Step 5: virt-launcher Pod Logs

Review the logs from the virt-launcher pod containers:
- Look for error messages or stack traces
- Check for VM boot issues or QEMU errors

`),
		func(b *api.BuilderWithBudget) error {
			fetchVirtLauncherPodLogs(ctx, client, namespace, podNames, b)
			return nil
		},
		api.Text(`

---

## Step 6: Events

Review events related to the VM and its components:
- Look for Warning events
- Check for scheduling, storage, or network issues

`),
		func(b *api.BuilderWithBudget) error {
			fetchEvents(ctx, client, namespace, name, b)
			return nil
		},
		api.Text(`

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
`),
	)
}

func vmTroubleshootResult(guide string, includeAssistant bool) *api.PromptCallResult {
	messages := []api.PromptMessage{{
		Role: "user",
		Content: api.PromptContent{
			Type: "text",
			Text: guide,
		},
	}}
	if includeAssistant {
		messages = append(messages, api.PromptMessage{
			Role: "assistant",
			Content: api.PromptContent{
				Type: "text",
				Text: vmTroubleshootAssistantText,
			},
		})
	}
	return api.NewPromptCallResult("VirtualMachine troubleshooting guide generated", messages, nil)
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

// fetchVirtLauncherPod lists virt-launcher pods once and writes each pod into
// b. Further pods are not formatted once b is spent. The returned names are
// the pods that were written, for the log section.
func fetchVirtLauncherPod(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string, b *api.BuilderWithBudget) []string {
	if b == nil {
		return nil
	}
	labelSelector := fmt.Sprintf("kubevirt.io=virt-launcher,vm.kubevirt.io/name=%s", name)
	podList, err := dynamicClient.Resource(kubevirt.PodGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		b.WriteString(fmt.Sprintf("### virt-launcher Pod\n\n*Error listing pods: %v*", err))
		return nil
	}

	if len(podList.Items) == 0 {
		b.WriteString("### virt-launcher Pod\n\n*No virt-launcher pod found*\n\n(This is expected if the VM is stopped or the VMI hasn't been scheduled yet)")
		return nil
	}

	var podNames []string
	steps := []api.Step{api.Text("### virt-launcher Pod\n\n")}
	for _, pod := range podList.Items {
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			podNames = append(podNames, pod.GetName())
			yamlStr, err := output.MarshalYaml(&pod)
			if err != nil {
				b.WriteString(fmt.Sprintf("*Error marshaling pod %s: %v*\n\n", pod.GetName(), err))
				return nil
			}
			b.WriteString(fmt.Sprintf("#### %s\n\n```yaml\n%s```\n\n", pod.GetName(), yamlStr))
			return nil
		})
	}
	_ = b.Sections(steps...)
	return podNames
}

// fetchVirtLauncherPodLogs writes each pod's compute log into b. The read is
// limited to the bytes b can still accept. The next pod is not requested once
// b is spent.
func fetchVirtLauncherPodLogs(ctx context.Context, client api.KubernetesClient, namespace string, podNames []string, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	if len(podNames) == 0 {
		b.WriteString("### virt-launcher Pod Logs\n\n*No virt-launcher pod found - no logs available*")
		return
	}

	core := kubernetes.NewCore(client)
	const containerName = "compute"
	steps := []api.Step{api.Text("### virt-launcher Pod Logs\n\n")}
	for _, podName := range podNames {
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			logs, err := core.PodsLogLimited(ctx, namespace, podName, containerName, false, 50, b.Remaining())
			if err != nil {
				b.WriteString(fmt.Sprintf("#### Pod: %s\n\n*Error fetching logs: %v*\n\n", podName, err))
				return nil
			}
			b.WriteString(fmt.Sprintf("#### Pod: %s\n\n**Container: %s**\n\n```\n", podName, containerName))
			b.WriteString(logs)
			if !b.Truncated() {
				b.WriteString("\n```\n\n")
			}
			return nil
		})
	}
	_ = b.Sections(steps...)
}

// fetchEvents writes events related to the VM into b. A list error is recorded
// and the other list still runs when the budget is not spent, so events
// already written stay in the section. The second list does not start once b
// is spent.
func fetchEvents(ctx context.Context, client api.KubernetesClient, namespace, vmName string, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	core := kubernetes.NewCore(client)
	var problems []string
	opened := false

	writeMatches := func(events []map[string]any, keep func(map[string]string) bool) {
		steps := make([]api.Step, 0, len(events))
		for _, event := range events {
			involved, ok := event["InvolvedObject"].(map[string]string)
			if !ok || !keep(involved) {
				continue
			}
			steps = append(steps, func(b *api.BuilderWithBudget) error {
				if !opened {
					b.WriteString(fmt.Sprintf("### Events (related to %s)\n\n```yaml\n", vmName))
					opened = true
				}
				return b.Sections(func(b *api.BuilderWithBudget) error {
					item, err := yamlListItem(event)
					if err != nil {
						b.WriteString(fmt.Sprintf("*Error marshaling events: %v*", err))
						return err
					}
					b.WriteString(item)
					return nil
				})
			})
		}
		// A marshal error stops the rest of this list. The other list still
		// runs when the budget is not spent.
		_ = b.Sections(steps...)
	}

	_ = b.Sections(
		func(b *api.BuilderWithBudget) error {
			// Server-side filter: events whose involvedObject.name exactly matches the VM name.
			vmEvents, err := core.EventsList(ctx, namespace, api.ListOptions{
				ListOptions: metav1.ListOptions{FieldSelector: "involvedObject.name=" + vmName},
			})
			if err != nil {
				problems = append(problems, fmt.Sprintf("*Error listing events: %v*", err))
				return nil
			}
			writeMatches(vmEvents, func(obj map[string]string) bool {
				kind := obj["Kind"]
				return kind == "VirtualMachine" || kind == "VirtualMachineInstance"
			})
			return nil
		},
		func(b *api.BuilderWithBudget) error {
			// Prefix matches (virt-launcher pods, vmName-suffixed objects) are not a
			// field selector. List the namespace only when the budget still has room.
			allEvents, err := core.EventsList(ctx, namespace, api.ListOptions{})
			if err != nil {
				problems = append(problems, fmt.Sprintf("*Error listing events: %v*", err))
				return nil
			}
			writeMatches(allEvents, func(obj map[string]string) bool {
				objName := obj["Name"]
				return strings.HasPrefix(objName, vmName+"-") || strings.HasPrefix(objName, "virt-launcher-"+vmName)
			})
			return nil
		},
		func(b *api.BuilderWithBudget) error {
			if !opened {
				b.WriteString("### Events\n\n")
				if len(problems) == 0 {
					b.WriteString("*No events found related to this VM*")
					return nil
				}
				b.WriteString(problems[0])
				problems = problems[1:]
				return nil
			}
			b.WriteString("```")
			return nil
		},
		func(b *api.BuilderWithBudget) error {
			steps := make([]api.Step, 0, len(problems))
			for _, problem := range problems {
				steps = append(steps, api.Text("\n\n"+problem))
			}
			return b.Sections(steps...)
		},
	)
}
