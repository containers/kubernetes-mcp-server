package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/prometheus/alertmanager/api/v2/models"

	"github.com/rhobs/obs-mcp/pkg/metrics/alertmanager"
	"github.com/rhobs/obs-mcp/pkg/metrics/prometheus"
)

// MockedLoader is a mock implementation of prometheus.PromClient for testing
type MockedLoader struct {
	ListMetricsFunc         func(ctx context.Context, nameRegex string) ([]string, error)
	ExecuteRangeQueryFunc   func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error)
	ExecuteInstantQueryFunc func(ctx context.Context, query string, time time.Time) (map[string]any, error)
	GetLabelNamesFunc       func(ctx context.Context, metricName string, start, end time.Time) ([]string, error)
	GetLabelValuesFunc      func(ctx context.Context, label string, metricName string, start, end time.Time) ([]string, error)
	GetSeriesFunc           func(ctx context.Context, matches []string, start, end time.Time) ([]map[string]string, error)
}

func (m *MockedLoader) ListMetrics(ctx context.Context, nameRegex string) ([]string, error) {
	if m.ListMetricsFunc != nil {
		return m.ListMetricsFunc(ctx, nameRegex)
	}
	return []string{}, nil
}

func (m *MockedLoader) ExecuteRangeQuery(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
	if m.ExecuteRangeQueryFunc != nil {
		return m.ExecuteRangeQueryFunc(ctx, query, start, end, step)
	}
	return map[string]any{
		"resultType": "matrix",
		"result":     []any{},
	}, nil
}

func (m *MockedLoader) ExecuteInstantQuery(ctx context.Context, query string, ts time.Time) (map[string]any, error) {
	if m.ExecuteInstantQueryFunc != nil {
		return m.ExecuteInstantQueryFunc(ctx, query, ts)
	}
	return map[string]any{
		"resultType": "vector",
		"result":     []any{},
	}, nil
}

func (m *MockedLoader) GetLabelNames(ctx context.Context, metricName string, start, end time.Time) ([]string, error) {
	if m.GetLabelNamesFunc != nil {
		return m.GetLabelNamesFunc(ctx, metricName, start, end)
	}
	return []string{}, nil
}

func (m *MockedLoader) GetLabelValues(ctx context.Context, label, metricName string, start, end time.Time) ([]string, error) {
	if m.GetLabelValuesFunc != nil {
		return m.GetLabelValuesFunc(ctx, label, metricName, start, end)
	}
	return []string{}, nil
}

func (m *MockedLoader) GetSeries(ctx context.Context, matches []string, start, end time.Time) ([]map[string]string, error) {
	if m.GetSeriesFunc != nil {
		return m.GetSeriesFunc(ctx, matches, start, end)
	}
	return []map[string]string{}, nil
}

// Ensure MockPromClient implements prometheus.PromClient at compile time
var _ prometheus.Loader = (*MockedLoader)(nil)

// MockedAlertmanagerLoader is a mock implementation of alertmanager.Loader for testing
type MockedAlertmanagerLoader struct {
	GetAlertsFunc   func(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filter []string, receiver string) (models.GettableAlerts, error)
	GetSilencesFunc func(ctx context.Context, filter []string) (models.GettableSilences, error)
}

func (m *MockedAlertmanagerLoader) GetAlerts(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filter []string, receiver string) (models.GettableAlerts, error) {
	if m.GetAlertsFunc != nil {
		return m.GetAlertsFunc(ctx, active, silenced, inhibited, unprocessed, filter, receiver)
	}
	return models.GettableAlerts{}, nil
}

func (m *MockedAlertmanagerLoader) GetSilences(ctx context.Context, filter []string) (models.GettableSilences, error) {
	if m.GetSilencesFunc != nil {
		return m.GetSilencesFunc(ctx, filter)
	}
	return models.GettableSilences{}, nil
}

// Ensure MockedAlertmanagerLoader implements alertmanager.Loader at compile time
var _ alertmanager.Loader = (*MockedAlertmanagerLoader)(nil)

