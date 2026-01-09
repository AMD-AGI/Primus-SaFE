// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package scraper

import (
	"fmt"
	"io"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// MetricsParser parses Prometheus exposition format metrics
type MetricsParser struct {
	// Optional: framework-specific parsing hints
	frameworkHints map[string][]string
}

// NewMetricsParser creates a new metrics parser
func NewMetricsParser() *MetricsParser {
	return &MetricsParser{
		frameworkHints: make(map[string][]string),
	}
}

// Parse parses Prometheus format metrics from a reader
func (p *MetricsParser) Parse(r io.Reader) ([]*dto.MetricFamily, error) {
	parser := expfmt.TextParser{}

	mfMap, err := parser.TextToMetricFamilies(r)
	if err != nil {
		return nil, fmt.Errorf("parse text format: %w", err)
	}

	result := make([]*dto.MetricFamily, 0, len(mfMap))
	for _, mf := range mfMap {
		result = append(result, mf)
	}

	return result, nil
}

// ParseFromBytes parses metrics from a byte slice
func (p *MetricsParser) ParseFromBytes(data []byte) ([]*dto.MetricFamily, error) {
	parser := expfmt.TextParser{}

	mfMap, err := parser.TextToMetricFamilies(bytesReader{data: data})
	if err != nil {
		return nil, fmt.Errorf("parse text format: %w", err)
	}

	result := make([]*dto.MetricFamily, 0, len(mfMap))
	for _, mf := range mfMap {
		result = append(result, mf)
	}

	return result, nil
}

// bytesReader implements io.Reader for a byte slice
type bytesReader struct {
	data []byte
	pos  int
}

func (r bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// GetMetricNames extracts all metric names from parsed families
func GetMetricNames(families []*dto.MetricFamily) []string {
	names := make([]string, 0, len(families))
	for _, mf := range families {
		if mf.Name != nil {
			names = append(names, *mf.Name)
		}
	}
	return names
}

// FilterByPrefix filters metric families by name prefix
func FilterByPrefix(families []*dto.MetricFamily, prefix string) []*dto.MetricFamily {
	result := make([]*dto.MetricFamily, 0)
	for _, mf := range families {
		if mf.Name != nil && len(*mf.Name) >= len(prefix) && (*mf.Name)[:len(prefix)] == prefix {
			result = append(result, mf)
		}
	}
	return result
}

// CountMetrics counts the total number of metrics across all families
func CountMetrics(families []*dto.MetricFamily) int {
	count := 0
	for _, mf := range families {
		count += len(mf.Metric)
	}
	return count
}

// MetricsSummary provides a summary of parsed metrics
type MetricsSummary struct {
	TotalFamilies int            `json:"total_families"`
	TotalMetrics  int            `json:"total_metrics"`
	TypeCounts    map[string]int `json:"type_counts"`
	SampleNames   []string       `json:"sample_names,omitempty"`
}

// Summarize creates a summary of the parsed metrics
func Summarize(families []*dto.MetricFamily) MetricsSummary {
	summary := MetricsSummary{
		TotalFamilies: len(families),
		TypeCounts:    make(map[string]int),
	}

	for _, mf := range families {
		summary.TotalMetrics += len(mf.Metric)

		typeName := "unknown"
		if mf.Type != nil {
			typeName = mf.Type.String()
		}
		summary.TypeCounts[typeName]++

		if mf.Name != nil && len(summary.SampleNames) < 10 {
			summary.SampleNames = append(summary.SampleNames, *mf.Name)
		}
	}

	return summary
}

