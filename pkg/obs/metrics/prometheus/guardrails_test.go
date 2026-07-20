package prometheus

import (
	"context"
	"testing"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func TestParseGuardrails(t *testing.T) {
	allEnabled := DefaultGuardrails(true)

	tests := []struct {
		name           string
		input          string
		wantNil        bool // expect (nil, nil)
		wantErr        bool
		wantGuardrails *Guardrails
	}{
		// Special keywords
		{
			name:    "none disables all guardrails",
			input:   "none",
			wantNil: true,
		},
		{
			name:    "NONE is case-insensitive",
			input:   "NONE",
			wantNil: true,
		},
		{
			name:           "all enables all guardrails",
			input:          "all",
			wantGuardrails: allEnabled,
		},
		{
			name:           "empty string enables all guardrails",
			input:          "",
			wantGuardrails: allEnabled,
		},
		{
			name:  "disallow-explicit-name-label only",
			input: GuardrailDisallowExplicitNameLabel,
			wantGuardrails: &Guardrails{
				DisallowExplicitNameLabel: true,
				MaxMetricCardinality:      DefaultMaxMetricCardinality,
				MaxLabelCardinality:       DefaultMaxLabelCardinality,
			},
		},
		{
			name:  "two guardrails",
			input: GuardrailDisallowExplicitNameLabel + "," + GuardrailRequireLabelMatcher,
			wantGuardrails: &Guardrails{
				DisallowExplicitNameLabel: true,
				RequireLabelMatcher:       true,
				MaxMetricCardinality:      DefaultMaxMetricCardinality,
				MaxLabelCardinality:       DefaultMaxLabelCardinality,
			},
		},
		// Whitespace tolerance
		{
			name:  "spaces around commas are trimmed",
			input: GuardrailDisallowExplicitNameLabel + " , " + GuardrailRequireLabelMatcher,
			wantGuardrails: &Guardrails{
				DisallowExplicitNameLabel: true,
				RequireLabelMatcher:       true,
				MaxMetricCardinality:      DefaultMaxMetricCardinality,
				MaxLabelCardinality:       DefaultMaxLabelCardinality,
			},
		},
		{
			name:  "empty segments from extra commas are ignored",
			input: "," + GuardrailRequireLabelMatcher + ",",
			wantGuardrails: &Guardrails{
				RequireLabelMatcher:  true,
				MaxMetricCardinality: DefaultMaxMetricCardinality,
				MaxLabelCardinality:  DefaultMaxLabelCardinality,
			},
		},
		// Case insensitivity for named guardrails
		{
			name:  "guardrail name is case-insensitive",
			input: "REQUIRE-LABEL-MATCHER",
			wantGuardrails: &Guardrails{
				RequireLabelMatcher:  true,
				MaxMetricCardinality: DefaultMaxMetricCardinality,
				MaxLabelCardinality:  DefaultMaxLabelCardinality,
			},
		},
		// Negative guardrails (! prefix disables a guardrail, all others remain enabled)
		{
			name:  "single negative guardrail disables only that one",
			input: "!" + GuardrailRequireLabelMatcher,
			wantGuardrails: &Guardrails{
				DisallowExplicitNameLabel: true,
				RequireLabelMatcher:       false,
				DisallowBlanketRegex:      true,
				ForceMaxMetricCardinality: true,
				MaxMetricCardinality:      DefaultMaxMetricCardinality,
				MaxLabelCardinality:       DefaultMaxLabelCardinality,
			},
		},
		{
			name:  "multiple negative guardrails disable each listed one",
			input: "!" + GuardrailDisallowExplicitNameLabel + ",!" + GuardrailRequireLabelMatcher,
			wantGuardrails: &Guardrails{
				DisallowExplicitNameLabel: false,
				RequireLabelMatcher:       false,
				DisallowBlanketRegex:      true,
				ForceMaxMetricCardinality: true,
				MaxMetricCardinality:      DefaultMaxMetricCardinality,
				MaxLabelCardinality:       DefaultMaxLabelCardinality,
			},
		},
		{
			name:  "!tsdb shortcut disables both TSDB-dependent guardrails",
			input: "!" + GuardrailShortcutTSDB,
			wantGuardrails: &Guardrails{
				DisallowExplicitNameLabel: true,
				RequireLabelMatcher:       true,
				DisallowBlanketRegex:      false,
				ForceMaxMetricCardinality: false,
				MaxMetricCardinality:      DefaultMaxMetricCardinality,
				MaxLabelCardinality:       DefaultMaxLabelCardinality,
			},
		},
		{
			name:    "tsdb shortcut without ! is not allowed",
			input:   GuardrailShortcutTSDB,
			wantErr: true,
		},
		{
			name:  "negative max-metric-cardinality disables ForceMaxMetricCardinality",
			input: "!" + GuardrailMaxMetricCardinality,
			wantGuardrails: &Guardrails{
				DisallowExplicitNameLabel: true,
				RequireLabelMatcher:       true,
				DisallowBlanketRegex:      true,
				ForceMaxMetricCardinality: false,
				MaxMetricCardinality:      DefaultMaxMetricCardinality,
				MaxLabelCardinality:       DefaultMaxLabelCardinality,
			},
		},
		{
			name:  "negative guardrail name is case-insensitive",
			input: "!" + "REQUIRE-LABEL-MATCHER",
			wantGuardrails: &Guardrails{
				DisallowExplicitNameLabel: true,
				RequireLabelMatcher:       false,
				DisallowBlanketRegex:      true,
				ForceMaxMetricCardinality: true,
				MaxMetricCardinality:      DefaultMaxMetricCardinality,
				MaxLabelCardinality:       DefaultMaxLabelCardinality,
			},
		},
		// Error cases
		{
			name:    "unknown name mixed with valid name returns error",
			input:   GuardrailRequireLabelMatcher + ",bad-name",
			wantErr: true,
		},
		{
			name:    "unknown negative name returns error",
			input:   "!bad-name",
			wantErr: true,
		},
		{
			name:    "mixing positive and negative guardrails returns error",
			input:   GuardrailDisallowExplicitNameLabel + ",!" + GuardrailRequireLabelMatcher,
			wantErr: true,
		},
		{
			name:    "mixing negative and positive guardrails returns error",
			input:   "!" + GuardrailDisallowExplicitNameLabel + "," + GuardrailRequireLabelMatcher,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGuardrails(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseGuardrails(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGuardrails(%q) unexpected error: %v", tt.input, err)
			}

			if tt.wantNil {
				if got != nil {
					t.Errorf("ParseGuardrails(%q) = %+v, want nil", tt.input, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("ParseGuardrails(%q) = nil, want non-nil", tt.input)
			}
			if *got != *tt.wantGuardrails {
				t.Errorf("ParseGuardrails(%q)\n got  %+v\n want %+v", tt.input, *got, *tt.wantGuardrails)
			}
		})
	}
}

func TestGuardrails_IsSafeQuery(t *testing.T) {
	// Use static guardrails without cardinality limits (no TSDB client needed)
	g := &Guardrails{
		DisallowExplicitNameLabel: true,
		RequireLabelMatcher:       true,
		DisallowBlanketRegex:      true,
		MaxMetricCardinality:      0, // Disabled - no TSDB needed
		MaxLabelCardinality:       0, // Disabled - blanket regex always rejected
	}
	tests := map[string]bool{
		// Rule 1: __name__ queries
		`{__name__="http_requests_total"}`:      false,
		`sum({__name__="http_requests_total"})`: false,

		// Rule 2: Vector selector without non-name label matchers
		`http_requests_total`:                          false,
		`some_very_high_cardinality_metric`:            false,
		`sum(http_requests_total)`:                     false, // No label matchers
		`rate(http_requests_total[5m])`:                false, // No label matchers
		`sum by (job) (rate(http_requests_total[5m]))`: false, // No label matchers

		// Rule 3: Expensive regex
		`http_requests_total{pod=~"web-.*"}`:  true,  // Has selective matcher "web-"
		`rate(my_metric{instance!~".+"}[5m])`: false, // Pure wildcard, no selective matcher
		`http_requests_total{pod=~".*"}`:      false, // Pure wildcard, no selective matcher
		`http_requests_total{job=~"api-.*"}`:  true,  // Has selective matcher "api-"

		// Complex failure cases combining rules
		`avg(rate(http_requests_total[5m]))`:                                              false, // No label matchers
		`sum by (status) (rate(http_requests_total[5m]))`:                                 false, // No label matchers
		`histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))`:        false, // No label matchers
		`avg(http_requests_total{pod=~".*"})`:                                             false, // Pure wildcard regex
		`sum by (job) (rate(http_requests_total{instance=~".+"}[5m]))`:                    false, // Pure wildcard regex
		`histogram_quantile(0.99, sum(rate(http_latency_bucket{pod=~".*"}[5m])) by (le))`: false, // Pure wildcard regex
		`rate(http_requests_total{job="api"}[5m]) / rate(http_requests_total[5m])`:        false, // Second selector has no labels
		`avg_over_time(cpu_usage[10m])`:                                                   false, // No label matchers

		`http_requests_total{job="api"}`:                          true,
		`http_requests_total{pod="web-1"}`:                        true,
		`sum by (job) (rate(http_requests_total{job="api"}[5m]))`: true,
		`histogram_quantile(0.9, sum(rate(http_request_duration_seconds_bucket{job="api"}[5m])) by (le, job))`: true,
		`up{job="prometheus"} == 0`:                true,
		`rate(http_requests_total{job="api"}[5m])`: true,
		`sum(http_requests_total{job="api"})`:      true,

		// Safe regex on high-cardinality labels
		`http_requests_total{pod=~"web-1|web-2"}`: true,
		`http_requests_total{pod=~"web-[0-9]+"}`:  true,
		`http_requests_total{job=~"api-1|api-2"}`: true,

		// Complex safe cases with aggregations and functions
		`avg(rate(http_requests_total{job="api"}[5m]))`:                                                     true,
		`sum by (status) (rate(http_requests_total{job="api"}[5m]))`:                                        true,
		`histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{job="api"}[5m]))`:               true,
		`avg by (instance) (rate(http_requests_total{job="api",status=~"5.."}[5m]))`:                        true,
		`sum(rate(http_requests_total{job="api"}[5m])) / sum(rate(http_requests_total{job="backend"}[5m]))`: true,
		`max_over_time(cpu_usage{instance="server-1"}[10m])`:                                                true,
		`topk(5, rate(http_requests_total{environment="production"}[5m]))`:                                  true,
		`histogram_quantile(0.99, sum by (le) (rate(http_latency_bucket{service=~"api-.*"}[5m])))`:          true,
		`avg(irate(node_cpu_seconds_total{mode!="idle",instance="localhost"}[5m])) by (cpu)`:                true,
	}

	for query, expectedSafe := range tests {
		t.Run(query, func(t *testing.T) {
			safe, err := g.IsSafeQuery(context.TODO(), query, nil)
			if safe != expectedSafe {
				t.Errorf("IsSafeQuery(%q) = %v (err: %v), want %v", query, safe, err, expectedSafe)
			}
			// If expected to be unsafe, we should have an error explaining why
			if !expectedSafe && err == nil {
				t.Errorf("IsSafeQuery(%q) returned unsafe but no error explaining why", query)
			}
		})
	}
}

func TestGuardrails_DisabledRules(t *testing.T) {
	t.Run("DisallowExplicitNameLabel disabled", func(t *testing.T) {
		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       true,
			DisallowBlanketRegex:      true,
		}
		safe, err := g.IsSafeQuery(context.TODO(), `{__name__="http_requests_total", job="api"}`, nil)
		if !safe {
			t.Errorf("expected query to be safe when DisallowExplicitNameLabel is disabled, got error: %v", err)
		}
	})

	t.Run("RequireLabelMatcher disabled", func(t *testing.T) {
		g := &Guardrails{
			DisallowExplicitNameLabel: true,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
		}
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total`, nil)
		if !safe {
			t.Errorf("expected query to be safe when RequireLabelMatcher is disabled, got error: %v", err)
		}
	})

	t.Run("DisallowBlanketRegex disabled", func(t *testing.T) {
		g := &Guardrails{
			DisallowExplicitNameLabel: true,
			RequireLabelMatcher:       true,
			DisallowBlanketRegex:      false,
		}
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, nil)
		if !safe {
			t.Errorf("expected query to be safe when DisallowBlanketRegex is disabled, got error: %v", err)
		}
	})
}

func TestExtractMetricNames(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "single metric",
			query:    `http_requests_total{job="api"}`,
			expected: []string{"http_requests_total"},
		},
		{
			name:     "multiple metrics",
			query:    `rate(http_requests_total{job="api"}[5m]) / rate(http_requests_total{job="backend"}[5m])`,
			expected: []string{"http_requests_total"},
		},
		{
			name:     "different metrics",
			query:    `up{job="prometheus"} + node_cpu_seconds_total{mode="idle"}`,
			expected: []string{"up", "node_cpu_seconds_total"},
		},
		{
			name:     "metric with __name__ label",
			query:    `{__name__="http_requests_total", job="api"}`,
			expected: []string{"http_requests_total"},
		},
		{
			name:     "complex query with aggregations",
			query:    `sum by (job) (rate(http_requests_total{job="api"}[5m]))`,
			expected: []string{"http_requests_total"},
		},
		{
			name:     "histogram quantile",
			query:    `histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{job="api"}[5m]))`,
			expected: []string{"http_request_duration_seconds_bucket"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractMetricNames(tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d metric names, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			// Convert to map for easier comparison (order doesn't matter)
			resultMap := make(map[string]bool)
			for _, name := range result {
				resultMap[name] = true
			}

			for _, expected := range tt.expected {
				if !resultMap[expected] {
					t.Errorf("expected metric name %q not found in result: %v", expected, result)
				}
			}
		})
	}
}

func TestExtractBlanketRegexLabels(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "single blanket regex",
			query:    `http_requests_total{pod=~".*"}`,
			expected: []string{"pod"},
		},
		{
			name:     "multiple blanket regex",
			query:    `http_requests_total{pod=~".*", instance=~".+"}`,
			expected: []string{"pod", "instance"},
		},
		{
			name:     "no blanket regex",
			query:    `http_requests_total{pod=~"web-.*", job="api"}`,
			expected: []string{},
		},
		{
			name:     "negative regex blanket",
			query:    `http_requests_total{pod!~".*"}`,
			expected: []string{"pod"},
		},
		{
			name:     "mixed regex patterns",
			query:    `http_requests_total{pod=~"web-.*", instance=~".*", job="api"}`,
			expected: []string{"instance"},
		},
		{
			name:     "complex query with blanket regex",
			query:    `sum by (job) (rate(http_requests_total{pod=~".*"}[5m]))`,
			expected: []string{"pod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractBlanketRegexLabels(tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d label names, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			// Convert to map for easier comparison (order doesn't matter)
			resultMap := make(map[string]bool)
			for _, name := range result {
				resultMap[name] = true
			}

			for _, expected := range tt.expected {
				if !resultMap[expected] {
					t.Errorf("expected label name %q not found in result: %v", expected, result)
				}
			}
		})
	}
}

func TestGuardrails_MaxLabelCardinality(t *testing.T) {
	t.Run("MaxLabelCardinality set with DisallowBlanketRegex but no client", func(t *testing.T) {
		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       100, // Set threshold
		}
		// With MaxLabelCardinality set but no client provided, blanket regex should be rejected
		// because we can't verify the cardinality
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, nil)
		if safe {
			t.Error("expected query with blanket regex to be unsafe when MaxLabelCardinality is set but no client provided")
		}
		if err == nil {
			t.Error("expected error explaining why query is unsafe")
		}
	})

	t.Run("MaxLabelCardinality 0 always disallows blanket regex", func(t *testing.T) {
		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       0, // 0 means always disallow
		}
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, nil)
		if safe {
			t.Error("expected query with blanket regex to be unsafe when MaxLabelCardinality is 0")
		}
		if err == nil {
			t.Error("expected error explaining why query is unsafe")
		}
	})

	t.Run("DisallowBlanketRegex disabled ignores MaxLabelCardinality", func(t *testing.T) {
		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      false, // Disabled
			MaxLabelCardinality:       100,
		}
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, nil)
		if !safe {
			t.Errorf("expected query to be safe when DisallowBlanketRegex is disabled, got error: %v", err)
		}
	})

	t.Run("Selective regex passes without TSDB", func(t *testing.T) {
		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       0, // 0 = blanket regex always rejected, but selective passes
		}
		// Selective regex should always pass (no blanket regex)
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~"web-.*"}`, nil)
		if !safe {
			t.Errorf("expected query with selective regex to be safe, got error: %v", err)
		}
	})
}

