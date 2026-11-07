package kubevirt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

// createTestVM creates a test VirtualMachine with the given name, namespace, and runStrategy
func createTestVM(name, namespace string, runStrategy RunStrategy) *unstructured.Unstructured {
	vm := &unstructured.Unstructured{}
	vm.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"runStrategy": string(runStrategy),
		},
	})
	return vm
}

func TestStartVM(t *testing.T) {
	tests := []struct {
		name               string
		runPolicy          RunPolicy
		initialVM          *unstructured.Unstructured
		wantStarted        bool
		wantStrategyChange bool
		wantRunStrategy    RunStrategy
		wantError          bool
		errorContains      string
	}{
		// HighAvailability policy tests
		{
			name:               "HighAvailability: Start halted VM",
			runPolicy:          RunPolicyHighAvailability,
			initialVM:          createTestVM("test-vm", "default", RunStrategyHalted),
			wantStarted:        true,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyAlways,
		},
		{
			name:               "HighAvailability: VM already running with Always",
			runPolicy:          RunPolicyHighAvailability,
			initialVM:          createTestVM("test-vm", "default", RunStrategyAlways),
			wantStarted:        false,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyAlways,
		},
		{
			name:               "HighAvailability: Change from RerunOnFailure to Always",
			runPolicy:          RunPolicyHighAvailability,
			initialVM:          createTestVM("test-vm", "default", RunStrategyRerunOnFailure),
			wantStarted:        false,
			wantStrategyChange: true,
			wantRunStrategy:    RunStrategyAlways,
		},
		{
			name:               "HighAvailability: Change from Once to Always",
			runPolicy:          RunPolicyHighAvailability,
			initialVM:          createTestVM("test-vm", "default", RunStrategyOnce),
			wantStarted:        false,
			wantStrategyChange: true,
			wantRunStrategy:    RunStrategyAlways,
		},
		{
			name:               "HighAvailability: Change from Manual to Always",
			runPolicy:          RunPolicyHighAvailability,
			initialVM:          createTestVM("test-vm", "default", RunStrategyManual),
			wantStarted:        true,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyAlways,
		},

		// RestartOnFailure policy tests
		{
			name:               "RestartOnFailure: Start halted VM",
			runPolicy:          RunPolicyRestartOnFailure,
			initialVM:          createTestVM("test-vm", "default", RunStrategyHalted),
			wantStarted:        true,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyRerunOnFailure,
		},
		{
			name:               "RestartOnFailure: VM already running with RerunOnFailure",
			runPolicy:          RunPolicyRestartOnFailure,
			initialVM:          createTestVM("test-vm", "default", RunStrategyRerunOnFailure),
			wantStarted:        false,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyRerunOnFailure,
		},
		{
			name:               "RestartOnFailure: Change from Always to RerunOnFailure",
			runPolicy:          RunPolicyRestartOnFailure,
			initialVM:          createTestVM("test-vm", "default", RunStrategyAlways),
			wantStarted:        false,
			wantStrategyChange: true,
			wantRunStrategy:    RunStrategyRerunOnFailure,
		},
		{
			name:               "RestartOnFailure: Change from Once to RerunOnFailure",
			runPolicy:          RunPolicyRestartOnFailure,
			initialVM:          createTestVM("test-vm", "default", RunStrategyOnce),
			wantStarted:        false,
			wantStrategyChange: true,
			wantRunStrategy:    RunStrategyRerunOnFailure,
		},
		{
			name:               "RestartOnFailure: Change from Manual to RerunOnFailure",
			runPolicy:          RunPolicyRestartOnFailure,
			initialVM:          createTestVM("test-vm", "default", RunStrategyManual),
			wantStarted:        true,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyRerunOnFailure,
		},

		// Once policy tests
		{
			name:               "Once: Start halted VM",
			runPolicy:          RunPolicyOnce,
			initialVM:          createTestVM("test-vm", "default", RunStrategyHalted),
			wantStarted:        true,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyOnce,
		},
		{
			name:               "Once: VM already running with Once",
			runPolicy:          RunPolicyOnce,
			initialVM:          createTestVM("test-vm", "default", RunStrategyOnce),
			wantStarted:        false,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyOnce,
		},
		{
			name:               "Once: Change from Always to Once",
			runPolicy:          RunPolicyOnce,
			initialVM:          createTestVM("test-vm", "default", RunStrategyAlways),
			wantStarted:        false,
			wantStrategyChange: true,
			wantRunStrategy:    RunStrategyOnce,
		},
		{
			name:               "Once: Change from RerunOnFailure to Once",
			runPolicy:          RunPolicyOnce,
			initialVM:          createTestVM("test-vm", "default", RunStrategyRerunOnFailure),
			wantStarted:        false,
			wantStrategyChange: true,
			wantRunStrategy:    RunStrategyOnce,
		},
		{
			name:               "Once: Change from Manual to Once",
			runPolicy:          RunPolicyOnce,
			initialVM:          createTestVM("test-vm", "default", RunStrategyManual),
			wantStarted:        true,
			wantStrategyChange: false,
			wantRunStrategy:    RunStrategyOnce,
		},

		// Invalid policy test
		{
			name:          "Invalid run policy returns error",
			runPolicy:     RunPolicy("invalid"),
			initialVM:     createTestVM("test-vm", "default", RunStrategyHalted),
			wantError:     true,
			errorContains: "invalid run policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			client := fake.NewSimpleDynamicClient(scheme, tt.initialVM)
			ctx := context.Background()

			vm, wasStarted, strategyChanged, err := StartVM(ctx, client, tt.initialVM.GetNamespace(), tt.initialVM.GetName(), tt.runPolicy)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if vm == nil {
				t.Errorf("Expected non-nil VM, got nil")
				return
			}

			if wasStarted != tt.wantStarted {
				t.Errorf("wasStarted = %v, want %v", wasStarted, tt.wantStarted)
			}
			if strategyChanged != tt.wantStrategyChange {
				t.Errorf("strategyChanged = %v, want %v", strategyChanged, tt.wantStrategyChange)
			}

			// Verify the VM's runStrategy matches expected
			strategy, found, err := GetVMRunStrategy(vm)
			if err != nil {
				t.Errorf("Failed to get runStrategy: %v", err)
				return
			}
			if !found {
				t.Errorf("runStrategy not found")
				return
			}
			if strategy != tt.wantRunStrategy {
				t.Errorf("Strategy = %q, want %q", strategy, tt.wantRunStrategy)
			}
		})
	}
}

func TestStartVMNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClient(scheme)
	ctx := context.Background()

	_, _, _, err := StartVM(ctx, client, "default", "non-existent-vm", RunPolicyHighAvailability)
	if err == nil {
		t.Errorf("Expected error for non-existent VM, got nil")
		return
	}
	if !strings.Contains(err.Error(), "failed to get VirtualMachine") {
		t.Errorf("Error = %v, want to contain 'failed to get VirtualMachine'", err)
	}
}

func TestStopVM(t *testing.T) {
	tests := []struct {
		name          string
		initialVM     *unstructured.Unstructured
		wantStopped   bool
		wantError     bool
		errorContains string
	}{
		{
			name:        "Stop VM that is running (Always)",
			initialVM:   createTestVM("test-vm", "default", RunStrategyAlways),
			wantStopped: true,
			wantError:   false,
		},
		{
			name:        "Stop VM that is already stopped (Halted)",
			initialVM:   createTestVM("test-vm", "default", RunStrategyHalted),
			wantStopped: false,
			wantError:   false,
		},
		{
			name: "Stop VM without runStrategy",
			initialVM: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubevirt.io/v1",
					"kind":       "VirtualMachine",
					"metadata": map[string]interface{}{
						"name":      "test-vm",
						"namespace": "default",
					},
					"spec": map[string]interface{}{},
				},
			},
			wantStopped: true,
			wantError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			client := fake.NewSimpleDynamicClient(scheme, tt.initialVM)
			ctx := context.Background()

			vm, wasStopped, err := StopVM(ctx, client, tt.initialVM.GetNamespace(), tt.initialVM.GetName())

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if vm == nil {
				t.Errorf("Expected non-nil VM, got nil")
				return
			}

			if wasStopped != tt.wantStopped {
				t.Errorf("wasStopped = %v, want %v", wasStopped, tt.wantStopped)
			}

			// Verify the VM's runStrategy is Halted
			strategy, found, err := GetVMRunStrategy(vm)
			if err != nil {
				t.Errorf("Failed to get runStrategy: %v", err)
				return
			}
			if !found {
				t.Errorf("runStrategy not found")
				return
			}
			if strategy != RunStrategyHalted {
				t.Errorf("Strategy = %q, want %q", strategy, RunStrategyHalted)
			}
		})
	}
}

func TestStopVMNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClient(scheme)
	ctx := context.Background()

	_, _, err := StopVM(ctx, client, "default", "non-existent-vm")
	if err == nil {
		t.Errorf("Expected error for non-existent VM, got nil")
		return
	}
	if !strings.Contains(err.Error(), "failed to get VirtualMachine") {
		t.Errorf("Error = %v, want to contain 'failed to get VirtualMachine'", err)
	}
}

func TestRestartVM(t *testing.T) {
	tests := []struct {
		name          string
		initialVM     *unstructured.Unstructured
		wantError     bool
		errorContains string
	}{
		{
			name:      "Restart VM that is running (Always)",
			initialVM: createTestVM("test-vm", "default", RunStrategyAlways),
			wantError: false,
		},
		{
			name:      "Restart VM that is stopped (Halted)",
			initialVM: createTestVM("test-vm", "default", RunStrategyHalted),
			wantError: false,
		},
		{
			name: "Restart VM without runStrategy",
			initialVM: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubevirt.io/v1",
					"kind":       "VirtualMachine",
					"metadata": map[string]interface{}{
						"name":      "test-vm",
						"namespace": "default",
					},
					"spec": map[string]interface{}{},
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			client := fake.NewSimpleDynamicClient(scheme, tt.initialVM)
			ctx := context.Background()

			vm, err := RestartVM(ctx, client, tt.initialVM.GetNamespace(), tt.initialVM.GetName(), RunPolicyHighAvailability)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if vm == nil {
				t.Errorf("Expected non-nil VM, got nil")
				return
			}

			// Verify the VM's runStrategy is Always (after restart)
			strategy, found, err := GetVMRunStrategy(vm)
			if err != nil {
				t.Errorf("Failed to get runStrategy: %v", err)
				return
			}
			if !found {
				t.Errorf("runStrategy not found")
				return
			}
			if strategy != RunStrategyAlways {
				t.Errorf("Strategy = %q, want %q after restart", strategy, RunStrategyAlways)
			}
		})
	}
}

func TestRestartVMNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClient(scheme)
	ctx := context.Background()

	_, err := RestartVM(ctx, client, "default", "non-existent-vm", RunPolicyHighAvailability)
	if err == nil {
		t.Errorf("Expected error for non-existent VM, got nil")
		return
	}
	if !strings.Contains(err.Error(), "failed to get VirtualMachine") {
		t.Errorf("Error = %v, want to contain 'failed to get VirtualMachine'", err)
	}
}

func TestRestartVMWithDifferentRunPolicies(t *testing.T) {
	tests := []struct {
		name             string
		runPolicy        RunPolicy
		expectedStrategy RunStrategy
	}{
		{
			name:             "Restart with HighAvailability",
			runPolicy:        RunPolicyHighAvailability,
			expectedStrategy: RunStrategyAlways,
		},
		{
			name:             "Restart with RestartOnFailure",
			runPolicy:        RunPolicyRestartOnFailure,
			expectedStrategy: RunStrategyRerunOnFailure,
		},
		{
			name:             "Restart with Once",
			runPolicy:        RunPolicyOnce,
			expectedStrategy: RunStrategyOnce,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			initialVM := createTestVM("test-vm", "default", RunStrategyAlways)
			client := fake.NewSimpleDynamicClient(scheme, initialVM)
			ctx := context.Background()

			vm, err := RestartVM(ctx, client, initialVM.GetNamespace(), initialVM.GetName(), tt.runPolicy)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if vm == nil {
				t.Errorf("Expected non-nil VM, got nil")
				return
			}

			// Verify the VM's runStrategy matches the expected strategy
			strategy, found, err := GetVMRunStrategy(vm)
			if err != nil {
				t.Errorf("Failed to get runStrategy: %v", err)
				return
			}
			if !found {
				t.Errorf("runStrategy not found")
				return
			}
			if strategy != tt.expectedStrategy {
				t.Errorf("Strategy = %q, want %q after restart with policy %q", strategy, tt.expectedStrategy, tt.runPolicy)
			}
		})
	}
}

func TestRestartVMWithInvalidRunPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	initialVM := createTestVM("test-vm", "default", RunStrategyAlways)
	client := fake.NewSimpleDynamicClient(scheme, initialVM)
	ctx := context.Background()

	_, err := RestartVM(ctx, client, initialVM.GetNamespace(), initialVM.GetName(), RunPolicy("invalid"))
	if err == nil {
		t.Errorf("Expected error for invalid run policy, got nil")
		return
	}
	if !strings.Contains(err.Error(), "invalid run policy") {
		t.Errorf("Error = %v, want to contain 'invalid run policy'", err)
	}
}

