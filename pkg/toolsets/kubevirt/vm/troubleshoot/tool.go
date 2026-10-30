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

func Tools(inspector api.ClusterInspector) []api.ServerTool {
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
					Verbs:     []string{"get"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "cdi.kubevirt.io", Resource: "datavolumes"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
				api.RBACRequirement{
					Verbs:     []string{"get"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "persistentvolumeclaims"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
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
				api.RBACRequirement{
					Verbs:  []string{"get", "list"},
					Target: api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "storage.k8s.io", Resource: "storageclasses"}},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "kubevirt.io", Resource: "virtualmachineinstancemigrations"}},
					Namespace: &api.RBACNamespace{Argument: "namespace"},
				},
			),
			TargetCompatibilityFilters: []api.TargetCompatibilityFilter{
				kubevirt.HasVirtualMachine(inspector),
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

	b := api.NewBuilderWithBudget(api.ResultLimit(params.Context))
	writeVMTroubleshootReport(params.Context, params.KubernetesClient, params.DynamicClient(), namespace, name, b)
	return api.NewToolCallResult(b.String(), nil), nil
}

// writeVMTroubleshootReport writes the report into one result budget. Each
// section is written as it is produced. The next pod is not formatted, and
// the next DataVolume is not fetched, once that budget is spent.
func writeVMTroubleshootReport(ctx context.Context, client api.KubernetesClient, dynamicClient dynamic.Interface, namespace, name string, b *api.BuilderWithBudget) {
	var vmYaml, vmiYaml string
	var vm, vmi *unstructured.Unstructured
	var podNames []string
	var podDiags []map[string]interface{}
	var podSection string
	var loggedPods []string
	_ = b.Sections(
		api.Text(fmt.Sprintf("# VirtualMachine Diagnostic Report: %s/%s\n\n", namespace, name)),
		func(b *api.BuilderWithBudget) error {
			// Why aren't these interim calls done under the budgeter?
			// Because they accumulate data that isn't necessarily going to end up in the result,
			// and we won't know that until `analyzeIssues()` runs; so if we tried to count them
			// against the budget, we might fail even though the actual result would be under.
			// Each backend call is still bounded by `max_backend_response_bytes`, and the extras
			// are small and bounded (mostly static strings).
			vmYaml, vm = fetchVMStatus(ctx, dynamicClient, namespace, name)
			vmiYaml, vmi = fetchVMIStatus(ctx, dynamicClient, namespace, name)
			podNames, podDiags, podSection = listVirtLauncherPods(ctx, dynamicClient, namespace, name)
			b.WriteString(analyzeIssues(ctx, dynamicClient, namespace, name, vm, vmi, podDiags))
			return nil
		},
		api.Text("\n\n"),
		func(b *api.BuilderWithBudget) error {
			b.WriteString(vmYaml)
			return nil
		},
		api.Text("\n\n"),
		func(b *api.BuilderWithBudget) error {
			b.WriteString(vmiYaml)
			return nil
		},
		api.Text("\n\n"),
		func(b *api.BuilderWithBudget) error {
			b.WriteString(fetchVolumes(namespace, name, vm, vmi))
			return nil
		},
		api.Text("\n\n"),
		func(b *api.BuilderWithBudget) error {
			fetchDataVolumeStatus(ctx, dynamicClient, namespace, vm, b)
			return nil
		},
		api.Text("\n\n"),
		func(b *api.BuilderWithBudget) error {
			writeCloudInit(vm, vmi, b)
			return nil
		},
		api.Text("\n\n"),
		func(b *api.BuilderWithBudget) error {
			loggedPods = writeVirtLauncherPods(b, podNames, podDiags, podSection)
			return nil
		},
		api.Text("\n\n"),
		func(b *api.BuilderWithBudget) error {
			fetchVirtLauncherPodLogs(ctx, client, namespace, loggedPods, b)
			return nil
		},
		api.Text("\n\n"),
		func(b *api.BuilderWithBudget) error {
			fetchEvents(ctx, client, namespace, name, loggedPods, b)
			return nil
		},
	)
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

// listVirtLauncherPods lists virt-launcher pods once. section is the pod
// heading when the list fails or is empty. Diagnostics stay in memory for
// the issues section; their YAML is written later, one pod at a time.
func listVirtLauncherPods(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (names []string, diags []map[string]interface{}, section string) {
	labelSelector := fmt.Sprintf("kubevirt.io=virt-launcher,vm.kubevirt.io/name=%s", name)
	podList, err := dynamicClient.Resource(kubevirt.PodGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, nil, fmt.Sprintf("## virt-launcher Pod\n\n*Error listing pods: %v*", err)
	}
	if len(podList.Items) == 0 {
		return nil, nil, "## virt-launcher Pod\n\n*No virt-launcher pod found (VM may be stopped or not yet scheduled)*"
	}
	for _, pod := range podList.Items {
		names = append(names, pod.GetName())
		diags = append(diags, extractPodDiagnostics(&pod))
	}
	return names, diags, ""
}

// writeVirtLauncherPods writes each pod diagnostic into b and returns the
// names that were written. Further pods are not formatted once b is spent.
func writeVirtLauncherPods(b *api.BuilderWithBudget, names []string, diags []map[string]interface{}, section string) []string {
	if b == nil {
		return nil
	}
	if section != "" {
		b.WriteString(section)
		return nil
	}
	var wrote []string
	steps := []api.Step{api.Text("## virt-launcher Pod\n\n")}
	for i, name := range names {
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			wrote = append(wrote, name)
			yamlStr, err := output.MarshalYaml(diags[i])
			if err != nil {
				b.WriteString(fmt.Sprintf("*Error marshaling pod %s: %v*\n\n", name, err))
				return nil
			}
			b.WriteString(fmt.Sprintf("### %s\n\n```yaml\n%s```\n\n", name, yamlStr))
			return nil
		})
	}
	_ = b.Sections(steps...)
	return wrote
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

func fetchVirtLauncherPodLogs(ctx context.Context, client api.KubernetesClient, namespace string, podNames []string, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	if len(podNames) == 0 {
		b.WriteString("## virt-launcher Pod Logs\n\n*No pod found — no logs available*")
		return
	}

	core := kubernetes.NewCore(client)
	const containerName = "compute"
	steps := []api.Step{api.Text("## virt-launcher Pod Logs\n\n")}
	for _, podName := range podNames {
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			logs, err := core.PodsLogLimited(ctx, namespace, podName, containerName, false, 50, b.Remaining())
			if err != nil {
				b.WriteString(fmt.Sprintf("### %s\n\n*Error fetching logs: %v*\n\n", podName, err))
				return nil
			}
			b.WriteString(fmt.Sprintf("### %s (container: %s)\n\n```\n", podName, containerName))
			writeRedactedLines(b, logs)
			if !b.Truncated() {
				b.WriteString("\n```\n\n")
			}
			return nil
		})
	}
	_ = b.Sections(steps...)
}

