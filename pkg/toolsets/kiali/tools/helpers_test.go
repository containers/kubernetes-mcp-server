package tools

import (
	"encoding/json"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type mockToolCallRequest struct {
	args map[string]any
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.args
}

func TestGetStringArgOrDefault(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal string
		want       string
		wantErr    bool
	}{
		{
			name:       "returns default when string is whitespace",
			args:       map[string]any{"graphType": "   "},
			key:        "graphType",
			defaultVal: "app",
			want:       "app",
			wantErr:    false,
		},
		{
			name:       "returns default when key missing",
			args:       map[string]any{},
			key:        "missing",
			defaultVal: "fallback",
			want:       "fallback",
			wantErr:    false,
		},
		{
			name:       "converts float64 without trailing zeros",
			args:       map[string]any{"step": 10.0},
			key:        "step",
			defaultVal: "15",
			want:       "10",
			wantErr:    false,
		},
		{
			name:       "converts float64 with decimals",
			args:       map[string]any{"step": 10.5},
			key:        "step",
			defaultVal: "15",
			want:       "10.5",
			wantErr:    false,
		},
		{
			name:       "converts int",
			args:       map[string]any{"duration": 1800},
			key:        "duration",
			defaultVal: "3600",
			want:       "1800",
			wantErr:    false,
		},
		{
			name:       "converts int64",
			args:       map[string]any{"duration": int64(7200)},
			key:        "duration",
			defaultVal: "3600",
			want:       "7200",
			wantErr:    false,
		},
		{
			name:       "converts json.Number",
			args:       map[string]any{"duration": json.Number("300")},
			key:        "duration",
			defaultVal: "3600",
			want:       "300",
			wantErr:    false,
		},
		{
			name:       "formats other types via Sprint",
			args:       map[string]any{"flag": true},
			key:        "flag",
			defaultVal: "false",
			want:       "true",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				ToolCallRequest: &mockToolCallRequest{args: tt.args},
			}
			got, err := getStringArgOrDefault(params, tt.key, tt.defaultVal)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("getStringArgOrDefault() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetQueryParam(t *testing.T) {
	type args struct {
		key        string
		defaultVal string
		errMsg     string
		args       map[string]any
	}
	tests := []struct {
		name      string
		in        args
		wantKey   string
		wantValue string
		wantErr   string
	}{
		{
			name: "sets string value from args",
			in: args{
				key:        "step",
				defaultVal: "15",
				errMsg:     "invalid step",
				args:       map[string]any{"step": "30"},
			},
			wantKey:   "step",
			wantValue: "30",
		},
		{
			name: "uses default for missing non-special key",
			in: args{
				key:        "reporter",
				defaultVal: "source",
				errMsg:     "invalid reporter",
				args:       map[string]any{},
			},
			wantKey:   "reporter",
			wantValue: "source",
		},
		{
			name: "uses default duration without conversion",
			in: args{
				key:        "duration",
				defaultVal: "10m",
				errMsg:     "invalid duration",
				args:       map[string]any{},
			},
			wantKey:   "duration",
			wantValue: "10m",
		},
		{
			name: "uses provided duration without conversion",
			in: args{
				key:        "duration",
				defaultVal: "10m",
				errMsg:     "invalid duration",
				args:       map[string]any{"duration": "2h"},
			},
			wantKey:   "duration",
			wantValue: "2h",
		},
		{
			name: "accepts numeric tail",
			in: args{
				key:        "tail",
				defaultVal: "100",
				errMsg:     "invalid tail",
				args:       map[string]any{"tail": 200},
			},
			wantKey:   "tail",
			wantValue: "200",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			params := api.ToolHandlerParams{
				ToolCallRequest: &mockToolCallRequest{args: tt.in.args},
			}
			q := make(map[string]string)
			err := setQueryParam(params, q, tt.in.key, tt.in.defaultVal)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error mismatch: got %q, want %q", err.Error(), tt.wantErr)
				}
				if _, ok := q[tt.wantKey]; ok {
					t.Fatalf("queryParams[%q] should not be set on error", tt.wantKey)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, ok := q[tt.wantKey]
			if !ok {
				t.Fatalf("queryParams[%q] not set", tt.wantKey)
			}
			if got != tt.wantValue {
				t.Fatalf("queryParams[%q] = %q, want %q", tt.wantKey, got, tt.wantValue)
			}
		})
	}
}
