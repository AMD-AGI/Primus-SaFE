// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package exporter

import (
	"context"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/controller"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// MetricLabels defines the standard labels for storage metrics
var MetricLabels = []string{
	"storage_type",
	"filesystem_name",
}

// Exporter manages collection and exposure of storage metrics
type Exporter struct {
	controller *controller.Controller
	config     *config.StorageExporterConfig

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
	scrapeTotal      prometheus.Counter
	scrapeDuration   prometheus.Histogram
	scrapeErrors     *prometheus.CounterVec
	filesystemsTotal prometheus.Gauge

	// Internal state
	mu            sync.RWMutex
	lastCollected time.Time

	// Registry
	registry *prometheus.Registry
}

// NewExporter creates a new storage exporter
func NewExporter(ctrl *controller.Controller, conf *config.StorageExporterConfig) *Exporter {
	clusterName := ""
	if conf.Metrics.StaticLabels != nil {
		clusterName = conf.Metrics.StaticLabels["primus_lens_cluster"]
	}

	e := &Exporter{
		controller: ctrl,
		config:     conf,
		registry:   prometheus.NewRegistry(),
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
			Help:        "Total number of scrape errors by filesystem",
			ConstLabels: constLabels,
		},
		[]string{"filesystem_name"},
	)

	e.filesystemsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   "primus_lens",
			Subsystem:   "storage_exporter",
			Name:        "filesystems_total",
			Help:        "Total number of discovered filesystems",
			ConstLabels: constLabels,
		},
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
		e.filesystemsTotal,
	)

	// Register HTTP routes
	router.RegisterGroup(func(group *gin.RouterGroup) error {
		g := group.Group("/storage")
		{
			g.GET("/filesystems", e.handleListFilesystems)
			g.GET("/health", e.handleHealthCheck)
			g.GET("/metrics-cache", e.handleMetricsCache)
		}
		return nil
	})
}

// StartMetricsUpdateLoop starts the loop to update prometheus metrics from controller
func (e *Exporter) StartMetricsUpdateLoop(ctx context.Context, interval time.Duration) {
	log.Infof("Starting metrics update loop with interval %v", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial update after a short delay to let controller collect first batch
	time.Sleep(10 * time.Second)
	e.updatePrometheusMetrics()

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping storage exporter metrics update loop")
			return
		case <-ticker.C:
			e.updatePrometheusMetrics()
		}
	}
}

func (e *Exporter) updatePrometheusMetrics() {
	startTime := time.Now()
	e.scrapeTotal.Inc()

	metrics := e.controller.GetMetrics()
	filesystems := e.controller.GetFilesystems()

	log.Infof("Updating prometheus metrics: %d filesystems discovered, %d metrics collected",
		len(filesystems), len(metrics))

	// Reset gauges
	e.capacityBytes.Reset()
	e.usedBytes.Reset()
	e.availableBytes.Reset()
	e.usagePercent.Reset()
	e.totalInodes.Reset()
	e.usedInodes.Reset()
	e.freeInodes.Reset()

	e.filesystemsTotal.Set(float64(len(filesystems)))

	for fsName, m := range metrics {
		if m.Error != nil {
			e.scrapeErrors.WithLabelValues(fsName).Inc()
			continue
		}

		labels := []string{m.StorageType, m.FilesystemName}

		e.capacityBytes.WithLabelValues(labels...).Set(float64(m.TotalBytes))
		e.usedBytes.WithLabelValues(labels...).Set(float64(m.UsedBytes))
		e.availableBytes.WithLabelValues(labels...).Set(float64(m.AvailableBytes))
		e.usagePercent.WithLabelValues(labels...).Set(m.UsagePercent)

		if m.TotalInodes > 0 {
			e.totalInodes.WithLabelValues(labels...).Set(float64(m.TotalInodes))
			e.usedInodes.WithLabelValues(labels...).Set(float64(m.UsedInodes))
			e.freeInodes.WithLabelValues(labels...).Set(float64(m.FreeInodes))
		}
	}

	e.mu.Lock()
	e.lastCollected = time.Now()
	e.mu.Unlock()

	e.scrapeDuration.Observe(time.Since(startTime).Seconds())

	log.Debugf("Updated prometheus metrics for %d filesystems", len(metrics))
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