func TestExecuteRangeQueryHandler_ExplicitTimeRange_RFC3339(t *testing.T) {
	expectedStart, _ := prometheus.ParseTimestamp("2024-01-01T00:00:00Z")
	expectedEnd, _ := prometheus.ParseTimestamp("2024-01-01T01:00:00Z")

	mockClient := &MockedLoader{
		ExecuteRangeQueryFunc: func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
			if query != "up{job=\"api\"}" {
				t.Errorf("expected query 'up{job=\"api\"}', got %q", query)
			}
			if step != time.Minute {
				t.Errorf("expected step 1m, got %v", step)
			}
			if !start.Equal(expectedStart) {
				t.Errorf("expected start %v, got %v", expectedStart, start)
			}
			if !end.Equal(expectedEnd) {
				t.Errorf("expected end %v, got %v", expectedEnd, end)
			}
			return map[string]any{"resultType": "matrix", "result": []any{}}, nil
		},
	}

	input := BuildRangeQueryInput(map[string]any{
		"query": "up{job=\"api\"}",
		"step":  "1m",
		"start": "2024-01-01T00:00:00Z",
		"end":   "2024-01-01T01:00:00Z",
	})
	result := ExecuteRangeQueryHandler(context.Background(), mockClient, input, false)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestExecuteRangeQueryHandler_StepParsing_ValidSteps(t *testing.T) {
	mockClient := &MockedLoader{
		ExecuteRangeQueryFunc: func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
			return map[string]any{"resultType": "matrix", "result": []any{}}, nil
		},
	}

	for _, step := range []string{"30s", "5m", "1h"} {
		t.Run(step, func(t *testing.T) {
			input := BuildRangeQueryInput(map[string]any{
				"query": "up{job=\"api\"}",
				"step":  step,
			})
			result := ExecuteRangeQueryHandler(t.Context(), mockClient, input, false)
			if result.Error != nil {
				t.Fatalf("%s step failed: %v", step, result.Error)
			}
		})
	}
}

func TestExecuteRangeQueryHandler_RequiredParameters(t *testing.T) {
	tests := []struct {
		name          string
		params        map[string]any
		expectedError string
	}{
		{
			name:          "missing query parameter",
			params:        map[string]any{"step": "1m"},
			expectedError: "query parameter is required and must be a string",
		},
		{
			name:          "missing step parameter",
			params:        map[string]any{"query": "up{job=\"api\"}"},
			expectedError: "step parameter is required and must be a string",
		},
		{
			name: "invalid step format",
			params: map[string]any{
				"query": "up{job=\"api\"}",
				"step":  "invalid",
			},
			expectedError: "invalid step format: not a valid duration string: \"invalid\"",
		},
		{
			name: "start without end",
			params: map[string]any{
				"query": "up{job=\"api\"}",
				"step":  "1m",
				"start": "2024-01-01T00:00:00Z",
			},
			expectedError: "both start and end must be provided together",
		},
		{
			name: "end without start",
			params: map[string]any{
				"query": "up{job=\"api\"}",
				"step":  "1m",
				"end":   "2024-01-01T00:00:00Z",
			},
			expectedError: "both start and end must be provided together",
		},
		{
			name: "invalid start time",
			params: map[string]any{
				"query": "up{job=\"api\"}",
				"step":  "1m",
				"start": "invalid",
				"end":   "2024-01-01T01:00:00Z",
			},
			expectedError: "invalid start time format: timestamp must be RFC3339 format, Unix timestamp, NOW, or relative time (NOW±duration)",
		},
		{
			name: "invalid end time",
			params: map[string]any{
				"query": "up{job=\"api\"}",
				"step":  "1m",
				"start": "2024-01-01T00:00:00Z",
				"end":   "invalid",
			},
			expectedError: "invalid end time format: timestamp must be RFC3339 format, Unix timestamp, NOW, or relative time (NOW±duration)",
		},
		{
			name: "invalid duration",
			params: map[string]any{
				"query":    "up{job=\"api\"}",
				"step":     "1m",
				"duration": "invalid",
			},
			expectedError: "invalid duration format: not a valid duration string: \"invalid\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockedLoader{}
			input := BuildRangeQueryInput(tt.params)
			result := ExecuteRangeQueryHandler(t.Context(), mockClient, input, false)

			if result.Error == nil {
				t.Errorf("expected error %q, got nil", tt.expectedError)
			} else if result.Error.Error() != tt.expectedError {
				t.Errorf("expected error %q, got %q", tt.expectedError, result.Error.Error())
			}
		})
	}
}

