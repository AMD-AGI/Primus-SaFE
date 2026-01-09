// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package exporter

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// MetricsExporter manages the metrics registry for inference metrics
type MetricsExporter struct {
	// Registry for self metrics (exporter internal metrics)
	selfRegistry *prometheus.Registry

	// Collector for inference metrics from all targets
	inferenceCollector *InferenceMetricsCollector

	// Combined registry for /metrics endpoint
	combinedRegistry *prometheus.Registry

	// Per-workload metadata storage
	workloadMetrics map[string]*WorkloadMetrics
	mu              sync.RWMutex
}

// WorkloadMetrics holds metrics for a single workload
type WorkloadMetrics struct {
	WorkloadUID string
	Framework   string
	Labels      map[string]string
}

// NewMetricsExporter creates a new metrics exporter
func NewMetricsExporter() *MetricsExporter {
	selfRegistry := prometheus.NewRegistry()
	inferenceCollector := NewInferenceMetricsCollector()
	combinedRegistry := prometheus.NewRegistry()

	// Register inference collector to combined registry
	combinedRegistry.MustRegister(inferenceCollector)

	return &MetricsExporter{
		selfRegistry:       selfRegistry,
		inferenceCollector: inferenceCollector,
		combinedRegistry:   combinedRegistry,
		workloadMetrics:    make(map[string]*WorkloadMetrics),
	}
}

// GetSelfRegistry returns the registry for self metrics
func (e *MetricsExporter) GetSelfRegistry() *prometheus.Registry {
	return e.selfRegistry
}

// GetRegistry returns the combined registry (for backward compatibility)
func (e *MetricsExporter) GetRegistry() *prometheus.Registry {
	return e.combinedRegistry
}

// GetInferenceCollector returns the inference metrics collector
func (e *MetricsExporter) GetInferenceCollector() *InferenceMetricsCollector {
	return e.inferenceCollector
}

// Handler returns the HTTP handler for /metrics endpoint
// This handler serves both self metrics and inference metrics
func (e *MetricsExporter) Handler() http.Handler {
	// Create a gatherer that combines both registries
	gatherers := prometheus.Gatherers{
		e.selfRegistry,
		e.combinedRegistry,
	}

	return promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorHandling:     promhttp.ContinueOnError,
	})
}

// SelfMetricsHandler returns handler for exporter self metrics only
func (e *MetricsExporter) SelfMetricsHandler() http.Handler {
	return promhttp.HandlerFor(e.selfRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// InferenceMetricsHandler returns handler for inference metrics only
func (e *MetricsExporter) InferenceMetricsHandler() http.Handler {
	return promhttp.HandlerFor(e.combinedRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// UpdateWorkloadMetrics updates transformed metrics for a workload
func (e *MetricsExporter) UpdateWorkloadMetrics(workloadUID string, framework string, labels map[string]string, metrics []*dto.MetricFamily) {
	e.mu.Lock()
	e.workloadMetrics[workloadUID] = &WorkloadMetrics{
		WorkloadUID: workloadUID,
		Framework:   framework,
		Labels:      labels,
	}
	e.mu.Unlock()

	// Update the collector with new metrics
	e.inferenceCollector.UpdateMetrics(workloadUID, metrics)
}

// UpdateMetrics updates metadata for a workload (legacy interface)
func (e *MetricsExporter) UpdateMetrics(workloadUID string, framework string, labels map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.workloadMetrics[workloadUID] = &WorkloadMetrics{
		WorkloadUID: workloadUID,
		Framework:   framework,
		Labels:      labels,
	}
}

// RemoveMetrics removes metrics for a workload
func (e *MetricsExporter) RemoveMetrics(workloadUID string) {
	e.mu.Lock()
	delete(e.workloadMetrics, workloadUID)
	e.mu.Unlock()

	// Remove from collector
	e.inferenceCollector.RemoveMetrics(workloadUID)
}

// GetWorkloadCount returns the number of workloads being tracked
func (e *MetricsExporter) GetWorkloadCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.workloadMetrics)
}

// ListWorkloads returns a list of tracked workload UIDs
func (e *MetricsExporter) ListWorkloads() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	uids := make([]string, 0, len(e.workloadMetrics))
	for uid := range e.workloadMetrics {
		uids = append(uids, uid)
	}
	return uids
}

// GetMetricsStats returns statistics about collected metrics
func (e *MetricsExporter) GetMetricsStats() MetricsStats {
	return e.inferenceCollector.GetStats()
}

// SerializeInferenceMetrics serializes inference metrics to Prometheus text format
func (e *MetricsExporter) SerializeInferenceMetrics() ([]byte, error) {
	return e.inferenceCollector.SerializeToText()
}

