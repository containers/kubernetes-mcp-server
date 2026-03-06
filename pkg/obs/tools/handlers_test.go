package tools

import (
	"math"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

// TestIsFinite tests the isFinite function
func TestIsFinite(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected bool
	}{
		{
			name:     "finite positive",
			input:    42.5,
			expected: true,
		},
		{
			name:     "finite negative",
			input:    -42.5,
			expected: true,
		},
		{
			name:     "zero",
			input:    0.0,
			expected: true,
		},
		{
			name:     "NaN",
			input:    math.NaN(),
			expected: false,
		},
		{
			name:     "positive infinity",
			input:    math.Inf(1),
			expected: false,
		},
		{
			name:     "negative infinity",
			input:    math.Inf(-1),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFinite(tt.input)
			if result != tt.expected {
				t.Errorf("isFinite(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

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
			name:               "mixed finite and Inf",
			seriesValues:       []float64{5, 15, math.Inf(1), 25},
			expectedMin:        5,
			expectedMax:        25,
			expectedAvg:        15, // (5 + 15 + 25) / 3
			expectedCount:      4,
			expectedHasNaN:     false,
			expectedHasInf:     true,
			expectedNonFinite:  1,
			expectedFirstValue: 5,
			expectedLastValue:  25,
			expectedDelta:      20,
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
			name:               "all NaN",
			seriesValues:       []float64{math.NaN(), math.NaN(), math.NaN()},
			expectedMin:        0,
			expectedMax:        0,
			expectedAvg:        0,
			expectedCount:      3,
			expectedHasNaN:     true,
			expectedHasInf:     false,
			expectedNonFinite:  3,
			expectedFirstValue: math.NaN(),
			expectedLastValue:  math.NaN(),
			expectedDelta:      math.NaN(),
			expectFiniteStats:  false,
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
			name:               "mixed with -Inf",
			seriesValues:       []float64{10, math.Inf(-1), 20, 30},
			expectedMin:        10,
			expectedMax:        30,
			expectedAvg:        20, // (10 + 20 + 30) / 3
			expectedCount:      4,
			expectedHasNaN:     false,
			expectedHasInf:     true,
			expectedNonFinite:  1,
			expectedFirstValue: 10,
			expectedLastValue:  30,
			expectedDelta:      20,
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

			// Calculate summary using the new function
			extremaOpts := ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			}
			summary := CalculateSeriesSummary(metric, samples, extremaOpts)

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

// TestDetectExtrema tests the peak and trough detection function
func TestDetectExtrema(t *testing.T) {
	tests := []struct {
		name           string
		samples        []model.SamplePair
		opts           ExtremaOptions
		expectedCount  int
		expectedPeaks  int
		expectedTrough int
		validate       func(t *testing.T, extrema []Extremum)
	}{
		{
			name: "simple peak",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 10},
				{Timestamp: 2000, Value: 20}, // Peak
				{Timestamp: 3000, Value: 10},
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			expectedCount:  1,
			expectedPeaks:  1,
			expectedTrough: 0,
			validate: func(t *testing.T, extrema []Extremum) {
				if len(extrema) != 1 {
					return
				}
				if extrema[0].Kind != ExtremumPeak {
					t.Errorf("expected peak, got %v", extrema[0].Kind)
				}
				if extrema[0].Value != 20 {
					t.Errorf("expected value 20, got %v", extrema[0].Value)
				}
				if extrema[0].Index != 1 {
					t.Errorf("expected index 1, got %v", extrema[0].Index)
				}
			},
		},
		{
			name: "simple trough",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 20},
				{Timestamp: 2000, Value: 10}, // Trough
				{Timestamp: 3000, Value: 20},
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			expectedCount:  1,
			expectedPeaks:  0,
			expectedTrough: 1,
			validate: func(t *testing.T, extrema []Extremum) {
				if len(extrema) != 1 {
					return
				}
				if extrema[0].Kind != ExtremumTrough {
					t.Errorf("expected trough, got %v", extrema[0].Kind)
				}
				if extrema[0].Value != 10 {
					t.Errorf("expected value 10, got %v", extrema[0].Value)
				}
			},
		},
		{
			name: "multiple peaks and troughs",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 10},
				{Timestamp: 2000, Value: 20}, // Peak
				{Timestamp: 3000, Value: 5},  // Trough
				{Timestamp: 4000, Value: 25}, // Peak
				{Timestamp: 5000, Value: 15},
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			expectedCount:  3,
			expectedPeaks:  2,
			expectedTrough: 1,
		},
		{
			name: "minDelta filtering",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 10},
				{Timestamp: 2000, Value: 11}, // Small peak, should be filtered
				{Timestamp: 3000, Value: 10},
				{Timestamp: 4000, Value: 30}, // Large peak, should be kept
				{Timestamp: 5000, Value: 10},
			},
			opts: ExtremaOptions{
				MinDelta:      5.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			expectedCount:  1,
			expectedPeaks:  1,
			expectedTrough: 0,
			validate: func(t *testing.T, extrema []Extremum) {
				if len(extrema) != 1 {
					return
				}
				if extrema[0].Value != 30 {
					t.Errorf("expected peak at 30, got %v", extrema[0].Value)
				}
			},
		},
		{
			name: "minSeparation filtering",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 10},
				{Timestamp: 2000, Value: 20}, // Peak 1
				{Timestamp: 3000, Value: 15}, // Trough (between the two peaks)
				{Timestamp: 4000, Value: 25}, // Peak 2, close to Peak 1, higher
				{Timestamp: 5000, Value: 10},
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 5 * time.Second,
				IncludeEdges:  false,
			},
			expectedCount:  2, // One peak (25) and one trough (15)
			expectedPeaks:  1,
			expectedTrough: 1,
			validate: func(t *testing.T, extrema []Extremum) {
				// Should keep the higher peak (25, not 20)
				foundPeak25 := false
				foundTrough15 := false
				for _, e := range extrema {
					if e.Kind == ExtremumPeak && e.Value == 25 {
						foundPeak25 = true
					}
					if e.Kind == ExtremumTrough && e.Value == 15 {
						foundTrough15 = true
					}
				}
				if !foundPeak25 {
					t.Errorf("expected to find peak at 25")
				}
				if !foundTrough15 {
					t.Errorf("expected to find trough at 15")
				}
			},
		},
		{
			name: "include edges - first point peak",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 30}, // Edge peak
				{Timestamp: 2000, Value: 20}, // Trough
				{Timestamp: 3000, Value: 10},
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  true,
			},
			expectedCount:  2, // Edge peak at 30 + trough at 20
			expectedPeaks:  1,
			expectedTrough: 1,
		},
		{
			name: "include edges - last point trough",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 30}, // Peak
				{Timestamp: 2000, Value: 20},
				{Timestamp: 3000, Value: 10}, // Edge trough
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  true,
			},
			expectedCount:  2, // Peak at 30 + edge trough at 10
			expectedPeaks:  1,
			expectedTrough: 1,
		},
		{
			name: "exclude edges",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 30}, // Would be edge peak
				{Timestamp: 2000, Value: 20},
				{Timestamp: 3000, Value: 10}, // Would be edge trough
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			expectedCount:  0,
			expectedPeaks:  0,
			expectedTrough: 0,
		},
		{
			name: "NaN values ignored",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 10},
				{Timestamp: 2000, Value: model.SampleValue(math.NaN())},
				{Timestamp: 3000, Value: 20},
				{Timestamp: 4000, Value: model.SampleValue(math.NaN())},
				{Timestamp: 5000, Value: 10},
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			expectedCount: 0, // Can't detect extrema with NaN neighbors
		},
		{
			name: "monotonic increasing - no extrema",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 10},
				{Timestamp: 2000, Value: 20},
				{Timestamp: 3000, Value: 30},
				{Timestamp: 4000, Value: 40},
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			expectedCount:  0,
			expectedPeaks:  0,
			expectedTrough: 0,
		},
		{
			name: "insufficient samples",
			samples: []model.SamplePair{
				{Timestamp: 1000, Value: 10},
				{Timestamp: 2000, Value: 20},
			},
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extrema := detectExtrema(tt.samples, tt.opts)

			if len(extrema) != tt.expectedCount {
				t.Errorf("detectExtrema() count = %v, want %v", len(extrema), tt.expectedCount)
			}

			peaks := 0
			troughs := 0
			for _, e := range extrema {
				switch e.Kind {
				case ExtremumPeak:
					peaks++
				case ExtremumTrough:
					troughs++
				}
			}

			if peaks != tt.expectedPeaks {
				t.Errorf("detectExtrema() peaks = %v, want %v", peaks, tt.expectedPeaks)
			}
			if troughs != tt.expectedTrough {
				t.Errorf("detectExtrema() troughs = %v, want %v", troughs, tt.expectedTrough)
			}

			if tt.validate != nil {
				tt.validate(t, extrema)
			}
		})
	}
}