func TestPauseVM(t *testing.T) {
	tests := []struct {
		name          string
		serverStatus  int
		serverBody    string
		initialVM     *unstructured.Unstructured
		wantError     bool
		errorContains string
	}{
		{
			name:         "Pause running VM",
			serverStatus: http.StatusOK,
			initialVM:    createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:    false,
		},
		{
			name:          "VMI not found",
			serverStatus:  http.StatusNotFound,
			serverBody:    `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","message":"virtualmachineinstances.subresources.kubevirt.io \"test-vm\" not found","code":404}`,
			initialVM:     createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:     true,
			errorContains: "failed to pause VirtualMachineInstance",
		},
		{
			name:          "VM already paused",
			serverStatus:  http.StatusConflict,
			serverBody:    `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Conflict","message":"VM is already paused","code":409}`,
			initialVM:     createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:     true,
			errorContains: "failed to pause VirtualMachineInstance",
		},
		{
			name:          "Migration in progress",
			serverStatus:  http.StatusConflict,
			serverBody:    `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Conflict","message":"cannot pause VMI with a live migration in progress","code":409}`,
			initialVM:     createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:     true,
			errorContains: "failed to pause VirtualMachineInstance",
		},
		{
			name:          "Pause fails when subresource returns error",
			serverStatus:  http.StatusInternalServerError,
			initialVM:     createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:     true,
			errorContains: "failed to pause VirtualMachineInstance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					t.Errorf("expected PUT, got %s", r.Method)
				}
				if !strings.HasSuffix(r.URL.Path, "/pause") {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if tt.serverBody != "" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tt.serverStatus)
				if tt.serverBody != "" {
					_, _ = w.Write([]byte(tt.serverBody))
				}
			}))
			defer server.Close()

			restConfig := createTestRESTConfig(server)
			scheme := runtime.NewScheme()
			dynamicClient := fake.NewSimpleDynamicClient(scheme, tt.initialVM)
			ctx := context.Background()

			vm, err := PauseVM(ctx, dynamicClient, restConfig, tt.initialVM.GetNamespace(), tt.initialVM.GetName())

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if vm == nil {
				t.Errorf("Expected non-nil VM, got nil")
			}
		})
	}
}

func TestUnpauseVM(t *testing.T) {
	tests := []struct {
		name          string
		serverStatus  int
		serverBody    string
		initialVM     *unstructured.Unstructured
		wantError     bool
		errorContains string
	}{
		{
			name:         "Unpause paused VM",
			serverStatus: http.StatusOK,
			initialVM:    createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:    false,
		},
		{
			name:          "VMI not found",
			serverStatus:  http.StatusNotFound,
			serverBody:    `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","message":"virtualmachineinstances.subresources.kubevirt.io \"test-vm\" not found","code":404}`,
			initialVM:     createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:     true,
			errorContains: "failed to unpause VirtualMachineInstance",
		},
		{
			name:          "VM not paused",
			serverStatus:  http.StatusConflict,
			serverBody:    `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Conflict","message":"VMI is not paused","code":409}`,
			initialVM:     createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:     true,
			errorContains: "failed to unpause VirtualMachineInstance",
		},
		{
			name:          "Unpause fails when subresource returns error",
			serverStatus:  http.StatusInternalServerError,
			initialVM:     createTestVM("test-vm", "default", RunStrategyAlways),
			wantError:     true,
			errorContains: "failed to unpause VirtualMachineInstance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					t.Errorf("expected PUT, got %s", r.Method)
				}
				if !strings.HasSuffix(r.URL.Path, "/unpause") {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if tt.serverBody != "" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tt.serverStatus)
				if tt.serverBody != "" {
					_, _ = w.Write([]byte(tt.serverBody))
				}
			}))
			defer server.Close()

			restConfig := createTestRESTConfig(server)
			scheme := runtime.NewScheme()
			dynamicClient := fake.NewSimpleDynamicClient(scheme, tt.initialVM)
			ctx := context.Background()

			vm, err := UnpauseVM(ctx, dynamicClient, restConfig, tt.initialVM.GetNamespace(), tt.initialVM.GetName())

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if vm == nil {
				t.Errorf("Expected non-nil VM, got nil")
			}
		})
	}
}
