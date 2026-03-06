package tools

import (
	"math"
	"sync"
	"time"

	"github.com/prometheus/common/model"
)

var (
	// Pool for Extremum slices to reduce allocations
	extremumSlicePool = sync.Pool{
		New: func() any {
			s := make([]Extremum, 0, 32)
			return &s
		},
	}
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
func CalculateSeriesSummary(metric model.Metric, values []model.SamplePair, opts ExtremaOptions) SeriesResultSummary {
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

	summary.FirstTimestamp = float64(values[0].Timestamp) / 1000
	summary.LastTimestamp = float64(values[count-1].Timestamp) / 1000

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

	// Detect peaks and troughs
	summary.Extrema = detectExtrema(values, opts)

	return summary
}

// getExtremumSlice gets a slice from the pool
func getExtremumSlice() *[]Extremum {
	return extremumSlicePool.Get().(*[]Extremum)
}

// putExtremumSlice returns a slice to the pool after clearing it
func putExtremumSlice(s *[]Extremum) {
	*s = (*s)[:0]
	extremumSlicePool.Put(s)
}

// detectExtrema finds peaks and troughs in a time series.
func detectExtrema(samples []model.SamplePair, opts ExtremaOptions) []Extremum {
	if len(samples) < 3 && !opts.IncludeEdges {
		return nil
	}
	if len(samples) == 0 {
		return nil
	}

	// Get candidates slice from pool
	candidatesPtr := getExtremumSlice()
	candidates := *candidatesPtr

	// Estimate capacity based on sample count
	// For typical oscillating metrics: ~1 extremum per 12-25 samples
	// Add small buffer to reduce growth
	estimatedCap := (len(samples) / 12) + 4
	if estimatedCap > cap(candidates) {
		// Grow the slice for this call, pool will retain the larger capacity
		candidates = make([]Extremum, 0, estimatedCap)
		*candidatesPtr = candidates
	}

	// Helper to check if point at index i is a peak or trough
	checkExtremum := func(i int) {
		val := float64(samples[i].Value)
		if !isFinite(val) {
			return
		}

		var leftVal, rightVal float64
		hasLeft := i > 0
		hasRight := i < len(samples)-1

		if hasLeft {
			leftVal = float64(samples[i-1].Value)
			if !isFinite(leftVal) {
				return // Skip if neighbor is non-finite
			}
		}
		if hasRight {
			rightVal = float64(samples[i+1].Value)
			if !isFinite(rightVal) {
				return // Skip if neighbor is non-finite
			}
		}

		// Edge cases: first or last point
		if !hasLeft || !hasRight {
			if !opts.IncludeEdges {
				return
			}
			// For edge points, only check the one neighbor
			if !hasLeft && hasRight {
				// First point - check if it's different enough from right
				if math.Abs(val-rightVal) < opts.MinDelta {
					return
				}
				kind := ExtremumPeak
				if val < rightVal {
					kind = ExtremumTrough
				}
				candidates = append(candidates, Extremum{
					Kind:      kind,
					Timestamp: time.Unix(int64(samples[i].Timestamp)/1000, (int64(samples[i].Timestamp)%1000)*1e6),
					Value:     val,
					Index:     i,
				})
				return
			}
			if hasLeft && !hasRight {
				// Last point - check if it's different enough from left
				if math.Abs(val-leftVal) < opts.MinDelta {
					return
				}
				kind := ExtremumPeak
				if val < leftVal {
					kind = ExtremumTrough
				}
				candidates = append(candidates, Extremum{
					Kind:      kind,
					Timestamp: time.Unix(int64(samples[i].Timestamp)/1000, (int64(samples[i].Timestamp)%1000)*1e6),
					Value:     val,
					Index:     i,
				})
				return
			}
		}

		// Normal case: check both neighbors
		isPeak := val > leftVal && val > rightVal
		isTrough := val < leftVal && val < rightVal

		if !isPeak && !isTrough {
			return
		}

		// Check MinDelta threshold
		minDiff := math.Min(math.Abs(val-leftVal), math.Abs(val-rightVal))
		if minDiff < opts.MinDelta {
			return
		}

		kind := ExtremumPeak
		if isTrough {
			kind = ExtremumTrough
		}

		candidates = append(candidates, Extremum{
			Kind:      kind,
			Timestamp: time.Unix(int64(samples[i].Timestamp)/1000, (int64(samples[i].Timestamp)%1000)*1e6),
			Value:     val,
			Index:     i,
		})
	}

	// Scan all points
	for i := range samples {
		checkExtremum(i)
	}

	// Apply MinSeparation filtering if needed
	if opts.MinSeparation > 0 {
		candidates = filterBySeparation(candidates, opts.MinSeparation)
	}

	// Make a copy of the result since we'll return the pool buffer
	result := make([]Extremum, len(candidates))
	copy(result, candidates)

	// Return pool buffer
	*candidatesPtr = candidates
	putExtremumSlice(candidatesPtr)

	return result
}

// filterBySeparation removes extrema that are too close together,
// keeping the more extreme one (higher peak or lower trough).
func filterBySeparation(candidates []Extremum, minSep time.Duration) []Extremum {
	if len(candidates) <= 1 {
		return candidates
	}

	// Get temp buffer from pool
	tempPtr := getExtremumSlice()
	temp := *tempPtr

	// Ensure capacity
	if cap(temp) < len(candidates) {
		temp = make([]Extremum, 0, len(candidates))
	}
	temp = temp[:len(candidates)]

	peakCount := 0
	troughCount := 0

	// Separate peaks and troughs in one pass
	for i := range candidates {
		if candidates[i].Kind == ExtremumPeak {
			temp[peakCount] = candidates[i]
			peakCount++
		}
	}

	peaks := temp[:peakCount]

	for i := range candidates {
		if candidates[i].Kind == ExtremumTrough {
			temp[peakCount+troughCount] = candidates[i]
			troughCount++
		}
	}
	troughs := temp[peakCount : peakCount+troughCount]

	// Filter each group in-place
	filteredPeakCount := filterSameKindInPlace(peaks, minSep, true)
	filteredTroughCount := filterSameKindInPlace(troughs, minSep, false)

	// Build result
	result := make([]Extremum, 0, filteredPeakCount+filteredTroughCount)
	result = append(result, peaks[:filteredPeakCount]...)
	result = append(result, troughs[:filteredTroughCount]...)

	// Return temp buffer to pool
	*tempPtr = temp
	putExtremumSlice(tempPtr)

	// Sort by index - simple bubble sort is fine for small slices
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Index < result[i].Index {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// filterSameKindInPlace filters extrema of the same kind in-place,
// keeping the more extreme one when they're too close together.
// Returns the new length of the filtered slice.
func filterSameKindInPlace(extrema []Extremum, minSep time.Duration, isPeak bool) int {
	if len(extrema) <= 1 {
		return len(extrema)
	}

	writeIdx := 0
	i := 0

	for i < len(extrema) {
		current := extrema[i]

		// Look ahead for extrema within minSep
		j := i + 1
		for j < len(extrema) && extrema[j].Timestamp.Sub(current.Timestamp) < minSep {
			// Keep the more extreme one
			if isPeak {
				if extrema[j].Value > current.Value {
					current = extrema[j]
				}
			} else {
				if extrema[j].Value < current.Value {
					current = extrema[j]
				}
			}
			j++
		}

		extrema[writeIdx] = current
		writeIdx++
		i = j
	}

	return writeIdx
}