// mockPrometheusAPI is a mock implementation of v1.API for testing
type mockPrometheusAPI struct {
	tsdbResult       v1.TSDBResult
	availableMetrics []string
}

func (m *mockPrometheusAPI) TSDB(ctx context.Context, opts ...v1.Option) (v1.TSDBResult, error) {
	return m.tsdbResult, nil
}

// Implement remaining v1.API methods as no-ops (not used in tests)
func (m *mockPrometheusAPI) Alerts(ctx context.Context) (v1.AlertsResult, error) {
	return v1.AlertsResult{}, nil
}
func (m *mockPrometheusAPI) AlertManagers(ctx context.Context) (v1.AlertManagersResult, error) {
	return v1.AlertManagersResult{}, nil
}
func (m *mockPrometheusAPI) CleanTombstones(ctx context.Context) error { return nil }
func (m *mockPrometheusAPI) Config(ctx context.Context) (v1.ConfigResult, error) {
	return v1.ConfigResult{}, nil
}
func (m *mockPrometheusAPI) DeleteSeries(ctx context.Context, matches []string, startTime, endTime time.Time) error {
	return nil
}
func (m *mockPrometheusAPI) Flags(ctx context.Context) (v1.FlagsResult, error) {
	return v1.FlagsResult{}, nil
}
func (m *mockPrometheusAPI) LabelNames(ctx context.Context, matches []string, startTime, endTime time.Time, opts ...v1.Option) ([]string, v1.Warnings, error) {
	return nil, nil, nil
}
func (m *mockPrometheusAPI) LabelValues(ctx context.Context, label string, matches []string, startTime, endTime time.Time, opts ...v1.Option) (model.LabelValues, v1.Warnings, error) {
	// Only handle __name__ label for metric listing
	if label == "__name__" && m.availableMetrics != nil {
		result := make(model.LabelValues, len(m.availableMetrics))
		for i, metric := range m.availableMetrics {
			result[i] = model.LabelValue(metric)
		}
		return result, nil, nil
	}
	return nil, nil, nil
}
func (m *mockPrometheusAPI) Query(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	return nil, nil, nil
}
func (m *mockPrometheusAPI) QueryRange(ctx context.Context, query string, r v1.Range, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	return nil, nil, nil
}
func (m *mockPrometheusAPI) QueryExemplars(ctx context.Context, query string, startTime, endTime time.Time) ([]v1.ExemplarQueryResult, error) {
	return nil, nil
}
func (m *mockPrometheusAPI) Buildinfo(ctx context.Context) (v1.BuildinfoResult, error) {
	return v1.BuildinfoResult{}, nil
}
func (m *mockPrometheusAPI) Runtimeinfo(ctx context.Context) (v1.RuntimeinfoResult, error) {
	return v1.RuntimeinfoResult{}, nil
}
func (m *mockPrometheusAPI) Series(ctx context.Context, matches []string, startTime, endTime time.Time, opts ...v1.Option) ([]model.LabelSet, v1.Warnings, error) {
	return nil, nil, nil
}
func (m *mockPrometheusAPI) Snapshot(ctx context.Context, skipHead bool) (v1.SnapshotResult, error) {
	return v1.SnapshotResult{}, nil
}
func (m *mockPrometheusAPI) Rules(ctx context.Context) (v1.RulesResult, error) {
	return v1.RulesResult{}, nil
}
func (m *mockPrometheusAPI) Targets(ctx context.Context) (v1.TargetsResult, error) {
	return v1.TargetsResult{}, nil
}
func (m *mockPrometheusAPI) TargetsMetadata(ctx context.Context, matchTarget, metric, limit string) ([]v1.MetricMetadata, error) {
	return nil, nil
}
func (m *mockPrometheusAPI) Metadata(ctx context.Context, metric, limit string) (map[string][]v1.Metadata, error) {
	return nil, nil
}
func (m *mockPrometheusAPI) WalReplay(ctx context.Context) (v1.WalReplayStatus, error) {
	return v1.WalReplayStatus{}, nil
}