func TestExecuteRangeQueryHandler_DurationMode_DefaultOneHour(t *testing.T) {
	mockClient := &MockedLoader{
		ExecuteRangeQueryFunc: func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
			if query != "up{job=\"api\"}" {
				t.Errorf("expected query 'up{job=\"api\"}', got %q", query)
			}
			if step != time.Minute {
				t.Errorf("expected step 1m, got %v", step)
			}
			duration := end.Sub(start)
			if duration < 59*time.Minute || duration > 61*time.Minute {
				t.Errorf("expected duration ~1h, got %v", duration)
			}
			if time.Since(end) > 2*time.Second {
				t.Errorf("expected end to be approximately now, got %v ago", time.Since(end))
			}
			return map[string]any{"resultType": "matrix", "result": []any{}}, nil
		},
	}

	input := BuildRangeQueryInput(map[string]any{
		"query": "up{job=\"api\"}",
		"step":  "1m",
	})
	result := ExecuteRangeQueryHandler(t.Context(), mockClient, input, false)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestExecuteRangeQueryHandler_DurationMode_CustomDuration(t *testing.T) {
	mockClient := &MockedLoader{
		ExecuteRangeQueryFunc: func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
			if query != "rate(http_requests_total{job=\"api\"}[5m])" {
				t.Errorf("expected query 'rate(http_requests_total{job=\"api\"}[5m])', got %q", query)
			}
			if step != 30*time.Second {
				t.Errorf("expected step 30s, got %v", step)
			}
			duration := end.Sub(start)
			if duration < 29*time.Minute || duration > 31*time.Minute {
				t.Errorf("expected duration ~30m, got %v", duration)
			}
			return map[string]any{"resultType": "matrix", "result": []any{}}, nil
		},
	}

	input := BuildRangeQueryInput(map[string]any{
		"query":    "rate(http_requests_total{job=\"api\"}[5m])",
		"step":     "30s",
		"duration": "30m",
	})
	result := ExecuteRangeQueryHandler(t.Context(), mockClient, input, false)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestExecuteRangeQueryHandler_DurationMode_NOWKeyword(t *testing.T) {
	mockClient := &MockedLoader{
		ExecuteRangeQueryFunc: func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
			duration := end.Sub(start)
			if duration < 59*time.Minute || duration > 61*time.Minute {
				t.Errorf("expected duration ~1h when using duration mode, got %v", duration)
			}
			return map[string]any{"resultType": "matrix", "result": []any{}}, nil
		},
	}

	input := BuildRangeQueryInput(map[string]any{
		"query":    "up{job=\"api\"}",
		"step":     "1m",
		"duration": "1h",
	})
	result := ExecuteRangeQueryHandler(t.Context(), mockClient, input, false)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestExecuteRangeQueryHandler_NOWKeyword_CaseInsensitive(t *testing.T) {
	nowVariations := []string{"NOW", "now", "Now", "nOw", "NoW"}
	expectedStart, _ := prometheus.ParseTimestamp("2024-01-01T00:00:00Z")

	for _, nowStr := range nowVariations {
		t.Run(nowStr, func(t *testing.T) {
			mockClient := &MockedLoader{
				ExecuteRangeQueryFunc: func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
					if time.Since(end) > 2*time.Second {
						t.Errorf("expected end to be approximately now, got %v ago", time.Since(end))
					}
					if !start.Equal(expectedStart) {
						t.Errorf("expected start %v, got %v", expectedStart, start)
					}
					return map[string]any{"resultType": "matrix", "result": []any{}}, nil
				},
			}

			input := BuildRangeQueryInput(map[string]any{
				"query": "up{job=\"api\"}",
				"step":  "1m",
				"start": "2024-01-01T00:00:00Z",
				"end":   nowStr,
			})
			result := ExecuteRangeQueryHandler(context.Background(), mockClient, input, false)
			if result.Error != nil {
				t.Fatalf("unexpected error with %q: %v", nowStr, result.Error)
			}
		})
	}
}

func TestExecuteInstantQueryHandler_NOWKeyword_CaseInsensitive(t *testing.T) {
	nowVariations := []string{"NOW", "now", "Now", "nOw", "NoW"}

	for _, nowStr := range nowVariations {
		t.Run(nowStr, func(t *testing.T) {
			mockClient := &MockedLoader{
				ExecuteInstantQueryFunc: func(ctx context.Context, query string, queryTime time.Time) (map[string]any, error) {
					if time.Since(queryTime) > 2*time.Second {
						t.Errorf("expected query time to be approximately now, got %v ago", time.Since(queryTime))
					}
					return map[string]any{"resultType": "vector", "result": []any{}}, nil
				},
			}

			input := BuildInstantQueryInput(map[string]any{
				"query": "up{job=\"api\"}",
				"time":  nowStr,
			})
			result := ExecuteInstantQueryHandler(context.Background(), mockClient, input)
			if result.Error != nil {
				t.Fatalf("unexpected error with %q: %v", nowStr, result.Error)
			}
		})
	}
}

