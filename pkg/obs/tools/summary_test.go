package tools

import (
	"math"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

// TestCalculateSeriesSummary tests the complete summary calculation including min, max, avg, count
func TestCalculateSeriesSummary(t *testing.T) {
	tests := []struct {
		name               string
		seriesValues       []float64
		expectedMin        float64
		expectedMax        float64
		expectedAvg        float64
		expectedCount      int
		expectedHasNaN     bool
		expectedHasInf     bool
		expectedNonFinite  int
		expectedFirstValue float64
		expectedLastValue  float64
		expectedDelta      float64
		expectFiniteStats  bool // Whether we expect valid min/max/avg
	}{
		{
			name:               "all finite values",
			seriesValues:       []float64{10, 20, 30, 40, 50},
			expectedMin:        10,
			expectedMax:        50,
			expectedAvg:        30,
			expectedCount:      5,
			expectedHasNaN:     false,
			expectedHasInf:     false,
			expectedNonFinite:  0,
			expectedFirstValue: 10,
			expectedLastValue:  50,
			expectedDelta:      40,
			expectFiniteStats:  true,
		},
		{
			name:               "mixed finite and NaN",
			seriesValues:       []float64{10, math.NaN(), 30, 40},
			expectedMin:        10,
			expectedMax:        40,
			expectedAvg:        26.666666, // (10 + 30 + 40) / 3
			expectedCount:      4,
			expectedHasNaN:     true,
			expectedHasInf:     false,
			expectedNonFinite:  1,
			expectedFirstValue: 10,
			expectedLastValue:  40,
			expectedDelta:      30,
			expectFiniteStats:  true,
		},
		{
			name:               "negative values",
			seriesValues:       []float64{-50, -20, -10, 0, 10},
			expectedMin:        -50,
			expectedMax:        10,
			expectedAvg:        -14, // (-50 - 20 - 10 + 0 + 10) / 5
			expectedCount:      5,
			expectedHasNaN:     false,
			expectedHasInf:     false,
			expectedNonFinite:  0,
			expectedFirstValue: -50,
			expectedLastValue:  10,
			expectedDelta:      60,
			expectFiniteStats:  true,
		},
		{
			name:               "single value",
			seriesValues:       []float64{42.5},
			expectedMin:        42.5,
			expectedMax:        42.5,
			expectedAvg:        42.5,
			expectedCount:      1,
			expectedHasNaN:     false,
			expectedHasInf:     false,
			expectedNonFinite:  0,
			expectedFirstValue: 42.5,
			expectedLastValue:  42.5,
			expectedDelta:      0,
			expectFiniteStats:  true,
		},
		{
			name:               "multiple NaN and Inf",
			seriesValues:       []float64{5, math.NaN(), 10, math.Inf(1), 15, math.Inf(-1), 20},
			expectedMin:        5,
			expectedMax:        20,
			expectedAvg:        12.5, // (5 + 10 + 15 + 20) / 4
			expectedCount:      7,
			expectedHasNaN:     true,
			expectedHasInf:     true,
			expectedNonFinite:  3,
			expectedFirstValue: 5,
			expectedLastValue:  20,
			expectedDelta:      15,
			expectFiniteStats:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert float64 values to model.SamplePair
			samples := make([]model.SamplePair, len(tt.seriesValues))
			baseTime := time.Now().Unix() * 1000
			for i, val := range tt.seriesValues {
				samples[i] = model.SamplePair{
					Timestamp: model.Time(baseTime + int64(i*1000)),
					Value:     model.SampleValue(val),
				}
			}

			// Create a dummy metric
			metric := model.Metric{
				"__name__": "test_metric",
			}

			summary := CalculateSeriesSummary(metric, samples)

			// Verify count
			if summary.Count != tt.expectedCount {
				t.Errorf("count = %v, want %v", summary.Count, tt.expectedCount)
			}

			// Verify non-finite tracking
			if summary.HasNaN != tt.expectedHasNaN {
				t.Errorf("hasNaN = %v, want %v", summary.HasNaN, tt.expectedHasNaN)
			}
			if summary.HasInf != tt.expectedHasInf {
				t.Errorf("hasInf = %v, want %v", summary.HasInf, tt.expectedHasInf)
			}
			if summary.NonFiniteCount != tt.expectedNonFinite {
				t.Errorf("nonFiniteCount = %v, want %v", summary.NonFiniteCount, tt.expectedNonFinite)
			}

			// Verify statistics (only if we expect finite stats)
			if tt.expectFiniteStats {
				if math.Abs(summary.Min-tt.expectedMin) > 0.0001 {
					t.Errorf("min = %v, want %v", summary.Min, tt.expectedMin)
				}
				if math.Abs(summary.Max-tt.expectedMax) > 0.0001 {
					t.Errorf("max = %v, want %v", summary.Max, tt.expectedMax)
				}
				if math.Abs(summary.Avg-tt.expectedAvg) > 0.0001 {
					t.Errorf("avg = %v, want %v", summary.Avg, tt.expectedAvg)
				}
			}

			// Verify first/last values and delta
			if tt.expectFiniteStats {
				if math.Abs(summary.FirstValue-tt.expectedFirstValue) > 0.0001 {
					t.Errorf("firstValue = %v, want %v", summary.FirstValue, tt.expectedFirstValue)
				}
				if math.Abs(summary.LastValue-tt.expectedLastValue) > 0.0001 {
					t.Errorf("lastValue = %v, want %v", summary.LastValue, tt.expectedLastValue)
				}
				if math.Abs(summary.Delta-tt.expectedDelta) > 0.0001 {
					t.Errorf("delta = %v, want %v", summary.Delta, tt.expectedDelta)
				}
			} else if !math.IsNaN(tt.expectedFirstValue) || !math.IsNaN(summary.FirstValue) {
				// For all NaN case, verify NaN values
				if math.Abs(summary.FirstValue-tt.expectedFirstValue) > 0.0001 {
					t.Errorf("firstValue = %v, want %v", summary.FirstValue, tt.expectedFirstValue)
				}
			}
		})
	}
}

