// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package exporter

import (
	"bytes"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// InferenceMetricsCollector collects metrics from all scrape targets
type InferenceMetricsCollector struct {
	mu sync.RWMutex

	// Store metrics by workload UID
	workloadMetrics map[string][]*dto.MetricFamily

	// Description for the collector
	desc *prometheus.Desc
}

// NewInferenceMetricsCollector creates a new inference metrics collector
func NewInferenceMetricsCollector() *InferenceMetricsCollector {
	return &InferenceMetricsCollector{
		workloadMetrics: make(map[string][]*dto.MetricFamily),
		desc: prometheus.NewDesc(
			"inference_metrics_collector",
			"Collector for inference service metrics",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector
func (c *InferenceMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect implements prometheus.Collector
func (c *InferenceMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for workloadUID, families := range c.workloadMetrics {
		for _, mf := range families {
			if mf == nil || mf.Name == nil {
				continue
			}

			for _, m := range mf.Metric {
				metric, err := c.convertMetric(mf, m, workloadUID)
				if err != nil {
					log.Debugf("Failed to convert metric %s: %v", *mf.Name, err)
					continue
				}
				if metric != nil {
					ch <- metric
				}
			}
		}
	}
}

// convertMetric converts a dto.Metric to a prometheus.Metric
func (c *InferenceMetricsCollector) convertMetric(mf *dto.MetricFamily, m *dto.Metric, workloadUID string) (prometheus.Metric, error) {
	name := *mf.Name

	// Extract label names and values
	labelNames := make([]string, 0, len(m.Label))
	labelValues := make([]string, 0, len(m.Label))
	for _, lp := range m.Label {
		if lp.Name != nil && lp.Value != nil {
			labelNames = append(labelNames, *lp.Name)
			labelValues = append(labelValues, *lp.Value)
		}
	}

	// Create appropriate metric type
	switch {
	case m.Counter != nil:
		desc := prometheus.NewDesc(name, getHelp(mf), labelNames, nil)
		return prometheus.MustNewConstMetric(desc, prometheus.CounterValue, m.Counter.GetValue(), labelValues...), nil

	case m.Gauge != nil:
		desc := prometheus.NewDesc(name, getHelp(mf), labelNames, nil)
		return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, m.Gauge.GetValue(), labelValues...), nil

	case m.Untyped != nil:
		desc := prometheus.NewDesc(name, getHelp(mf), labelNames, nil)
		return prometheus.MustNewConstMetric(desc, prometheus.UntypedValue, m.Untyped.GetValue(), labelValues...), nil

	case m.Histogram != nil:
		// For histograms, we need to create bucket metrics
		return c.createHistogramMetrics(name, mf, m, labelNames, labelValues)

	case m.Summary != nil:
		// For summaries, we need to create quantile metrics
		return c.createSummaryMetrics(name, mf, m, labelNames, labelValues)
	}

	return nil, nil
}

// createHistogramMetrics creates histogram metrics
func (c *InferenceMetricsCollector) createHistogramMetrics(name string, mf *dto.MetricFamily, m *dto.Metric, labelNames, labelValues []string) (prometheus.Metric, error) {
	h := m.Histogram

	// Create buckets map
	buckets := make(map[float64]uint64)
	for _, b := range h.Bucket {
		if b.UpperBound != nil && b.CumulativeCount != nil {
			buckets[*b.UpperBound] = *b.CumulativeCount
		}
	}

	desc := prometheus.NewDesc(name, getHelp(mf), labelNames, nil)
	return prometheus.MustNewConstHistogram(
		desc,
		h.GetSampleCount(),
		h.GetSampleSum(),
		buckets,
		labelValues...,
	), nil
}

// createSummaryMetrics creates summary metrics
func (c *InferenceMetricsCollector) createSummaryMetrics(name string, mf *dto.MetricFamily, m *dto.Metric, labelNames, labelValues []string) (prometheus.Metric, error) {
	s := m.Summary

	// Create quantiles map
	quantiles := make(map[float64]float64)
	for _, q := range s.Quantile {
		if q.Quantile != nil && q.Value != nil {
			quantiles[*q.Quantile] = *q.Value
		}
	}

	desc := prometheus.NewDesc(name, getHelp(mf), labelNames, nil)
	return prometheus.MustNewConstSummary(
		desc,
		s.GetSampleCount(),
		s.GetSampleSum(),
		quantiles,
		labelValues...,
	), nil
}

// UpdateMetrics updates metrics for a workload
func (c *InferenceMetricsCollector) UpdateMetrics(workloadUID string, metrics []*dto.MetricFamily) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workloadMetrics[workloadUID] = metrics
}

// RemoveMetrics removes metrics for a workload
func (c *InferenceMetricsCollector) RemoveMetrics(workloadUID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.workloadMetrics, workloadUID)
}

// GetWorkloadCount returns the number of workloads
func (c *InferenceMetricsCollector) GetWorkloadCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.workloadMetrics)
}

// GetAllMetrics returns all metrics as metric families
func (c *InferenceMetricsCollector) GetAllMetrics() map[string][]*dto.MetricFamily {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string][]*dto.MetricFamily, len(c.workloadMetrics))
	for k, v := range c.workloadMetrics {
		result[k] = v
	}
	return result
}

// SerializeToText serializes all metrics to Prometheus text format
func (c *InferenceMetricsCollector) SerializeToText() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var buf bytes.Buffer

	for _, families := range c.workloadMetrics {
		for _, mf := range families {
			if mf == nil {
				continue
			}
			_, err := expfmt.MetricFamilyToText(&buf, mf)
			if err != nil {
				log.Debugf("Failed to serialize metric family: %v", err)
				continue
			}
		}
	}

	return buf.Bytes(), nil
}

// getHelp returns help text from metric family
func getHelp(mf *dto.MetricFamily) string {
	if mf.Help != nil {
		return *mf.Help
	}
	return ""
}

// MetricsStats contains statistics about collected metrics
type MetricsStats struct {
	WorkloadCount   int            `json:"workload_count"`
	MetricFamilies  int            `json:"metric_families"`
	TotalMetrics    int            `json:"total_metrics"`
	ByWorkload      map[string]int `json:"by_workload"`
}

// GetStats returns statistics about collected metrics
func (c *InferenceMetricsCollector) GetStats() MetricsStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := MetricsStats{
		WorkloadCount: len(c.workloadMetrics),
		ByWorkload:    make(map[string]int),
	}

	for uid, families := range c.workloadMetrics {
		stats.MetricFamilies += len(families)
		count := 0
		for _, mf := range families {
			count += len(mf.Metric)
		}
		stats.TotalMetrics += count
		stats.ByWorkload[uid] = count
	}

	return stats
}

// Ensure interface compliance
var _ prometheus.Collector = (*InferenceMetricsCollector)(nil)

