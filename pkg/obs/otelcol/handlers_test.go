package otelcol

import (
	"context"
	"slices"
	"testing"

	"github.com/os-observability/redhat-opentelemetry-collector/configschemas"
)

func TestListComponentsHandler(t *testing.T) {
	loader := NewSchemaLoaderFromFS(configschemas.Schemas, "schemas")
	ctx := context.Background()

	input := ListComponentsInput{Version: "0.144.0"}
	result := ListComponentsHandler(ctx, loader, input)

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	output, ok := result.Data.(ListComponentsOutput)
	if !ok {
		t.Fatalf("unexpected result type: %T", result.Data)
	}

	// Verify known components are present
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
	loader := NewSchemaLoaderFromFS(configschemas.Schemas, "schemas")
	ctx := context.Background()

	// Empty version should default to latest
	input := ListComponentsInput{}
	result := ListComponentsHandler(ctx, loader, input)

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetComponentSchemaHandler(t *testing.T) {
	loader := NewSchemaLoaderFromFS(configschemas.Schemas, "schemas")
	ctx := context.Background()

	tests := []struct {
		name    string
		input   GetComponentSchemaInput
		wantErr bool
	}{
		{
			name: "valid receiver",
			input: GetComponentSchemaInput{
				ComponentType: ComponentTypeReceiver,
				ComponentName: "otlp",
				Version:       "0.144.0",
			},
			wantErr: false,
		},
		{
			name: "valid processor",
			input: GetComponentSchemaInput{
				ComponentType: ComponentTypeProcessor,
				ComponentName: "batch",
				Version:       "0.144.0",
			},
			wantErr: false,
		},
		{
			name: "version with v prefix",
			input: GetComponentSchemaInput{
				ComponentType: ComponentTypeProcessor,
				ComponentName: "batch",
				Version:       "v0.144.0",
			},
			wantErr: false,
		},
		{
			name: "invalid component type",
			input: GetComponentSchemaInput{
				ComponentType: "invalid",
				ComponentName: "otlp",
			},
			wantErr: true,
		},
		{
			name: "missing component name",
			input: GetComponentSchemaInput{
				ComponentType: ComponentTypeReceiver,
			},
			wantErr: true,
		},
		{
			name: "nonexistent component",
			input: GetComponentSchemaInput{
				ComponentType: ComponentTypeReceiver,
				ComponentName: "nonexistent_xyz",
				Version:       "0.144.0",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetComponentSchemaHandler(ctx, loader, tt.input)
			if (result.Error != nil) != tt.wantErr {
				t.Errorf("GetComponentSchemaHandler() error = %v, wantErr %v", result.Error, tt.wantErr)
			}
		})
	}
}

func TestGetComponentSchemaHandler_SchemaContent(t *testing.T) {
	loader := NewSchemaLoaderFromFS(configschemas.Schemas, "schemas")
	ctx := context.Background()

	input := GetComponentSchemaInput{
		ComponentType: ComponentTypeProcessor,
		ComponentName: "batch",
		Version:       "0.144.0",
	}
	result := GetComponentSchemaHandler(ctx, loader, input)

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	output, ok := result.Data.(GetComponentSchemaOutput)
	if !ok {
		t.Fatalf("unexpected result type: %T", result.Data)
	}

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
	loader := NewSchemaLoaderFromFS(configschemas.Schemas, "schemas")
	ctx := context.Background()

	tests := []struct {
		name      string
		input     ValidateConfigInput
		wantErr   bool
		wantValid bool
	}{
		{
			name: "valid yaml config",
			input: ValidateConfigInput{
				ComponentType: ComponentTypeProcessor,
				ComponentName: "batch",
				Config:        "send_batch_size: 8192\ntimeout: 200ms",
				Format:        "yaml",
				Version:       "0.144.0",
			},
			wantErr:   false,
			wantValid: true,
		},
		{
			name: "valid json config",
			input: ValidateConfigInput{
				ComponentType: ComponentTypeProcessor,
				ComponentName: "batch",
				Config:        `{"send_batch_size": 8192, "timeout": "200ms"}`,
				Format:        "json",
				Version:       "0.144.0",
			},
			wantErr:   false,
			wantValid: true,
		},
		{
			name: "invalid config - unknown field",
			input: ValidateConfigInput{
				ComponentType: ComponentTypeProcessor,
				ComponentName: "batch",
				Config:        "does_not_exist: 200ms",
				Format:        "yaml",
				Version:       "0.144.0",
			},
			wantErr:   false,
			wantValid: false,
		},
		{
			name: "invalid format",
			input: ValidateConfigInput{
				ComponentType: ComponentTypeReceiver,
				ComponentName: "otlp",
				Config:        "{}",
				Format:        "xml",
			},
			wantErr: true,
		},
		{
			name: "missing config",
			input: ValidateConfigInput{
				ComponentType: ComponentTypeReceiver,
				ComponentName: "otlp",
			},
			wantErr: true,
		},
		{
			name: "missing component name",
			input: ValidateConfigInput{
				ComponentType: ComponentTypeReceiver,
				Config:        "{}",
				Format:        "json",
			},
			wantErr: true,
		},
		{
			name: "invalid component type",
			input: ValidateConfigInput{
				ComponentType: "invalid",
				ComponentName: "batch",
				Config:        "{}",
				Format:        "json",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateConfigHandler(ctx, loader, tt.input)
			if (result.Error != nil) != tt.wantErr {
				t.Errorf("ValidateConfigHandler() error = %v, wantErr %v", result.Error, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			output, ok := result.Data.(ValidateConfigOutput)
			if !ok {
				t.Fatalf("unexpected result type: %T", result.Data)
			}
			if output.Valid != tt.wantValid {
				t.Errorf("ValidateConfigHandler() valid = %v, wantValid %v, errors = %v",
					output.Valid, tt.wantValid, output.Errors)
			}
		})
	}
}

func TestGetVersionsHandler(t *testing.T) {
	loader := NewSchemaLoaderFromFS(configschemas.Schemas, "schemas")
	ctx := context.Background()

	result := GetVersionsHandler(ctx, loader, GetVersionsInput{})

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	output, ok := result.Data.(GetVersionsOutput)
	if !ok {
		t.Fatalf("unexpected result type: %T", result.Data)
	}

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