func TestGuardrails_MaxLabelCardinalityWithMockedTSDB(t *testing.T) {
	t.Run("Label cardinality below threshold allows blanket regex", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "pod", Value: 50},      // Below threshold
					{Name: "instance", Value: 80}, // Below threshold
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       100, // Threshold
		}

		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, mock)
		if !safe {
			t.Errorf("expected query to be safe when label cardinality is below threshold, got error: %v", err)
		}
	})

	t.Run("Label cardinality above threshold disallows blanket regex", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "pod", Value: 150},     // Above threshold
					{Name: "instance", Value: 80}, // Below threshold
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       100, // Threshold
		}

		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, mock)
		if safe {
			t.Error("expected query to be unsafe when label cardinality is above threshold")
		}
		if err == nil {
			t.Error("expected error explaining why query is unsafe")
		}
	})

	t.Run("Multiple blanket regex labels - all below threshold", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "pod", Value: 50},
					{Name: "instance", Value: 80},
					{Name: "job", Value: 30},
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       100,
		}

		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*", instance=~".+"}`, mock)
		if !safe {
			t.Errorf("expected query to be safe when all label cardinalities are below threshold, got error: %v", err)
		}
	})

	t.Run("Multiple blanket regex labels - one above threshold", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "pod", Value: 50},       // Below threshold
					{Name: "instance", Value: 150}, // Above threshold
					{Name: "job", Value: 30},       // Below threshold
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       100,
		}

		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*", instance=~".+"}`, mock)
		if safe {
			t.Error("expected query to be unsafe when any label cardinality is above threshold")
		}
		if err == nil {
			t.Error("expected error explaining why query is unsafe")
		}
	})

	t.Run("Label not in TSDB result allows blanket regex", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "instance", Value: 80},
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       100,
		}

		// pod label not in TSDB result, should be allowed
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, mock)
		if !safe {
			t.Errorf("expected query to be safe when label not found in TSDB result, got error: %v", err)
		}
	})

	t.Run("Mixed regex patterns - only blanket regex checked", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "pod", Value: 150},      // Above threshold but uses selective regex
					{Name: "instance", Value: 200}, // Above threshold and uses blanket regex
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       100,
		}

		// pod uses selective regex (web-.*), instance uses blanket regex (.*)
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~"web-.*", instance=~".*"}`, mock)
		if safe {
			t.Error("expected query to be unsafe because instance has blanket regex above threshold")
		}
		if err == nil {
			t.Error("expected error explaining why query is unsafe")
		}
	})

	t.Run("Negative blanket regex also checked", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "pod", Value: 150}, // Above threshold
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			MaxLabelCardinality:       100,
		}

		// Negative blanket regex (pod!~".*") should also be checked
		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod!~".*"}`, mock)
		if safe {
			t.Error("expected query to be unsafe for negative blanket regex above threshold")
		}
		if err == nil {
			t.Error("expected error explaining why query is unsafe")
		}
	})

	t.Run("Combined with MaxMetricCardinality check", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				SeriesCountByMetricName: []v1.Stat{
					{Name: "http_requests_total", Value: 5000}, // Below metric threshold
				},
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "pod", Value: 50}, // Below label threshold
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			ForceMaxMetricCardinality: true,
			MaxMetricCardinality:      10000, // Metric threshold
			MaxLabelCardinality:       100,   // Label threshold
		}

		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, mock)
		if !safe {
			t.Errorf("expected query to be safe when both metric and label cardinality are below thresholds, got error: %v", err)
		}
	})

	t.Run("Combined check - metric above threshold", func(t *testing.T) {
		mock := &mockPrometheusAPI{
			tsdbResult: v1.TSDBResult{
				SeriesCountByMetricName: []v1.Stat{
					{Name: "http_requests_total", Value: 15000}, // Above metric threshold
				},
				LabelValueCountByLabelName: []v1.Stat{
					{Name: "pod", Value: 50}, // Below label threshold
				},
			},
		}

		g := &Guardrails{
			DisallowExplicitNameLabel: false,
			RequireLabelMatcher:       false,
			DisallowBlanketRegex:      true,
			ForceMaxMetricCardinality: true,
			MaxMetricCardinality:      10000,
			MaxLabelCardinality:       100,
		}

		safe, err := g.IsSafeQuery(context.TODO(), `http_requests_total{pod=~".*"}`, mock)
		if safe {
			t.Error("expected query to be unsafe when metric cardinality is above threshold")
		}
		if err == nil {
			t.Error("expected error explaining why query is unsafe")
		}
	})
}