func TestExecuteRangeQueryHandler_RelativeTime(t *testing.T) {
	tests := []struct {
		name       string
		start      string
		end        string
		validateFn func(t *testing.T, start, end time.Time)
	}{
		{
			name:  "NOW-5m to NOW",
			start: "NOW-5m",
			end:   "NOW",
			validateFn: func(t *testing.T, start, end time.Time) {
				duration := end.Sub(start)
				expectedDuration := 5 * time.Minute
				diff := duration - expectedDuration
				if diff.Abs() > 2*time.Second {
					t.Errorf("expected duration ~5m, got %v (diff: %v)", duration, diff)
				}
				if time.Since(end) > 2*time.Second {
					t.Errorf("expected end to be approximately now, got %v ago", time.Since(end))
				}
			},
		},
		{
			name:  "NOW-1h to NOW-30m",
			start: "NOW-1h",
			end:   "NOW-30m",
			validateFn: func(t *testing.T, start, end time.Time) {
				duration := end.Sub(start)
				expectedDuration := 30 * time.Minute
				diff := duration - expectedDuration
				if diff.Abs() > 2*time.Second {
					t.Errorf("expected duration ~30m, got %v (diff: %v)", duration, diff)
				}
				expectedEnd := time.Now().Add(-30 * time.Minute)
				if end.Sub(expectedEnd).Abs() > 2*time.Second {
					t.Errorf("expected end to be ~30m ago, got %v", time.Since(end))
				}
			},
		},
		{
			name:  "now-15m to now (lowercase)",
			start: "now-15m",
			end:   "now",
			validateFn: func(t *testing.T, start, end time.Time) {
				duration := end.Sub(start)
				expectedDuration := 15 * time.Minute
				diff := duration - expectedDuration
				if diff.Abs() > 2*time.Second {
					t.Errorf("expected duration ~15m, got %v (diff: %v)", duration, diff)
				}
			},
		},
		{
			name:  "RFC3339 to NOW-5m",
			start: "2024-01-01T00:00:00Z",
			end:   "NOW-5m",
			validateFn: func(t *testing.T, start, end time.Time) {
				expectedStart, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
				if !start.Equal(expectedStart) {
					t.Errorf("expected start %v, got %v", expectedStart, start)
				}
				expectedEnd := time.Now().Add(-5 * time.Minute)
				if end.Sub(expectedEnd).Abs() > 2*time.Second {
					t.Errorf("expected end to be ~5m ago, got %v", end)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockedLoader{
				ExecuteRangeQueryFunc: func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
					if tt.validateFn != nil {
						tt.validateFn(t, start, end)
					}
					return map[string]any{"resultType": "matrix", "result": []any{}}, nil
				},
			}

			input := BuildRangeQueryInput(map[string]any{
				"query": "up{job=\"api\"}",
				"step":  "1m",
				"start": tt.start,
				"end":   tt.end,
			})
			result := ExecuteRangeQueryHandler(context.Background(), mockClient, input, false)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
		})
	}
}

func TestShowTimeseriesHandler(t *testing.T) {
	queryCalled := false
	mockClient := &MockedLoader{
		ExecuteRangeQueryFunc: func(ctx context.Context, query string, start, end time.Time, step time.Duration) (map[string]any, error) {
			queryCalled = true
			if query != "up{job=\"api\"}" {
				t.Errorf("expected query 'up{job=\"api\"}', got %q", query)
			}
			return map[string]any{"resultType": "matrix", "result": []any{}}, nil
		},
	}

	input := BuildShowTimeseriesInput(map[string]any{
		"query": "up{job=\"api\"}",
		"step":  "1m",
		"start": "2024-01-01T00:00:00Z",
		"end":   "2024-01-01T01:00:00Z",
	})
	result := ShowTimeseriesHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !queryCalled {
		t.Fatal("expected range query to be executed")
	}
}

