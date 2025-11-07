package stop

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type mockToolCallRequest struct {
	arguments map[string]interface{}
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.arguments
}

func TestStopParameterValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing namespace parameter",
			args: map[string]interface{}{
				"name": "test-vm",
			},
			wantErr: true,
			errMsg:  "namespace parameter required",
		},
		{
			name: "missing name parameter",
			args: map[string]interface{}{
				"namespace": "test-ns",
			},
			wantErr: true,
			errMsg:  "name parameter required",
		},
		{
			name: "invalid namespace type",
			args: map[string]interface{}{
				"namespace": 123,
				"name":      "test-vm",
			},
			wantErr: true,
			errMsg:  "namespace parameter must be a string",
		},
		{
			name: "invalid name type",
			args: map[string]interface{}{
				"namespace": "test-ns",
				"name":      456,
			},
			wantErr: true,
			errMsg:  "name parameter must be a string",
		},
		{
			name: "valid parameters - cluster interaction expected",
			args: map[string]interface{}{
				"namespace": "test-ns",
				"name":      "test-vm",
			},
			wantErr: true, // Will fail due to missing cluster connection, but parameters are valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				Context:         context.Background(),
				Kubernetes:      &internalk8s.Kubernetes{},
				ToolCallRequest: &mockToolCallRequest{arguments: tt.args},
			}

			result, err := stop(params)
			if err != nil {
				t.Errorf("stop() unexpected Go error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected non-nil result")
				return
			}

			// For parameter validation errors, check the error message
			if tt.wantErr && tt.errMsg != "" {
				if result.Error == nil {
					t.Error("Expected error in result.Error, got nil")
					return
				}
				if result.Error.Error() != tt.errMsg {
					t.Errorf("Expected error message %q, got %q", tt.errMsg, result.Error.Error())
				}
			}
		})
	}
}
