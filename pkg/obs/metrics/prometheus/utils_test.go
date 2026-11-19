package prometheus

import (
	"testing"
	"time"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectError  bool
		validateTime func(t *testing.T, result time.Time)
	}{
		{
			name:        "NOW keyword uppercase",
			input:       "NOW",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				if time.Since(result) > 2*time.Second {
					t.Errorf("expected result to be approximately now, got %v ago", time.Since(result))
				}
			},
		},
		{
			name:        "NOW keyword lowercase",
			input:       "now",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				if time.Since(result) > 2*time.Second {
					t.Errorf("expected result to be approximately now, got %v ago", time.Since(result))
				}
			},
		},
		{
			name:        "NOW-5m (5 minutes ago)",
			input:       "NOW-5m",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				expected := time.Now().Add(-5 * time.Minute)
				diff := result.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected result to be ~5 minutes ago, got %v (diff: %v)", result, diff)
				}
			},
		},
		{
			name:        "NOW-1h (1 hour ago)",
			input:       "NOW-1h",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				expected := time.Now().Add(-1 * time.Hour)
				diff := result.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected result to be ~1 hour ago, got %v (diff: %v)", result, diff)
				}
			},
		},
		{
			name:        "NOW-30s (30 seconds ago)",
			input:       "NOW-30s",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				expected := time.Now().Add(-30 * time.Second)
				diff := result.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected result to be ~30 seconds ago, got %v (diff: %v)", result, diff)
				}
			},
		},
		{
			name:        "NOW+5m (5 minutes from now)",
			input:       "NOW+5m",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				expected := time.Now().Add(5 * time.Minute)
				diff := result.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected result to be ~5 minutes from now, got %v (diff: %v)", result, diff)
				}
			},
		},
		{
			name:        "NOW+1h (1 hour from now)",
			input:       "NOW+1h",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				expected := time.Now().Add(1 * time.Hour)
				diff := result.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected result to be ~1 hour from now, got %v (diff: %v)", result, diff)
				}
			},
		},
		{
			name:        "now-15m (lowercase with relative time)",
			input:       "now-15m",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				expected := time.Now().Add(-15 * time.Minute)
				diff := result.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("expected result to be ~15 minutes ago, got %v (diff: %v)", result, diff)
				}
			},
		},
		{
			name:        "RFC3339 timestamp",
			input:       "2024-01-01T00:00:00Z",
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				expected, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
				if !result.Equal(expected) {
					t.Errorf("expected %v, got %v", expected, result)
				}
			},
		},
		{
			name:        "Unix timestamp",
			input:       "1704067200", // 2024-01-01T00:00:00Z
			expectError: false,
			validateTime: func(t *testing.T, result time.Time) {
				expected := time.Unix(1704067200, 0)
				if !result.Equal(expected) {
					t.Errorf("expected %v, got %v", expected, result)
				}
			},
		},
		{
			name:        "invalid duration in relative time",
			input:       "NOW-invalid",
			expectError: true,
		},
		{
			name:        "invalid format",
			input:       "not-a-timestamp",
			expectError: true,
		},
		{
			name:        "NOW with no operator",
			input:       "NOW5m",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTimestamp(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validateTime != nil {
				tt.validateTime(t, result)
			}
		})
	}
}
