package tools

import (
	"math"

	"github.com/prometheus/common/model"
)

// isFinite checks if a float64 value is neither NaN nor Inf
func isFinite(x float64) bool {
	return !(math.IsNaN(x) || math.IsInf(x, 0))
}

// convertMetricToMap converts a model.Metric to map[string]string efficiently
func convertMetricToMap(metric model.Metric) map[string]string {
	if len(metric) == 0 {
		return nil
	}

	labels := make(map[string]string, len(metric))
	for k, v := range metric {
		labels[string(k)] = string(v)
	}
	return labels
}

// CalculateSeriesSummary computes summary statistics for a time series.
func CalculateSeriesSummary(metric model.Metric, values []model.SamplePair) SeriesResultSummary {
	count := len(values)

	summary := SeriesResultSummary{
		Series: convertMetricToMap(metric),
		Count:  count,
	}

	if count == 0 {
		return summary
	}

	var sum float64
	var minValue, maxValue float64
	finiteCount := 0
	hasNaN := false
	hasInf := false
	nonFiniteCount := 0

	summary.FirstTimestamp = float64(values[0].Timestamp) / millisecondsPerSecond
	summary.LastTimestamp = float64(values[count-1].Timestamp) / millisecondsPerSecond

	// Get first and last values
	summary.FirstValue = float64(values[0].Value)
	summary.LastValue = float64(values[count-1].Value)

	// Process all values
	for _, sample := range values {
		val := float64(sample.Value)

		// Track non-finite values
		if !isFinite(val) {
			nonFiniteCount++
			if math.IsNaN(val) {
				hasNaN = true
			}
			if math.IsInf(val, 0) {
				hasInf = true
			}
			continue
		}

		// Calculate statistics only for finite values
		if finiteCount == 0 {
			minValue = val
			maxValue = val
			sum = val
		} else {
			if val < minValue {
				minValue = val
			}
			if val > maxValue {
				maxValue = val
			}
			sum += val
		}
		finiteCount++
	}

	// Set summary statistics
	summary.HasNaN = hasNaN
	summary.HasInf = hasInf
	summary.NonFiniteCount = nonFiniteCount

	if finiteCount > 0 {
		summary.Min = minValue
		summary.Max = maxValue
		summary.Avg = sum / float64(finiteCount)
	}

	// Calculate delta
	summary.Delta = summary.LastValue - summary.FirstValue

	return summary
}
