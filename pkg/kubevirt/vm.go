package kubevirt

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// RunStrategy represents the Kubernetes VirtualMachine runStrategy.
type RunStrategy string

// RunPolicy is a user-facing abstraction over RunStrategy used by lifecycle tools.
type RunPolicy string

const (
	RunStrategyAlways         RunStrategy = "Always"
	RunStrategyHalted         RunStrategy = "Halted"
	RunStrategyManual         RunStrategy = "Manual"
	RunStrategyRerunOnFailure RunStrategy = "RerunOnFailure"
	RunStrategyOnce           RunStrategy = "Once"
)

const (
	RunPolicyHighAvailability RunPolicy = "HighAvailability"
	RunPolicyRestartOnFailure RunPolicy = "RestartOnFailure"
	RunPolicyOnce             RunPolicy = "Once"
)

// Validate reports whether the run policy is one of the supported values.
func (p RunPolicy) Validate() error {
	switch p {
	case RunPolicyHighAvailability, RunPolicyRestartOnFailure, RunPolicyOnce:
		return nil
	default:
		return fmt.Errorf("invalid run policy '%s': must be one of 'HighAvailability', 'RestartOnFailure', 'Once'", p)
	}
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
//
// Returns (vm, wasStarted, strategyChanged, err):
//   - wasStarted is true when the VM was not already under an auto-running strategy
//     (Halted, Manual, missing, or unknown) and is now scheduled to start.
//   - strategyChanged is true when the runStrategy was updated in place on a VM that was
//     already auto-running (Always, RerunOnFailure, or Once); the VM was not stopped.
//   - Both are false when the VM was already running with the desired run strategy (no-op).
func StartVM(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string, runPolicy RunPolicy) (*unstructured.Unstructured, bool, bool, error) {
	desiredStrategy, err := runStrategyFromRunPolicy(runPolicy)
	if err != nil {
		return nil, false, false, err
	}

	// Get the current VirtualMachine
	vm, err := GetVirtualMachine(ctx, dynamicClient, namespace, name)
	if err != nil {
		return nil, false, false, fmt.Errorf("failed to get VirtualMachine: %w", err)
	}

	currentStrategy, found, err := GetVMRunStrategy(vm)
	if err != nil {
		return nil, false, false, fmt.Errorf("failed to read runStrategy from VirtualMachine: %w", err)
	}

	// Already at the desired strategy — no-op
	if found && currentStrategy == desiredStrategy {
		return vm, false, false, nil
	}

	// Update runStrategy to the appropriate value
	if err := SetVMRunStrategy(vm, desiredStrategy); err != nil {
		return nil, false, false, fmt.Errorf("failed to set runStrategy: %w", err)
	}

	// Update the VM in the cluster
	updatedVM, err := UpdateVirtualMachine(ctx, dynamicClient, vm)
	if err != nil {
		return nil, false, false, fmt.Errorf("failed to start VirtualMachine: %w", err)
	}

	// In-place policy changes only apply when the VM was already auto-running.
	// Halted/Manual/missing strategies cause KubeVirt to (re)start a VMI.
	if found && isAutoRunningStrategy(currentStrategy) {
		return updatedVM, false, true, nil
	}
	return updatedVM, true, false, nil
}

// isAutoRunningStrategy reports whether strategy keeps or creates a VMI automatically.
func isAutoRunningStrategy(strategy RunStrategy) bool {
	switch strategy {
	case RunStrategyAlways, RunStrategyRerunOnFailure, RunStrategyOnce:
		return true
	default:
		return false
	}
}

// runStrategyFromRunPolicy maps a RunPolicy to its RunStrategy.
// - HighAvailability: Always
// - RestartOnFailure: RerunOnFailure
// - Once: Once
func runStrategyFromRunPolicy(runPolicy RunPolicy) (RunStrategy, error) {
	switch runPolicy {
	case RunPolicyHighAvailability:
		return RunStrategyAlways, nil
	case RunPolicyRestartOnFailure:
		return RunStrategyRerunOnFailure, nil
	case RunPolicyOnce:
		return RunStrategyOnce, nil
	default:
		// Validate rejects unknown policies. The second return exists to catch a
		// future bug where a policy is added to Validate without a mapping case above.
		if err := runPolicy.Validate(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("run policy %q has no strategy mapping", runPolicy)
	}
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
		return nil, false, fmt.Errorf("failed to read runStrategy from VirtualMachine: %w", err)
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

// newSubresourceClient creates a REST client for the KubeVirt subresources API group
func newSubresourceClient(restConfig *rest.Config) (rest.Interface, error) {
	cfg := rest.CopyConfig(restConfig)
	cfg.GroupVersion = &schema.GroupVersion{Group: "subresources.kubevirt.io", Version: "v1"}
	cfg.APIPath = "/apis"
	cfg.NegotiatedSerializer = subresourcesCodec.WithoutConversion()
	return rest.RESTClientFor(cfg)
}

// PauseVM pauses a running VirtualMachineInstance via the KubeVirt subresource API
// and returns the parent VirtualMachine
func PauseVM(ctx context.Context, dynamicClient dynamic.Interface, restConfig *rest.Config, namespace, name string) (*unstructured.Unstructured, error) {
	client, err := newSubresourceClient(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create subresource client: %w", err)
	}
	result := client.Put().
		Namespace(namespace).
		Resource("virtualmachineinstances").
		Name(name).
		SubResource("pause").
		Body([]byte("{}")).
		Do(ctx)
	if err := result.Error(); err != nil {
		return nil, fmt.Errorf("failed to pause VirtualMachineInstance: %w", err)
	}
	return GetVirtualMachine(ctx, dynamicClient, namespace, name)
}

// UnpauseVM unpauses a paused VirtualMachineInstance via the KubeVirt subresource API
// and returns the parent VirtualMachine
func UnpauseVM(ctx context.Context, dynamicClient dynamic.Interface, restConfig *rest.Config, namespace, name string) (*unstructured.Unstructured, error) {
	client, err := newSubresourceClient(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create subresource client: %w", err)
	}
	result := client.Put().
		Namespace(namespace).
		Resource("virtualmachineinstances").
		Name(name).
		SubResource("unpause").
		Body([]byte("{}")).
		Do(ctx)
	if err := result.Error(); err != nil {
		return nil, fmt.Errorf("failed to unpause VirtualMachineInstance: %w", err)
	}
	return GetVirtualMachine(ctx, dynamicClient, namespace, name)
}

// RestartVM restarts a VirtualMachine by temporarily setting runStrategy to Halted then back to the specified run policy
func RestartVM(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string, runPolicy RunPolicy) (*unstructured.Unstructured, error) {
	desiredStrategy, err := runStrategyFromRunPolicy(runPolicy)
	if err != nil {
		return nil, err
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
	if err := SetVMRunStrategy(vm, desiredStrategy); err != nil {
		return nil, fmt.Errorf("failed to set runStrategy: %w", err)
	}

	updatedVM, err := UpdateVirtualMachine(ctx, dynamicClient, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to start VirtualMachine: %w", err)
	}

	return updatedVM, nil
}
