package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// mockToolCallRequest implements api.ToolCallRequest for testing
type mockToolCallRequest struct {
	args map[string]any
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.args
}

func TestTroubleshoot(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]interface{}
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name: "generates troubleshooting guide",
			args: map[string]interface{}{
				"namespace": "test-ns",
				"name":      "test-vm",
				"action":    "troubleshoot",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !strings.Contains(result, "VirtualMachine Troubleshooting Guide") {
					t.Errorf("Expected troubleshooting guide header")
				}
				if !strings.Contains(result, "test-vm") {
					t.Errorf("Expected VM name in guide")
				}
				if !strings.Contains(result, "test-ns") {
					t.Errorf("Expected namespace in guide")
				}
				if !strings.Contains(result, "Step 1: Check VirtualMachine Status") {
					t.Errorf("Expected step 1 header")
				}
				if !strings.Contains(result, "resources_get") {
					t.Errorf("Expected resources_get tool reference")
				}
				if !strings.Contains(result, "VirtualMachineInstance") {
					t.Errorf("Expected VMI section")
				}
				if !strings.Contains(result, "virt-launcher") {
					t.Errorf("Expected virt-launcher pod section")
				}
			},
		},
		{
			name: "missing namespace",
			args: map[string]interface{}{
				"name":   "test-vm",
				"action": "troubleshoot",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			args: map[string]interface{}{
				"namespace": "test-ns",
				"action":    "troubleshoot",
			},
			wantErr: true,
		},
	}

	// Get the tool through the public API
	tools := Tools()
	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}
	vmTroubleshootTool := tools[0]

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				Context:         context.Background(),
				ToolCallRequest: &mockToolCallRequest{args: tt.args},
			}

			// Call through the public Handler interface
			result, err := vmTroubleshootTool.Handler(params)
			if err != nil {
				t.Errorf("Handler() unexpected Go error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected non-nil result")
				return
			}

			if tt.wantErr {
				if result.Error == nil {
					t.Error("Expected error in result.Error, got nil")
				}
			} else {
				if result.Error != nil {
					t.Errorf("Expected no error in result, got: %v", result.Error)
				}
				if result.Content == "" {
					t.Error("Expected non-empty result content")
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, result.Content)
				}
			}
		})
	}
}
