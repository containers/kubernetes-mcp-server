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
			name:       "rateInterval trims and converts to seconds",
			args:       map[string]any{"rateInterval": " 5m "},
			key:        "rateInterval",
			defaultVal: "10m",
			want:       "300",
			wantErr:    false,
		},
		{
			name:       "rateInterval empty returns default converted to seconds",
			args:       map[string]any{"rateInterval": ""},
			key:        "rateInterval",
			defaultVal: "10m",
			want:       "600",
			wantErr:    false,
		},
		{
			name:       "rateInterval invalid suffix returns error",
			args:       map[string]any{"rateInterval": "5x"},
			key:        "rateInterval",
			defaultVal: "10m",
			want:       "",
			wantErr:    true,
		},
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

func TestRateIntervalToSeconds(t *testing.T) {
	tests := []struct {
		in       string
		want     int64
		wantErr  bool
		testName string
	}{
		{in: "10m", want: 600, wantErr: false, testName: "minutes"},
		{in: "10", want: 10, wantErr: false, testName: "no suffix seconds"},
		{in: "10s", want: 10, wantErr: false, testName: "seconds suffix"},
		{in: "1h", want: 3600, wantErr: false, testName: "hours"},
		{in: "2d", want: 172800, wantErr: false, testName: "days"},
		{in: " 5m ", want: 300, wantErr: false, testName: "trim spaces"},
		{in: "0m", want: 0, wantErr: false, testName: "zero value"},
		{in: "", want: 0, wantErr: true, testName: "empty"},
		{in: "m", want: 0, wantErr: true, testName: "missing number"},
		{in: "5x", want: 0, wantErr: true, testName: "invalid suffix"},
		{in: "10.5m", want: 0, wantErr: true, testName: "decimal not allowed"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			got, err := rateIntervalToSeconds(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (input=%q, got=%d)", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("rateIntervalToSeconds(%q) = %d, want %d", tt.in, got, tt.want)
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
			name: "converts special default duration",
			in: args{
				key:        "duration",
				defaultVal: "10m",
				errMsg:     "invalid duration",
				args:       map[string]any{},
			},
			wantKey:   "duration",
			wantValue: "600",
		},
		{
			name: "converts special provided duration",
			in: args{
				key:        "duration",
				defaultVal: "10m",
				errMsg:     "invalid duration",
				args:       map[string]any{"duration": "2h"},
			},
			wantKey:   "duration",
			wantValue: "7200",
		},
		{
			name: "returns custom error for invalid rateInterval",
			in: args{
				key:        "rateInterval",
				defaultVal: "10m",
				errMsg:     "invalid rateInterval: invalid rateInterval/duration suffix: \"x\", values must be in the format '10m', '5m', '1h', '2d' or seconds",
				args:       map[string]any{"rateInterval": "5x"},
			},
			wantKey: "rateInterval",
			wantErr: "invalid rateInterval: invalid rateInterval/duration suffix: \"x\", values must be in the format '10m', '5m', '1h', '2d' or seconds",
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
