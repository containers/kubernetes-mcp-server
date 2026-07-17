package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/rhobs/obs-mcp/pkg/auth"
	"github.com/rhobs/obs-mcp/pkg/metrics/prometheus"
)

func parseConfig(t *testing.T, tomlStr string) *Config {
	t.Helper()
	var cfg Config
	if _, err := toml.Decode(tomlStr, &cfg); err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}
	return &cfg
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr string // substring of the expected error; empty means no error
	}{
		{
			name: "empty config is valid",
			toml: ``,
		},
		{
			name: "auth_mode header is valid",
			toml: `auth_mode = "header"`,
		},
		{
			name: "auth_mode kubeconfig is valid",
			toml: `auth_mode = "kubeconfig"`,
		},
		{
			name:    "unknown auth_mode returns error",
			toml:    `auth_mode = "magic"`,
			wantErr: `invalid auth_mode`,
		},
		// test just a sub set of guardrails validations, the rest is covered in `TestGetGuardrails`
		{
			name: "guardrails named list is valid",
			toml: `guardrails = "require-label-matcher,disallow-blanket-regex"`,
		},
		{
			name:    "unknown guardrail name returns error",
			toml:    `guardrails = "not-a-real-guardrail"`,
			wantErr: `unknown guardrail`,
		},
		{
			name: "max_metric_cardinality without enabling the guardrail returns error",
			toml: `
guardrails = "require-label-matcher"
max_metric_cardinality = 5000
`,
			wantErr: "max_metric_cardinality is set but",
		},
		{
			name: "full valid config",
			toml: `
auth_mode = "kubeconfig"
prometheus_url = "https://thanos.example.com"
alertmanager_url = "https://alertmanager.example.com"
insecure = true
guardrails = "all"
max_metric_cardinality = 5000
max_label_cardinality = 200
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseConfig(t, tt.toml)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("Validate() expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestGetAuthMode(t *testing.T) {
	tests := []struct {
		name string
		toml string
		want auth.AuthMode
	}{
		{
			name: "empty auth_mode defaults to header",
			toml: ``,
			want: auth.AuthModeHeader,
		},
		{
			name: "auth_mode header returns header",
			toml: `auth_mode = "header"`,
			want: auth.AuthModeHeader,
		},
		{
			name: "auth_mode kubeconfig returns kubeconfig",
			toml: `auth_mode = "kubeconfig"`,
			want: auth.AuthModeKubeConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseConfig(t, tt.toml)
			got := cfg.GetAuthMode()
			if got != tt.want {
				t.Errorf("GetAuthMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetGuardrails(t *testing.T) {
	tests := []struct {
		name           string
		toml           string
		wantErr        string
		wantNil        bool
		wantGuardrails *prometheus.Guardrails
	}{
		{
			name:           "empty config returns all guardrails enabled with defaults",
			toml:           ``,
			wantGuardrails: prometheus.DefaultGuardrails(true),
		},
		{
			name:           "guardrails all returns all enabled",
			toml:           `guardrails = "all"`,
			wantGuardrails: prometheus.DefaultGuardrails(true),
		},
		{
			name:    "guardrails none returns nil",
			toml:    `guardrails = "none"`,
			wantNil: true,
		},
		{
			name: "named guardrail list enables only listed",
			toml: `guardrails = "require-label-matcher,disallow-blanket-regex"`,
			wantGuardrails: &prometheus.Guardrails{
				RequireLabelMatcher:  true,
				DisallowBlanketRegex: true,
				MaxMetricCardinality: prometheus.DefaultMaxMetricCardinality,
				MaxLabelCardinality:  prometheus.DefaultMaxLabelCardinality,
			},
		},
		{
			name: "max_metric_cardinality overrides default when guardrail is enabled",
			toml: `
guardrails = "max-metric-cardinality"
max_metric_cardinality = 5000
`,
			wantGuardrails: &prometheus.Guardrails{
				ForceMaxMetricCardinality: true,
				MaxMetricCardinality:      5000,
				MaxLabelCardinality:       prometheus.DefaultMaxLabelCardinality,
			},
		},
		{
			name: "max_label_cardinality overrides default when disallow-blanket-regex is enabled",
			toml: `
guardrails = "disallow-blanket-regex"
max_label_cardinality = 200
`,
			wantGuardrails: &prometheus.Guardrails{
				DisallowBlanketRegex: true,
				MaxMetricCardinality: prometheus.DefaultMaxMetricCardinality,
				MaxLabelCardinality:  200,
			},
		},
		{
			name: "max_label_cardinality zero sets threshold to zero (always disallow blanket regex)",
			toml: `
guardrails = "disallow-blanket-regex"
max_label_cardinality = 0
`,
			wantGuardrails: &prometheus.Guardrails{
				DisallowBlanketRegex: true,
				MaxMetricCardinality: prometheus.DefaultMaxMetricCardinality,
				MaxLabelCardinality:  0,
			},
		},
		{
			name: "max_metric_cardinality zero returns error",
			toml: `
guardrails = "max-metric-cardinality"
max_metric_cardinality = 0
`,
			wantErr: "max_metric_cardinality = 0 is not supported to disable the guardrail",
		},
		{
			name: "guardrails none with max_metric_cardinality returns error",
			toml: `
guardrails = "none"
max_metric_cardinality = 5000
`,
			wantErr: "max_metric_cardinality is set but",
		},
		{
			name: "guardrails none with max_label_cardinality returns error",
			toml: `
guardrails = "none"
max_label_cardinality = 200
`,
			wantErr: "max_label_cardinality is set but",
		},
		{
			name: "max_metric_cardinality without enabling the guardrail returns error",
			toml: `
guardrails = "require-label-matcher"
max_metric_cardinality = 5000
`,
			wantErr: "max_metric_cardinality is set but",
		},
		{
			name: "max_label_cardinality without disallow-blanket-regex returns error",
			toml: `
guardrails = "require-label-matcher"
max_label_cardinality = 200
`,
			wantErr: "max_label_cardinality is set but",
		},
		{
			name: "all guardrails with custom cardinalities",
			toml: `
guardrails = "all"
max_metric_cardinality = 10000
max_label_cardinality = 300
`,
			wantGuardrails: &prometheus.Guardrails{
				DisallowExplicitNameLabel: true,
				RequireLabelMatcher:       true,
				DisallowBlanketRegex:      true,
				ForceMaxMetricCardinality: true,
				MaxMetricCardinality:      10000,
				MaxLabelCardinality:       300,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseConfig(t, tt.toml)
			got, err := cfg.GetGuardrails()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("GetGuardrails() expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("GetGuardrails() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetGuardrails() unexpected error: %v", err)
			}

			if tt.wantNil {
				if got != nil {
					t.Errorf("GetGuardrails() = %+v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("GetGuardrails() = nil, want %+v", tt.wantGuardrails)
			}
			if *got != *tt.wantGuardrails {
				t.Errorf("GetGuardrails()\n got  %+v\n want %+v", *got, *tt.wantGuardrails)
			}
		})
	}
}