// go test ./pkg/tools/... -bench=BenchmarkCalculateSeriesSummary -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof -run=^$ -count 3 -v
// BenchmarkCalculateSeriesSummary benchmarks the summary calculation function
func BenchmarkCalculateSeriesSummary(b *testing.B) {
	extremaOpts := ExtremaOptions{
		MinDelta:      0.0,
		MinSeparation: 0,
		IncludeEdges:  false,
	}

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
					_ = CalculateSeriesSummary(series[j].metric, series[j].values, extremaOpts)
				}
			}
		})
	}
}

// go test ./pkg/tools/... -bench=BenchmarkDetectExtrema -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof -run=^$ -count 3 -v
// BenchmarkDetectExtrema benchmarks peak and trough detection with various data patterns and sizes
func BenchmarkDetectExtrema(b *testing.B) {
	benchmarks := []struct {
		name    string
		samples int
		opts    ExtremaOptions
		genData func(n int) []model.SamplePair
	}{
		{
			name:    "100_samples_oscillating_no_filtering",
			samples: 100,
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			genData: generateOscillatingData,
		},
		{
			name:    "1000_samples_oscillating_no_filtering",
			samples: 1000,
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			genData: generateOscillatingData,
		},
		{
			name:    "10000_samples_oscillating_no_filtering",
			samples: 10000,
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 0,
				IncludeEdges:  false,
			},
			genData: generateOscillatingData,
		},
		{
			name:    "1000_samples_with_minSeparation",
			samples: 1000,
			opts: ExtremaOptions{
				MinDelta:      0.0,
				MinSeparation: 10 * time.Second,
				IncludeEdges:  false,
			},
			genData: generateOscillatingData,
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			samples := bm.genData(bm.samples)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = detectExtrema(samples, bm.opts)
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