// go test ./pkg/tools/... -bench=BenchmarkCalculateSeriesSummary -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof -run=^$ -count 3 -v
// BenchmarkCalculateSeriesSummary benchmarks the summary calculation function
func BenchmarkCalculateSeriesSummary(b *testing.B) {
	benchmarks := []struct {
		name        string
		samples     int
		seriesCount int
	}{
		{
			name:        "10_series_100_samples",
			samples:     100,
			seriesCount: 10,
		},
		{
			name:        "10_series_1000_samples",
			samples:     1000,
			seriesCount: 10,
		},
		{
			name:        "100_series_100_samples",
			samples:     100,
			seriesCount: 100,
		},
		{
			name:        "100_series_1000_samples",
			samples:     1000,
			seriesCount: 100,
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Pre-generate series data
			series := make([]struct {
				metric model.Metric
				values []model.SamplePair
			}, bm.seriesCount)

			for i := 0; i < bm.seriesCount; i++ {
				series[i].metric = model.Metric{
					"__name__": model.LabelValue("test_metric"),
					"instance": model.LabelValue("instance_" + string(rune(i))),
					"job":      model.LabelValue("test_job"),
				}
				series[i].values = generateOscillatingData(bm.samples)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < bm.seriesCount; j++ {
					_ = CalculateSeriesSummary(series[j].metric, series[j].values)
				}
			}
		})
	}
}

// generateOscillatingData creates a sine wave pattern
func generateOscillatingData(n int) []model.SamplePair {
	samples := make([]model.SamplePair, n)
	baseTime := time.Now().Unix() * 1000
	for i := range n {
		// Sine wave with period of ~50 samples
		value := 50 + 30*math.Sin(float64(i)*0.126) // 2*pi/50 ≈ 0.126
		samples[i] = model.SamplePair{
			Timestamp: model.Time(baseTime + int64(i*1000)),
			Value:     model.SampleValue(value),
		}
	}
	return samples
}
