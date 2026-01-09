// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workload_statistic

import (
	"encoding/json"
	"math"
	"sort"
)

// HistogramBucket represents a bucket in the histogram
type HistogramBucket struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
	Count int     `json:"count"`
}

// Histogram structure for storing GPU utilization distribution
type Histogram struct {
	Buckets []HistogramBucket `json:"buckets"`
}

// NewHistogram creates a new histogram with fixed bucket ranges (0-100, 10% per bucket)
func NewHistogram() *Histogram {
	buckets := make([]HistogramBucket, 10)
	for i := 0; i < 10; i++ {
		buckets[i] = HistogramBucket{
			Lower: float64(i * 10),
			Upper: float64((i + 1) * 10),
			Count: 0,
		}
	}
	return &Histogram{Buckets: buckets}
}

// AddValues adds new values to the histogram
func (h *Histogram) AddValues(values []float64) {
	for _, val := range values {
		// Ensure value is within 0-100 range
		if val < 0 {
			val = 0
		} else if val > 100 {
			val = 100
		}

		// Find the corresponding bucket
		for i := range h.Buckets {
			if val >= h.Buckets[i].Lower && val < h.Buckets[i].Upper {
				h.Buckets[i].Count++
				break
			}
			// Handle edge case: val = 100 should go into the last bucket
			if val == 100 && i == len(h.Buckets)-1 {
				h.Buckets[i].Count++
				break
			}
		}
	}
}

// CalculatePercentile calculates percentile from the histogram
func (h *Histogram) CalculatePercentile(percentile float64) float64 {
	if len(h.Buckets) == 0 {
		return 0
	}

	// Calculate total sample count
	totalCount := 0
	for _, bucket := range h.Buckets {
		totalCount += bucket.Count
	}

	if totalCount == 0 {
		return 0
	}

	// Calculate target position
	targetCount := int(math.Ceil(float64(totalCount) * percentile / 100.0))
	currentCount := 0

	// Find target bucket
	for _, bucket := range h.Buckets {
		currentCount += bucket.Count
		if currentCount >= targetCount {
			// For single value case, return bucket midpoint as best estimate
			if bucket.Count == 1 && totalCount == 1 {
				return (bucket.Lower + bucket.Upper) / 2.0
			}

			// Linear interpolation within the bucket
			bucketPosition := float64(targetCount-(currentCount-bucket.Count)) / float64(bucket.Count)
			return bucket.Lower + (bucket.Upper-bucket.Lower)*bucketPosition
		}
	}

	// If not found (should not happen), return max value
	return h.Buckets[len(h.Buckets)-1].Upper
}

// ToJSON converts histogram to JSON bytes
func (h *Histogram) ToJSON() ([]byte, error) {
	return json.Marshal(h)
}

// FromJSON parses histogram from JSON bytes
func FromJSON(data []byte) (*Histogram, error) {
	if len(data) == 0 {
		return NewHistogram(), nil
	}

	var h Histogram
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}

	// If buckets are empty after parsing, initialize default buckets
	if len(h.Buckets) == 0 {
		return NewHistogram(), nil
	}

	return &h, nil
}

// GetTotalCount returns the total number of samples in the histogram
func (h *Histogram) GetTotalCount() int {
	total := 0
	for _, bucket := range h.Buckets {
		total += bucket.Count
	}
	return total
}

// calculatePercentilesFromHistogram calculates multiple percentiles from histogram at once (performance optimization)
func calculatePercentilesFromHistogram(h *Histogram) (p50, p90, p95 float64) {
	if len(h.Buckets) == 0 {
		return 0, 0, 0
	}

	totalCount := h.GetTotalCount()
	if totalCount == 0 {
		return 0, 0, 0
	}

	// Calculate target positions
	target50 := int(math.Ceil(float64(totalCount) * 50.0 / 100.0))
	target90 := int(math.Ceil(float64(totalCount) * 90.0 / 100.0))
	target95 := int(math.Ceil(float64(totalCount) * 95.0 / 100.0))

	currentCount := 0
	found50, found90, found95 := false, false, false

	// Calculate all percentiles in a single pass
	for _, bucket := range h.Buckets {
		prevCount := currentCount
		currentCount += bucket.Count

		if !found50 && currentCount >= target50 {
			// For single value case, use bucket midpoint
			if bucket.Count == 1 && totalCount == 1 {
				p50 = (bucket.Lower + bucket.Upper) / 2.0
			} else {
				bucketPosition := float64(target50-prevCount) / float64(bucket.Count)
				p50 = bucket.Lower + (bucket.Upper-bucket.Lower)*bucketPosition
			}
			found50 = true
		}

		if !found90 && currentCount >= target90 {
			// For single value case, use bucket midpoint
			if bucket.Count == 1 && totalCount == 1 {
				p90 = (bucket.Lower + bucket.Upper) / 2.0
			} else {
				bucketPosition := float64(target90-prevCount) / float64(bucket.Count)
				p90 = bucket.Lower + (bucket.Upper-bucket.Lower)*bucketPosition
			}
			found90 = true
		}

		if !found95 && currentCount >= target95 {
			// For single value case, use bucket midpoint
			if bucket.Count == 1 && totalCount == 1 {
				p95 = (bucket.Lower + bucket.Upper) / 2.0
			} else {
				bucketPosition := float64(target95-prevCount) / float64(bucket.Count)
				p95 = bucket.Lower + (bucket.Upper-bucket.Lower)*bucketPosition
			}
			found95 = true
			break // Found all required percentiles
		}
	}

	return p50, p90, p95
}

// calculatePercentilesFromValues calculates percentiles from raw value array (for small datasets or validation)
func calculatePercentilesFromValues(values []float64) (p50, p90, p95 float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}

	// Sort values
	sortedValues := make([]float64, len(values))
	copy(sortedValues, values)
	sort.Float64s(sortedValues)

	p50 = calculatePercentileFromSorted(sortedValues, 50)
	p90 = calculatePercentileFromSorted(sortedValues, 90)
	p95 = calculatePercentileFromSorted(sortedValues, 95)

	return p50, p90, p95
}

// calculatePercentileFromSorted calculates percentile from sorted array
func calculatePercentileFromSorted(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}

	index := (percentile / 100.0) * float64(len(sortedValues)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sortedValues[lower]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}
