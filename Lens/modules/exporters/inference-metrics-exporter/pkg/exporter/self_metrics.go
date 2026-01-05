package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ScrapeTargetsTotal is the total number of active scrape targets
	ScrapeTargetsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "inference_exporter_scrape_targets_total",
		Help: "Total number of active scrape targets",
	})

	// ScrapeTotal is the total number of scrapes performed
	ScrapeTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "inference_exporter_scrape_total",
		Help: "Total number of scrapes performed",
	})

	// ScrapeLatencySeconds is the histogram of scrape latency
	ScrapeLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "inference_exporter_scrape_latency_seconds",
		Help:    "Histogram of scrape latency in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})

	// ScrapeErrorsTotal is the total number of scrape errors
	ScrapeErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "inference_exporter_scrape_errors_total",
		Help: "Total number of scrape errors",
	}, []string{"reason"})

	// PendingTasksTotal is the number of pending tasks
	PendingTasksTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "inference_exporter_pending_tasks_total",
		Help: "Number of pending tasks waiting to be acquired",
	})

	// OwnedTasksTotal is the number of tasks owned by this instance
	OwnedTasksTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "inference_exporter_owned_tasks_total",
		Help: "Number of tasks owned by this instance",
	})

	// TransformLatencySeconds is the histogram of metrics transformation latency
	TransformLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "inference_exporter_transform_latency_seconds",
		Help:    "Histogram of metrics transformation latency",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
	})

	// ConfigReloadsTotal is the total number of config reloads
	ConfigReloadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "inference_exporter_config_reloads_total",
		Help: "Total number of config reloads",
	}, []string{"framework", "status"})

	// LockOperationsTotal is the total number of lock operations
	LockOperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "inference_exporter_lock_operations_total",
		Help: "Total number of lock operations",
	}, []string{"operation", "status"})
)

func init() {
	// Self metrics are auto-registered via promauto to default registry
	// No additional registration needed
}

// UpdateScrapeTargets updates the scrape targets gauge
func UpdateScrapeTargets(count int) {
	ScrapeTargetsTotal.Set(float64(count))
	OwnedTasksTotal.Set(float64(count))
}

// RecordScrape records a scrape operation
func RecordScrape(duration float64, err error) {
	ScrapeTotal.Inc()
	ScrapeLatencySeconds.Observe(duration)
	if err != nil {
		ScrapeErrorsTotal.WithLabelValues("scrape_failed").Inc()
	}
}

// RecordLockOperation records a lock operation
func RecordLockOperation(operation string, success bool) {
	status := "success"
	if !success {
		status = "failed"
	}
	LockOperationsTotal.WithLabelValues(operation, status).Inc()
}

// RecordConfigReload records a config reload operation
func RecordConfigReload(framework string, success bool) {
	status := "success"
	if !success {
		status = "failed"
	}
	ConfigReloadsTotal.WithLabelValues(framework, status).Inc()
}