func TestExecuteInstantQueryHandler_RelativeTime(t *testing.T) {
	tests := []struct {
		name       string
		time       string
		validateFn func(t *testing.T, queryTime time.Time)
	}{
		{
			name: "NOW-5m",
			time: "NOW-5m",
			validateFn: func(t *testing.T, queryTime time.Time) {
				expected := time.Now().Add(-5 * time.Minute)
				diff := queryTime.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected query time to be ~5m ago, got %v (diff: %v)", queryTime, diff)
				}
			},
		},
		{
			name: "NOW-1h",
			time: "NOW-1h",
			validateFn: func(t *testing.T, queryTime time.Time) {
				expected := time.Now().Add(-1 * time.Hour)
				diff := queryTime.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected query time to be ~1h ago, got %v (diff: %v)", queryTime, diff)
				}
			},
		},
		{
			name: "now-30s (lowercase)",
			time: "now-30s",
			validateFn: func(t *testing.T, queryTime time.Time) {
				expected := time.Now().Add(-30 * time.Second)
				diff := queryTime.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected query time to be ~30s ago, got %v (diff: %v)", queryTime, diff)
				}
			},
		},
		{
			name: "NOW+5m (future time)",
			time: "NOW+5m",
			validateFn: func(t *testing.T, queryTime time.Time) {
				expected := time.Now().Add(5 * time.Minute)
				diff := queryTime.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected query time to be ~5m from now, got %v (diff: %v)", queryTime, diff)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockedLoader{
				ExecuteInstantQueryFunc: func(ctx context.Context, query string, queryTime time.Time) (map[string]any, error) {
					if tt.validateFn != nil {
						tt.validateFn(t, queryTime)
					}
					return map[string]any{"resultType": "vector", "result": []any{}}, nil
				},
			}

			input := BuildInstantQueryInput(map[string]any{
				"query": "up{job=\"api\"}",
				"time":  tt.time,
			})
			result := ExecuteInstantQueryHandler(context.Background(), mockClient, input)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
		})
	}
}

