package kubevirt

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
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

	// Build the troubleshooting guide message
	guideText := fmt.Sprintf(`# VirtualMachine Troubleshooting Guide

## VM: %s (namespace: %s)

Follow these steps to diagnose issues with the VirtualMachine:

## Step 1: Check VirtualMachine Status
Use resources_get with apiVersion=kubevirt.io/v1, kind=VirtualMachine, namespace=%s, name=%s

Look for: status.printableStatus (should be "Running"), status.ready (should be true), status.conditions for errors

## Step 2: Check VirtualMachineInstance
Use resources_get with apiVersion=kubevirt.io/v1, kind=VirtualMachineInstance, namespace=%s, name=%s

Look for: status.phase (should be "Running"), status.conditions for "Ready" condition

## Step 3: Check DataVolumes (if used)
Use resources_list with apiVersion=cdi.kubevirt.io/v1beta1, kind=DataVolume, namespace=%s

Look for DataVolumes with names starting with "%s-". Check status.phase (should be "Succeeded")

## Step 4: Check PersistentVolumeClaims
Use resources_list with apiVersion=v1, kind=PersistentVolumeClaim, namespace=%s

Check status.phase (should be "Bound")

## Step 5: Check virt-launcher Pod
Use pods_list_in_namespace with namespace=%s, labelSelector=kubevirt.io=virt-launcher,vm.kubevirt.io/name=%s

Pod should be "Running" with all containers ready. Get logs with pods_log if issues found.

## Step 6: Check Events
Use events_list with namespace=%s

Filter for events related to "%s" - look for warnings or errors.

## Report Findings

After completing troubleshooting, report:
- **Status:** Running/Stopped/Failed/Provisioning
- **Root Cause:** Description or "None found"
- **Recommended Action:** What the user should do
`, name, namespace, namespace, name, namespace, name, namespace, name, namespace, namespace, name, namespace, name)

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
					Text: "I'll follow this troubleshooting guide to diagnose the VirtualMachine issues systematically.",
				},
			},
		},
		nil,
	), nil
}
