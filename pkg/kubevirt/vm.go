package kubevirt

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// RunStrategy represents the run strategy for a VirtualMachine
type RunStrategy string
type RunPolicy string

const (
	RunStrategyAlways         RunStrategy = "Always"
	RunStrategyHalted         RunStrategy = "Halted"
	RunStrategyManual         RunStrategy = "Manual"
	RunStrategyRerunOnFailure RunStrategy = "RerunOnFailure"
	RunStrategyOnce           RunStrategy = "Once"
	RunStrategyWaitAsReceiver RunStrategy = "WaitAsReceiver"

	RunPolicyHighAvailability RunPolicy = "HighAvailability"
	RunPolicyRestartOnFailure RunPolicy = "RestartOnFailure"
	RunPolicyOnce             RunPolicy = "Once"
)

func IsValidRunPolicy(runPolicy RunPolicy) bool {
	switch runPolicy {
	case RunPolicyHighAvailability, RunPolicyRestartOnFailure, RunPolicyOnce:
		return true
	}
	return false
}

// GetVirtualMachine retrieves a VirtualMachine by namespace and name
func GetVirtualMachine(ctx context.Context, client dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	return client.Resource(VirtualMachineGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

// GetVMRunStrategy retrieves the current runStrategy from a VirtualMachine
// Returns the strategy, whether it was found, and any error
func GetVMRunStrategy(vm *unstructured.Unstructured) (RunStrategy, bool, error) {
	strategy, found, err := unstructured.NestedString(vm.Object, "spec", "runStrategy")
	if err != nil {
		return "", false, fmt.Errorf("failed to read runStrategy: %w", err)
	}

	return RunStrategy(strategy), found, nil
}

// SetVMRunStrategy sets the runStrategy on a VirtualMachine
func SetVMRunStrategy(vm *unstructured.Unstructured, strategy RunStrategy) error {
	return unstructured.SetNestedField(vm.Object, string(strategy), "spec", "runStrategy")
}

// UpdateVirtualMachine updates a VirtualMachine in the cluster
func UpdateVirtualMachine(ctx context.Context, client dynamic.Interface, vm *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return client.Resource(VirtualMachineGVR).
		Namespace(vm.GetNamespace()).
		Update(ctx, vm, metav1.UpdateOptions{})
}

// StartVM starts a VirtualMachine by updating its runStrategy based on the runPolicy
// runPolicy can be one of: HighAvailability, RestartOnFailure, Once
// - HighAvailability: The VM will be started if it is not already running, if it is already running the runStrategy
// will be set to Always.
// - RestartOnFailure: The VM will be started if it is not already running and will be restarted if it fails, if it
// is already running the runStrategy will be set to RerunOnFailure.
// - Once: The VM will be started if it is not already running and will be stopped after it completes, if it is already
// running the runStrategy will be set to Once.
// Returns the updated VM and true if the VM was started, or if it was already running and the runStrategy changed.
// Returns false if it was already running and the runStrategy did not change.
func StartVM(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string, runPolicy RunPolicy) (*unstructured.Unstructured, bool, error) {
	// Validate run policy
	if !IsValidRunPolicy(runPolicy) {
		return nil, false, fmt.Errorf("invalid run policy '%s': must be one of 'HighAvailability', 'RestartOnFailure', 'Once'", runPolicy)
	}

	// Get the current VirtualMachine
	vm, err := GetVirtualMachine(ctx, dynamicClient, namespace, name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get VirtualMachine: %w", err)
	}

	currentStrategy, found, err := GetVMRunStrategy(vm)
	if err != nil {
		return nil, false, err
	}

	// Check if already running
	if found && currentStrategy == getRunStrategyFromRunPolicy(runPolicy) {
		return vm, false, nil
	}

	// Update runStrategy to the appropriate value
	if err := SetVMRunStrategy(vm, getRunStrategyFromRunPolicy(runPolicy)); err != nil {
		return nil, false, fmt.Errorf("failed to set runStrategy: %w", err)
	}

	// Update the VM in the cluster
	updatedVM, err := UpdateVirtualMachine(ctx, dynamicClient, vm)
	if err != nil {
		return nil, false, fmt.Errorf("failed to start VirtualMachine: %w", err)
	}

	return updatedVM, true, nil
}

// getRunStrategyFromRunPolicy returns the RunStrategy for a given RunPolicy
// - HighAvailability: Always
// - RestartOnFailure: RerunOnFailure
// - Once: Once
// Returns the RunStrategy
func getRunStrategyFromRunPolicy(runPolicy RunPolicy) RunStrategy {
	switch runPolicy {
	case RunPolicyHighAvailability:
		return RunStrategyAlways
	case RunPolicyRestartOnFailure:
		return RunStrategyRerunOnFailure
	case RunPolicyOnce:
		return RunStrategyOnce
	}
	return RunStrategyAlways
}

// StopVM stops a VirtualMachine by updating its runStrategy to Halted
// Returns the updated VM and true if the VM was stopped, false if it was already stopped
func StopVM(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (*unstructured.Unstructured, bool, error) {
	// Get the current VirtualMachine
	vm, err := GetVirtualMachine(ctx, dynamicClient, namespace, name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get VirtualMachine: %w", err)
	}

	currentStrategy, found, err := GetVMRunStrategy(vm)
	if err != nil {
		return nil, false, err
	}

	// Check if already stopped
	if found && currentStrategy == RunStrategyHalted {
		return vm, false, nil
	}

	// Update runStrategy to Halted
	if err := SetVMRunStrategy(vm, RunStrategyHalted); err != nil {
		return nil, false, fmt.Errorf("failed to set runStrategy: %w", err)
	}

	// Update the VM in the cluster
	updatedVM, err := UpdateVirtualMachine(ctx, dynamicClient, vm)
	if err != nil {
		return nil, false, fmt.Errorf("failed to stop VirtualMachine: %w", err)
	}

	return updatedVM, true, nil
}

// CloneVM creates a VirtualMachineClone CR to clone a source VM to a target VM
func CloneVM(ctx context.Context, dynamicClient dynamic.Interface, namespace, sourceName, targetName string) (*unstructured.Unstructured, error) {
	clone := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "clone.kubevirt.io/v1beta1",
			"kind":       "VirtualMachineClone",
			"metadata": map[string]any{
				"namespace":    namespace,
				"generateName": sourceName + "-clone-",
			},
			"spec": map[string]any{
				"source": map[string]any{
					"apiGroup": "kubevirt.io",
					"kind":     "VirtualMachine",
					"name":     sourceName,
				},
				"target": map[string]any{
					"apiGroup": "kubevirt.io",
					"kind":     "VirtualMachine",
					"name":     targetName,
				},
			},
		},
	}

	result, err := dynamicClient.Resource(VirtualMachineCloneGVR).Namespace(namespace).Create(ctx, clone, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create VirtualMachineClone: %w", err)
	}

	return result, nil
}

// RestartVM restarts a VirtualMachine by temporarily setting runStrategy to Halted then back to the specified run policy
func RestartVM(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string, runPolicy RunPolicy) (*unstructured.Unstructured, error) {
	// Validate run policy
	if !IsValidRunPolicy(runPolicy) {
		return nil, fmt.Errorf("invalid run policy '%s': must be one of 'HighAvailability', 'RestartOnFailure', 'Once'", runPolicy)
	}

	// Get the current VirtualMachine
	vm, err := GetVirtualMachine(ctx, dynamicClient, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine: %w", err)
	}

	// Stop the VM first
	if err := SetVMRunStrategy(vm, RunStrategyHalted); err != nil {
		return nil, fmt.Errorf("failed to set runStrategy to Halted: %w", err)
	}

	vm, err = UpdateVirtualMachine(ctx, dynamicClient, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to stop VirtualMachine: %w", err)
	}

	// Start the VM again with the specified run policy
	if err := SetVMRunStrategy(vm, getRunStrategyFromRunPolicy(runPolicy)); err != nil {
		return nil, fmt.Errorf("failed to set runStrategy: %w", err)
	}

	updatedVM, err := UpdateVirtualMachine(ctx, dynamicClient, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to start VirtualMachine: %w", err)
	}

	return updatedVM, nil
}