func TestGetAlertsHandler_AllAlerts(t *testing.T) {
	activeState := "active"
	now := strfmt.DateTime(time.Now())
	mockClient := &MockedAlertmanagerLoader{
		GetAlertsFunc: func(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filter []string, receiver string) (models.GettableAlerts, error) {
			if active != nil || silenced != nil || inhibited != nil || unprocessed != nil {
				t.Error("expected no boolean filters")
			}
			if len(filter) != 0 {
				t.Errorf("expected no filters, got %v", filter)
			}
			if receiver != "" {
				t.Errorf("expected no receiver, got %s", receiver)
			}

			return models.GettableAlerts{
				&models.GettableAlert{
					Alert: models.Alert{
						Labels: models.LabelSet{
							"alertname": "HighCPU",
							"severity":  "warning",
						},
					},
					Annotations: models.LabelSet{
						"description": "CPU usage is high",
					},
					StartsAt: &now,
					EndsAt:   &now,
					Status: &models.AlertStatus{
						State:       &activeState,
						SilencedBy:  []string{},
						InhibitedBy: []string{},
					},
				},
			}, nil
		},
	}

	input := BuildAlertsInput(map[string]any{})
	result := GetAlertsHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetAlertsHandler_WithActiveFilter(t *testing.T) {
	active := true
	activeState := "active"
	now := strfmt.DateTime(time.Now())

	mockClient := &MockedAlertmanagerLoader{
		GetAlertsFunc: func(ctx context.Context, activeParam, silenced, inhibited, unprocessed *bool, filter []string, receiver string) (models.GettableAlerts, error) {
			if activeParam == nil || !*activeParam {
				t.Error("expected active parameter to be true")
			}

			return models.GettableAlerts{
				&models.GettableAlert{
					Alert: models.Alert{
						Labels: models.LabelSet{"alertname": "HighCPU"},
					},
					Annotations: models.LabelSet{},
					StartsAt:    &now,
					EndsAt:      &now,
					Status: &models.AlertStatus{
						State:       &activeState,
						SilencedBy:  []string{},
						InhibitedBy: []string{},
					},
				},
			}, nil
		},
	}

	input := BuildAlertsInput(map[string]any{"active": active})
	result := GetAlertsHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetAlertsHandler_WithFilter(t *testing.T) {
	activeState := "active"
	now := strfmt.DateTime(time.Now())

	mockClient := &MockedAlertmanagerLoader{
		GetAlertsFunc: func(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filterParam []string, receiver string) (models.GettableAlerts, error) {
			if len(filterParam) != 1 || filterParam[0] != "alertname=HighCPU" {
				t.Errorf("expected filter 'alertname=HighCPU', got %v", filterParam)
			}

			return models.GettableAlerts{
				&models.GettableAlert{
					Alert: models.Alert{
						Labels: models.LabelSet{"alertname": "HighCPU"},
					},
					Annotations: models.LabelSet{},
					StartsAt:    &now,
					EndsAt:      &now,
					Status: &models.AlertStatus{
						State:       &activeState,
						SilencedBy:  []string{},
						InhibitedBy: []string{},
					},
				},
			}, nil
		},
	}

	input := BuildAlertsInput(map[string]any{"filter": "alertname=HighCPU"})
	result := GetAlertsHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetAlertsHandler_WithReceiver(t *testing.T) {
	activeState := "active"
	now := strfmt.DateTime(time.Now())

	mockClient := &MockedAlertmanagerLoader{
		GetAlertsFunc: func(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filter []string, receiverParam string) (models.GettableAlerts, error) {
			if receiverParam != "team-notifications" {
				t.Errorf("expected receiver 'team-notifications', got %s", receiverParam)
			}

			return models.GettableAlerts{
				&models.GettableAlert{
					Alert: models.Alert{
						Labels: models.LabelSet{"alertname": "HighCPU"},
					},
					Annotations: models.LabelSet{},
					StartsAt:    &now,
					EndsAt:      &now,
					Status: &models.AlertStatus{
						State:       &activeState,
						SilencedBy:  []string{},
						InhibitedBy: []string{},
					},
				},
			}, nil
		},
	}

	input := BuildAlertsInput(map[string]any{"receiver": "team-notifications"})
	result := GetAlertsHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetSilencesHandler_AllSilences(t *testing.T) {
	silenceID := "test-silence-id"
	silenceState := "active"
	now := strfmt.DateTime(time.Now())

	mockClient := &MockedAlertmanagerLoader{
		GetSilencesFunc: func(ctx context.Context, filter []string) (models.GettableSilences, error) {
			if len(filter) != 0 {
				t.Errorf("expected no filters, got %v", filter)
			}

			return models.GettableSilences{
				&models.GettableSilence{
					ID: &silenceID,
					Status: &models.SilenceStatus{
						State: &silenceState,
					},
					Silence: models.Silence{
						Matchers: models.Matchers{
							&models.Matcher{
								Name:    new("alertname"),
								Value:   new("HighCPU"),
								IsRegex: new(false),
								IsEqual: new(true),
							},
						},
						StartsAt:  &now,
						EndsAt:    &now,
						CreatedBy: new("admin"),
						Comment:   new("Maintenance window"),
					},
				},
			}, nil
		},
	}

	input := BuildSilencesInput(map[string]any{})
	result := GetSilencesHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetSilencesHandler_WithFilter(t *testing.T) {
	silenceID := "test-silence-id"
	silenceState := "active"
	now := strfmt.DateTime(time.Now())

	mockClient := &MockedAlertmanagerLoader{
		GetSilencesFunc: func(ctx context.Context, filterParam []string) (models.GettableSilences, error) {
			if len(filterParam) != 1 || filterParam[0] != "alertname=HighCPU" {
				t.Errorf("expected filter 'alertname=HighCPU', got %v", filterParam)
			}

			return models.GettableSilences{
				&models.GettableSilence{
					ID: &silenceID,
					Status: &models.SilenceStatus{
						State: &silenceState,
					},
					Silence: models.Silence{
						Matchers: models.Matchers{
							&models.Matcher{
								Name:    new("alertname"),
								Value:   new("HighCPU"),
								IsRegex: new(false),
								IsEqual: new(true),
							},
						},
						StartsAt:  &now,
						EndsAt:    &now,
						CreatedBy: new("admin"),
						Comment:   new("Planned maintenance"),
					},
				},
			}, nil
		},
	}

	input := BuildSilencesInput(map[string]any{"filter": "alertname=HighCPU"})
	result := GetSilencesHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetSilencesHandler_EmptyResult(t *testing.T) {
	mockClient := &MockedAlertmanagerLoader{
		GetSilencesFunc: func(ctx context.Context, filter []string) (models.GettableSilences, error) {
			return models.GettableSilences{}, nil
		},
	}

	input := BuildSilencesInput(map[string]any{})
	result := GetSilencesHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetAlertsHandler_ClientError(t *testing.T) {
	mockClient := &MockedAlertmanagerLoader{
		GetAlertsFunc: func(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filter []string, receiver string) (models.GettableAlerts, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	input := BuildAlertsInput(map[string]any{})
	result := GetAlertsHandler(context.Background(), mockClient, input)
	if result.Error == nil {
		t.Fatal("expected error result, got success")
	}
	if result.Error.Error() != "failed to get alerts: connection refused" {
		t.Errorf("expected error message 'failed to get alerts: connection refused', got %q", result.Error.Error())
	}
}

func TestGetAlertsHandler_WithMultipleFilters(t *testing.T) {
	activeState := "active"
	now := strfmt.DateTime(time.Now())

	mockClient := &MockedAlertmanagerLoader{
		GetAlertsFunc: func(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filterParam []string, receiver string) (models.GettableAlerts, error) {
			expectedFilters := []string{"alertname=HighCPU", "severity=critical"}
			if len(filterParam) != len(expectedFilters) {
				t.Errorf("expected %d filters, got %d: %v", len(expectedFilters), len(filterParam), filterParam)
			}
			for i, expected := range expectedFilters {
				if i < len(filterParam) && filterParam[i] != expected {
					t.Errorf("expected filter[%d] to be %q, got %q", i, expected, filterParam[i])
				}
			}

			return models.GettableAlerts{
				&models.GettableAlert{
					Alert: models.Alert{
						Labels: models.LabelSet{"alertname": "HighCPU", "severity": "critical"},
					},
					Annotations: models.LabelSet{},
					StartsAt:    &now,
					EndsAt:      &now,
					Status: &models.AlertStatus{
						State:       &activeState,
						SilencedBy:  []string{},
						InhibitedBy: []string{},
					},
				},
			}, nil
		},
	}

	input := BuildAlertsInput(map[string]any{"filter": "alertname=HighCPU, severity=critical"})
	result := GetAlertsHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetAlertsHandler_WithMultipleFiltersNoSpaces(t *testing.T) {
	activeState := "active"
	now := strfmt.DateTime(time.Now())

	mockClient := &MockedAlertmanagerLoader{
		GetAlertsFunc: func(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filterParam []string, receiver string) (models.GettableAlerts, error) {
			expectedFilters := []string{"alertname=HighCPU", "severity=warning", "job=api"}
			if len(filterParam) != len(expectedFilters) {
				t.Errorf("expected %d filters, got %d: %v", len(expectedFilters), len(filterParam), filterParam)
			}
			for i, expected := range expectedFilters {
				if i < len(filterParam) && filterParam[i] != expected {
					t.Errorf("expected filter[%d] to be %q, got %q", i, expected, filterParam[i])
				}
			}

			return models.GettableAlerts{
				&models.GettableAlert{
					Alert: models.Alert{
						Labels: models.LabelSet{"alertname": "HighCPU"},
					},
					Annotations: models.LabelSet{},
					StartsAt:    &now,
					EndsAt:      &now,
					Status: &models.AlertStatus{
						State:       &activeState,
						SilencedBy:  []string{},
						InhibitedBy: []string{},
					},
				},
			}, nil
		},
	}

	input := BuildAlertsInput(map[string]any{"filter": "alertname=HighCPU,severity=warning,job=api"})
	result := GetAlertsHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetAlertsHandler_EmptyResult(t *testing.T) {
	mockClient := &MockedAlertmanagerLoader{
		GetAlertsFunc: func(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filter []string, receiver string) (models.GettableAlerts, error) {
			return models.GettableAlerts{}, nil
		},
	}

	input := BuildAlertsInput(map[string]any{})
	result := GetAlertsHandler(context.Background(), mockClient, input)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestGetSilencesHandler_ClientError(t *testing.T) {
	mockClient := &MockedAlertmanagerLoader{
		GetSilencesFunc: func(ctx context.Context, filter []string) (models.GettableSilences, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	input := BuildSilencesInput(map[string]any{})
	result := GetSilencesHandler(context.Background(), mockClient, input)
	if result.Error == nil {
		t.Fatal("expected error result, got success")
	}
	if result.Error.Error() != "failed to get silences: connection refused" {
		t.Errorf("expected error message 'failed to get silences: connection refused', got %q", result.Error.Error())
	}
}
