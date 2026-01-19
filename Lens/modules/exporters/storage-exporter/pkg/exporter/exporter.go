// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package exporter

import (
	"context"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/collector"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// MetricLabels defines the standard labels for storage metrics
var MetricLabels = []string{
	"storage_type",
	"filesystem_name",
	"mount_name",
	"mount_path",
}

// Exporter manages collection and exposure of storage metrics
type Exporter struct {
	collector *collector.Collector
	config    *config.StorageExporterConfig

	// Prometheus metrics
	capacityBytes  *prometheus.GaugeVec
	usedBytes      *prometheus.GaugeVec
	availableBytes *prometheus.GaugeVec
	usagePercent   *prometheus.GaugeVec

	// Inode metrics
	totalInodes *prometheus.GaugeVec
	usedInodes  *prometheus.GaugeVec
	freeInodes  *prometheus.GaugeVec

	// Collector metrics
	scrapeTotal    prometheus.Counter
	scrapeDuration prometheus.Histogram
	scrapeErrors   *prometheus.CounterVec

	// Internal state
	mu            sync.RWMutex
	lastCollected time.Time
	metricsCache  []collector.StorageMetrics

	// Registry
	registry *prometheus.Registry
}

// NewExporter creates a new storage exporter
func NewExporter(coll *collector.Collector, conf *config.StorageExporterConfig) *Exporter {
	clusterName := ""
	if conf.Metrics.StaticLabels != nil {
		clusterName = conf.Metrics.StaticLabels["primus_lens_cluster"]
	}

	e := &Exporter{
		collector: coll,
		config:    conf,
		registry:  prometheus.NewRegistry(),
	}

	constLabels := prometheus.Labels{}
	if clusterName != "" {
		constLabels["primus_lens_cluster"] = clusterName
	}

	// Initialize storage capacity metrics
	e.capacityBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage",
			Name:        "capacity_bytes",
			Help:        "Total capacity of the storage filesystem in bytes",
			ConstLabels: constLabels,
		},
		MetricLabels,
	)

	e.usedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage",
			Name:        "used_bytes",
			Help:        "Used storage in bytes",
			ConstLabels: constLabels,
		},
		MetricLabels,
	)

	e.availableBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage",
			Name:        "available_bytes",
			Help:        "Available storage in bytes",
			ConstLabels: constLabels,
		},
		MetricLabels,
	)

	e.usagePercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage",
			Name:        "usage_percent",
			Help:        "Storage usage percentage (0-100)",
			ConstLabels: constLabels,
		},
		MetricLabels,
	)

	// Inode metrics
	e.totalInodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage",
			Name:        "inodes_total",
			Help:        "Total number of inodes",
			ConstLabels: constLabels,
		},
		MetricLabels,
	)

	e.usedInodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage",
			Name:        "inodes_used",
			Help:        "Number of used inodes",
			ConstLabels: constLabels,
		},
		MetricLabels,
	)

	e.freeInodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage",
			Name:        "inodes_free",
			Help:        "Number of free inodes",
			ConstLabels: constLabels,
		},
		MetricLabels,
	)

	// Initialize collector metrics
	e.scrapeTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage_exporter",
			Name:        "scrapes_total",
			Help:        "Total number of scrapes performed",
			ConstLabels: constLabels,
		},
	)

	e.scrapeDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage_exporter",
			Name:        "scrape_duration_seconds",
			Help:        "Duration of scrapes in seconds",
			Buckets:     []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			ConstLabels: constLabels,
		},
	)

	e.scrapeErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage_exporter",
			Name:        "scrape_errors_total",
			Help:        "Total number of scrape errors by mount",
			ConstLabels: constLabels,
		},
		[]string{"mount_name"},
	)

	return e
}

// Register registers the exporter's routes and metrics
func (e *Exporter) Register() {
	// Register metrics with registry
	e.registry.MustRegister(
		// Storage metrics
		e.capacityBytes,
		e.usedBytes,
		e.availableBytes,
		e.usagePercent,
		// Inode metrics
		e.totalInodes,
		e.usedInodes,
		e.freeInodes,
		// Collector metrics
		e.scrapeTotal,
		e.scrapeDuration,
		e.scrapeErrors,
	)

	// Register HTTP routes
	router.RegisterGroup(func(group *gin.RouterGroup) error {
		g := group.Group("/storage")
		{
			g.GET("/mounts", e.handleListMounts)
			g.GET("/health", e.handleHealthCheck)
			g.GET("/metrics-cache", e.handleMetricsCache)
		}
		return nil
	})
}

// Collect collects metrics from all storage mounts
func (e *Exporter) Collect(ctx context.Context) error {
	startTime := time.Now()
	e.scrapeTotal.Inc()

	// Collect from all mounts
	metrics := e.collector.Collect(ctx)

	// Update prometheus metrics
	e.updatePrometheusMetrics(metrics)

	// Update cache
	e.mu.Lock()
	e.metricsCache = metrics
	e.lastCollected = time.Now()
	e.mu.Unlock()

	e.scrapeDuration.Observe(time.Since(startTime).Seconds())

	successCount := 0
	for _, m := range metrics {
		if m.Error == nil {
			successCount++
		}
	}
	log.Debugf("Collected storage metrics: %d/%d successful", successCount, len(metrics))

	return nil
}

func (e *Exporter) updatePrometheusMetrics(metrics []collector.StorageMetrics) {
	// Reset gauges to avoid stale data
	e.capacityBytes.Reset()
	e.usedBytes.Reset()
	e.availableBytes.Reset()
	e.usagePercent.Reset()
	e.totalInodes.Reset()
	e.usedInodes.Reset()
	e.freeInodes.Reset()

	for _, m := range metrics {
		if m.Error != nil {
			e.scrapeErrors.WithLabelValues(m.Name).Inc()
			continue
		}

		labels := []string{
			m.StorageType,
			m.FilesystemName,
			m.Name,
			m.MountPath,
		}

		e.capacityBytes.WithLabelValues(labels...).Set(float64(m.TotalBytes))
		e.usedBytes.WithLabelValues(labels...).Set(float64(m.UsedBytes))
		e.availableBytes.WithLabelValues(labels...).Set(float64(m.AvailableBytes))
		e.usagePercent.WithLabelValues(labels...).Set(m.UsagePercent)

		// Inode metrics (only set if available)
		if m.TotalInodes > 0 {
			e.totalInodes.WithLabelValues(labels...).Set(float64(m.TotalInodes))
			e.usedInodes.WithLabelValues(labels...).Set(float64(m.UsedInodes))
			e.freeInodes.WithLabelValues(labels...).Set(float64(m.FreeInodes))
		}
	}
}

// StartCollectionLoop starts the periodic collection loop
func (e *Exporter) StartCollectionLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial collection
	if err := e.Collect(ctx); err != nil {
		log.Errorf("Initial collection failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping storage exporter collection loop")
			return
		case <-ticker.C:
			if err := e.Collect(ctx); err != nil {
				log.Errorf("Collection failed: %v", err)
			}
		}
	}
}

// Gather implements prometheus.Gatherer
func (e *Exporter) Gather() ([]*dto.MetricFamily, error) {
	result := []*dto.MetricFamily{}

	// Gather from default registry
	defaultGather := prometheus.DefaultGatherer
	metrics, err := defaultGather.Gather()
	if err != nil {
		return nil, err
	}
	result = append(result, metrics...)

	// Gather from our registry
	storageMetrics, err := e.registry.Gather()
	if err != nil {
		return nil, err
	}
	result = append(result, storageMetrics...)

	return result, nil
}