// fetchEvents lists warning-related events for the VM and its pods. Each list
// is written as it returns, and the next List does not start once b is
// truncated. See docs/specs/memory-bounds.md#retention.
func fetchEvents(ctx context.Context, client api.KubernetesClient, namespace, vmName string, podNames []string, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	core := kubernetes.NewCore(client)

	objectNames := []string{vmName}
	objectNames = append(objectNames, podNames...)

	wrote := false
	steps := make([]api.Step, 0, len(objectNames)+1)
	for _, objName := range objectNames {
		if objName == "" {
			continue
		}
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			events, _, err := core.EventsList(ctx, namespace, api.ListOptions{
				ListOptions: metav1.ListOptions{FieldSelector: "involvedObject.name=" + objName},
			})
			if err != nil {
				if !wrote {
					b.WriteString("## Events\n\n")
					wrote = true
				}
				b.WriteString(fmt.Sprintf("*Error fetching events for %s: %v*\n", objName, err))
				return nil
			}
			if len(events) == 0 {
				return nil
			}
			yamlStr, err := output.MarshalYaml(events)
			if err != nil {
				if !wrote {
					b.WriteString("## Events\n\n")
					wrote = true
				}
				b.WriteString(fmt.Sprintf("*Error marshaling events for %s: %v*\n", objName, err))
				return nil
			}
			if !wrote {
				b.WriteString("## Events\n\n```yaml\n")
				wrote = true
			}
			return b.Sections(func(b *api.BuilderWithBudget) error {
				b.WriteString(yamlStr)
				b.WriteString("\n")
				return nil
			})
		})
	}
	steps = append(steps, func(b *api.BuilderWithBudget) error {
		if !wrote {
			b.WriteString("## Events\n\n*No events found related to this VM*")
			return nil
		}
		b.WriteString("```")
		return nil
	})
	_ = b.Sections(steps...)
}

