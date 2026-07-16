package otelcol

import (
	"slices"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/os-observability/redhat-opentelemetry-collector/configschemas"
)

type mockBaseConfig struct {
	api.BaseConfig
	config *Config
}

func (m *mockBaseConfig) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	if name == ToolsetName && m.config != nil {
		return m.config, true
	}
	return nil, false
}

type mockToolCallRequest struct {
	arguments map[string]any
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.arguments
}

func handlerParams(t *testing.T, args map[string]any) api.ToolHandlerParams {
	t.Helper()
	return api.ToolHandlerParams{
		Context:         t.Context(),
		BaseConfig:      &mockBaseConfig{config: &Config{SchemaFS: configschemas.Schemas}},
		ToolCallRequest: &mockToolCallRequest{arguments: args},
	}
}

func TestListComponentsHandler(t *testing.T) {
	result, err := ListComponentsHandler(handlerParams(t, map[string]any{
		"version": "0.144.0",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("unexpected tool error: %v", result.Error)
	}

	output := result.StructuredContent.(ListComponentsOutput)

	if len(output.Processors) == 0 {
		t.Error("expected processors to be non-empty")
	}
	if len(output.Receivers) == 0 {
		t.Error("expected receivers to be non-empty")
	}

	// Verify specific known components
	if !slices.Contains(output.Receivers, "otlp") {
		t.Error("expected otlp receiver to be present")
	}
	if !slices.Contains(output.Processors, "batch") {
		t.Error("expected batch processor to be present")
	}
}

func TestListComponentsHandler_DefaultVersion(t *testing.T) {
	// Empty version should default to latest
	result, err := ListComponentsHandler(handlerParams(t, map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("unexpected tool error: %v", result.Error)
	}
}

func TestGetComponentSchemaHandler(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{
			name: "valid receiver",
			args: map[string]any{
				"component_type": "receiver",
				"component_name": "otlp",
				"version":        "0.144.0",
			},
			wantErr: false,
		},
		{
			name: "valid processor",
			args: map[string]any{
				"component_type": "processor",
				"component_name": "batch",
				"version":        "0.144.0",
			},
			wantErr: false,
		},
		{
			name: "version with v prefix",
			args: map[string]any{
				"component_type": "processor",
				"component_name": "batch",
				"version":        "v0.144.0",
			},
			wantErr: false,
		},
		{
			name: "invalid component type",
			args: map[string]any{
				"component_type": "invalid",
				"component_name": "otlp",
			},
			wantErr: true,
		},
		{
			name: "missing component name",
			args: map[string]any{
				"component_type": "receiver",
			},
			wantErr: true,
		},
		{
			name: "nonexistent component",
			args: map[string]any{
				"component_type": "receiver",
				"component_name": "nonexistent_xyz",
				"version":        "0.144.0",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetComponentSchemaHandler(handlerParams(t, tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (result.Error != nil) != tt.wantErr {
				t.Errorf("getComponentSchemaHandler() error = %v, wantErr %v", result.Error, tt.wantErr)
			}
		})
	}
}

func TestGetComponentSchemaHandler_SchemaContent(t *testing.T) {
	result, err := GetComponentSchemaHandler(handlerParams(t, map[string]any{
		"component_type": "processor",
		"component_name": "batch",
		"version":        "0.144.0",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("unexpected tool error: %v", result.Error)
	}

	output := result.StructuredContent.(GetComponentSchemaOutput)

	if output.Name != "batch" {
		t.Errorf("expected name 'batch', got %q", output.Name)
	}
	if output.Type != "processor" {
		t.Errorf("expected type 'processor', got %q", output.Type)
	}
	if output.Schema == nil {
		t.Error("expected schema to be non-nil")
	}

	// Verify schema has expected properties
	props, ok := output.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected schema to have properties")
	}
	if _, ok := props["send_batch_size"]; !ok {
		t.Error("expected schema to have send_batch_size property")
	}
	if _, ok := props["timeout"]; !ok {
		t.Error("expected schema to have timeout property")
	}
}

func TestValidateConfigHandler(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		wantErr   bool
		wantValid bool
	}{
		{
			name: "valid yaml config",
			args: map[string]any{
				"component_type": "processor",
				"component_name": "batch",
				"config":         "send_batch_size: 8192\ntimeout: 200ms",
				"format":         "yaml",
				"version":        "0.144.0",
			},
			wantErr:   false,
			wantValid: true,
		},
		{
			name: "valid json config",
			args: map[string]any{
				"component_type": "processor",
				"component_name": "batch",
				"config":         `{"send_batch_size": 8192, "timeout": "200ms"}`,
				"format":         "json",
				"version":        "0.144.0",
			},
			wantErr:   false,
			wantValid: true,
		},
		{
			name: "invalid config - unknown field",
			args: map[string]any{
				"component_type": "processor",
				"component_name": "batch",
				"config":         "does_not_exist: 200ms",
				"format":         "yaml",
				"version":        "0.144.0",
			},
			wantErr:   false,
			wantValid: false,
		},
		{
			name: "invalid format",
			args: map[string]any{
				"component_type": "receiver",
				"component_name": "otlp",
				"config":         "{}",
				"format":         "xml",
			},
			wantErr: true,
		},
		{
			name: "missing config",
			args: map[string]any{
				"component_type": "receiver",
				"component_name": "otlp",
			},
			wantErr: true,
		},
		{
			name: "missing component name",
			args: map[string]any{
				"component_type": "receiver",
				"config":         "{}",
				"format":         "json",
			},
			wantErr: true,
		},
		{
			name: "invalid component type",
			args: map[string]any{
				"component_type": "invalid",
				"component_name": "batch",
				"config":         "{}",
				"format":         "json",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateConfigHandler(handlerParams(t, tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (result.Error != nil) != tt.wantErr {
				t.Errorf("validateConfigHandler() error = %v, wantErr %v", result.Error, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			output := result.StructuredContent.(ValidateConfigOutput)
			if output.Valid != tt.wantValid {
				t.Errorf("validateConfigHandler() valid = %v, wantValid %v, errors = %v",
					output.Valid, tt.wantValid, output.Errors)
			}
		})
	}
}

func TestGetVersionsHandler(t *testing.T) {
	result, err := GetVersionsHandler(handlerParams(t, map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("unexpected tool error: %v", result.Error)
	}

	output := result.StructuredContent.(GetVersionsOutput)

	if len(output.Versions) == 0 {
		t.Error("expected versions to be non-empty")
	}
	if output.LatestVersion == "" {
		t.Error("expected latest version to be non-empty")
	}

	// Verify 0.144.0 is in the versions list
	if !slices.Contains(output.Versions, "0.144.0") {
		t.Errorf("expected 0.144.0 to be in versions list: %v", output.Versions)
	}
}

func TestComponentType_IsValid(t *testing.T) {
	tests := []struct {
		ct   ComponentType
		want bool
	}{
		{ComponentTypeReceiver, true},
		{ComponentTypeProcessor, true},
		{ComponentTypeExporter, true},
		{ComponentTypeExtension, true},
		{ComponentTypeConnector, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.ct), func(t *testing.T) {
			if got := tt.ct.IsValid(); got != tt.want {
				t.Errorf("ComponentType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v0.144.0", "0.144.0"},
		{"0.144.0", "0.144.0"},
		{"v1.0.0", "1.0.0"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeVersion(tt.input); got != tt.want {
				t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
