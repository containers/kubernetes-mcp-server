package kubevirt

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// RunStrategy represents the run strategy for a VirtualMachine
type RunStrategy string

const (
	RunStrategyAlways RunStrategy = "Always"
	RunStrategyHalted RunStrategy = "Halted"
)

var (
	// VirtualMachineGVK is the GroupVersionKind for VirtualMachine resources
	VirtualMachineGVK = schema.GroupVersionKind{
		Group:   "kubevirt.io",
		Version: "v1",
		Kind:    "VirtualMachine",
	}

	// VirtualMachineGVR is the GroupVersionResource for VirtualMachine resources
	VirtualMachineGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}
)

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

// StartVM starts a VirtualMachine by updating its runStrategy to Always
// Returns the updated VM and true if the VM was started, false if it was already running
func StartVM(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (*unstructured.Unstructured, bool, error) {
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
	if found && currentStrategy == RunStrategyAlways {
		return vm, false, nil
	}

	// Update runStrategy to Always
	if err := SetVMRunStrategy(vm, RunStrategyAlways); err != nil {
		return nil, false, fmt.Errorf("failed to set runStrategy: %w", err)
	}

	// Update the VM in the cluster
	updatedVM, err := UpdateVirtualMachine(ctx, dynamicClient, vm)
	if err != nil {
		return nil, false, fmt.Errorf("failed to start VirtualMachine: %w", err)
	}

	return updatedVM, true, nil
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

// RestartVM restarts a VirtualMachine by temporarily setting runStrategy to Halted then back to Always
func RestartVM(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
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

	// Start the VM again
	if err := SetVMRunStrategy(vm, RunStrategyAlways); err != nil {
		return nil, fmt.Errorf("failed to set runStrategy to Always: %w", err)
	}

	updatedVM, err := UpdateVirtualMachine(ctx, dynamicClient, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to start VirtualMachine: %w", err)
	}

	return updatedVM, nil
}