// fetchDataVolumeStatus writes one DataVolume or PVC at a time and does not
// start the next Get once b is truncated.
// See docs/specs/memory-bounds.md#retention.
func fetchDataVolumeStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace string, vm *unstructured.Unstructured, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	if vm == nil {
		b.WriteString("## DataVolume/PVC Status\n\n*No VM available to extract DataVolume references*")
		return
	}

	dvTemplates, found, err := unstructured.NestedSlice(vm.Object, "spec", "dataVolumeTemplates")
	if err != nil || !found || len(dvTemplates) == 0 {
		b.WriteString("## DataVolume/PVC Status\n\n*No dataVolumeTemplates defined in VM spec*")
		return
	}

	steps := []api.Step{api.Text("## DataVolume/PVC Status\n\n")}
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
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			dv, err := dynamicClient.Resource(kubevirt.DataVolumeGVR).Namespace(namespace).Get(ctx, dvName, metav1.GetOptions{})
			if err != nil {
				if !apierrors.IsNotFound(err) {
					b.WriteString(fmt.Sprintf("### %s\n\n*Error fetching DataVolume: %v*\n\n", dvName, err))
					return nil
				}
				b.WriteString(fmt.Sprintf("### %s\n\n*DataVolume not found*\n\n", dvName))
				return b.Sections(func(b *api.BuilderWithBudget) error {
					pvc, pvcErr := dynamicClient.Resource(kubevirt.PersistentVolumeClaimGVR).Namespace(namespace).Get(ctx, dvName, metav1.GetOptions{})
					if pvcErr != nil {
						b.WriteString(fmt.Sprintf("*PVC also not found: %v*\n\n", pvcErr))
						return nil
					}
					pvcStatus, _, _ := unstructured.NestedMap(pvc.Object, "status")
					if pvcStatus == nil {
						return nil
					}
					yamlStr, marshalErr := output.MarshalYaml(pvcStatus)
					if marshalErr != nil {
						b.WriteString(fmt.Sprintf("*Error marshaling PVC status: %v*\n\n", marshalErr))
						return nil
					}
					b.WriteString(fmt.Sprintf("PVC exists with status:\n```yaml\n%s```\n\n", yamlStr))
					return nil
				})
			}

			dvStatus, found, _ := unstructured.NestedMap(dv.Object, "status")
			if !found {
				b.WriteString(fmt.Sprintf("### %s\n\n*DataVolume exists but has no status*\n\n", dvName))
				return nil
			}

			yamlStr, err := output.MarshalYaml(dvStatus)
			if err != nil {
				b.WriteString(fmt.Sprintf("### %s\n\n*Error marshaling DataVolume status: %v*\n\n", dvName, err))
				return nil
			}

			dvSpec, _, _ := unstructured.NestedMap(dv.Object, "spec")
			storageClass := ""
			if dvSpec != nil {
				sc, _, _ := unstructured.NestedString(dvSpec, "storage", "storageClassName")
				if sc != "" {
					storageClass = sc
				}
			}

			line := "### " + dvName
			if storageClass != "" {
				line += fmt.Sprintf(" (storageClass: %s)", storageClass)
			}
			b.WriteString(fmt.Sprintf("%s\n\n```yaml\n%s```\n\n", line, yamlStr))
			return nil
		})
	}
	_ = b.Sections(steps...)
}

func writeCloudInit(vm, vmi *unstructured.Unstructured, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	var volumes []interface{}
	if vm != nil {
		volumes, _, _ = unstructured.NestedSlice(vm.Object, "spec", "template", "spec", "volumes")
	}
	if len(volumes) == 0 && vmi != nil {
		volumes, _, _ = unstructured.NestedSlice(vmi.Object, "spec", "volumes")
	}
	if len(volumes) == 0 {
		b.WriteString("## Cloud-Init Configuration\n\n*No volumes found*")
		return
	}

	var steps []api.Step
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
			steps = append(steps, func(b *api.BuilderWithBudget) error {
				if !found {
					b.WriteString("## Cloud-Init Configuration\n\n")
					found = true
				}
				volName, _ := volMap["name"].(string)
				b.WriteString(fmt.Sprintf("### %s (type: %s)\n\n", volName, ciKey))
				userData := cloudInitData(ciData, "userData", "userDataBase64")
				if userData != "" {
					b.WriteString(fmt.Sprintf("```yaml\n%s\n```\n\n", redactCloudInitSensitiveFields(userData)))
				}
				networkData := cloudInitData(ciData, "networkData", "networkDataBase64")
				if networkData != "" {
					b.WriteString(fmt.Sprintf("**networkData:**\n```yaml\n%s\n```\n\n", redactCloudInitSensitiveFields(networkData)))
				}
				if userData == "" && networkData == "" {
					secretRef, _ := ciData["userDataSecretRef"].(map[string]interface{})
					if secretRef != nil {
						b.WriteString(fmt.Sprintf("*userData from Secret: %v*\n\n", secretRef["name"]))
					}
				}
				return nil
			})
		}
	}
	if len(steps) == 0 {
		steps = append(steps, api.Text("## Cloud-Init Configuration\n\n*No cloud-init volumes configured*"))
	}
	_ = b.Sections(steps...)
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

func redactLogLine(line string) string {
	lower := strings.ToLower(line)
	for _, pat := range logSecretPatterns {
		if idx := strings.Index(lower, pat); idx >= 0 {
			return line[:idx+len(pat)] + "<REDACTED>"
		}
	}
	return line
}

func writeRedactedLines(b *api.BuilderWithBudget, logs string) {
	rest := logs
	for rest != "" {
		line, next, found := strings.Cut(rest, "\n")
		b.WriteString(redactLogLine(line))
		if !found || b.Truncated() {
			return
		}
		b.WriteString("\n")
		rest = next
	}
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
