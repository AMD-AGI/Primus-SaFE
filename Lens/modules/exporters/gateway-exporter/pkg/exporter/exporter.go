package exporter

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/collector"
	gwconfig "github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/enricher"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// MetricLabels defines the standard labels for gateway metrics (without GPU workload)
var MetricLabels = []string{
	// Gateway info
	"gateway_type",
	"gateway_instance",

	// Routing info
	"host",
	"path",
	"method",
	"response_code",

	// Service info
	"service_name",
	"service_namespace",

	// Pod info
	"pod_name",
	"node_name",

	// Primus Lens standard labels
	"primus_lens_cluster",
}

// WorkloadMetricLabels defines the labels for workload gateway metrics (with GPU workload)
var WorkloadMetricLabels = []string{
	// Gateway info
	"gateway_type",
	"gateway_instance",

	// Routing info
	"host",
	"path",
	"method",
	"response_code",

	// Service info
	"service_name",
	"service_namespace",

	// Pod info
	"pod_name",
	"node_name",

	// Workload info (only for GPU workloads)
	"workload_name",
	"workload_uid",
	"workload_owner",

	// Primus Lens standard labels
	"primus_lens_cluster",
}

// Exporter manages collection and exposure of gateway metrics
type Exporter struct {
	manager  *collector.Manager
	enricher *enricher.Enricher
	config   *gwconfig.GatewayExporterConfig

	// Prometheus metrics for general gateway traffic (no GPU workload)
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestBytes    *prometheus.CounterVec
	responseBytes   *prometheus.CounterVec

	// Prometheus metrics for GPU workload traffic
	workloadRequestsTotal   *prometheus.CounterVec
	workloadRequestDuration *prometheus.HistogramVec
	workloadRequestBytes    *prometheus.CounterVec
	workloadResponseBytes   *prometheus.CounterVec

	// Collector metrics
	scrapeTotal    prometheus.Counter
	scrapeDuration prometheus.Histogram
	scrapeErrors   prometheus.Counter

	// Internal state
	mu            sync.RWMutex
	lastCollected time.Time
	metricsCache  []model.EnrichedTrafficMetric

	// Registry
	registry *prometheus.Registry
}

// NewExporter creates a new gateway exporter
func NewExporter(manager *collector.Manager, enricher *enricher.Enricher, config *gwconfig.GatewayExporterConfig) *Exporter {
	clusterName := ""
	if config.Metrics.StaticLabels != nil {
		clusterName = config.Metrics.StaticLabels["primus_lens_cluster"]
	}

	e := &Exporter{
		manager:  manager,
		enricher: enricher,
		config:   config,
		registry: prometheus.NewRegistry(),
	}

	// Initialize general gateway traffic metrics (primus_lens_gateway_*)
	e.requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "gateway",
			Name:      "requests_total",
			Help:      "Total number of requests processed by gateway (non-workload traffic)",
			ConstLabels: prometheus.Labels{
				"primus_lens_cluster": clusterName,
			},
		},
		MetricLabels[:len(MetricLabels)-1], // exclude primus_lens_cluster as it's a const label
	)

	e.requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "primus_lens",
			Subsystem: "gateway",
			Name:      "request_duration_milliseconds",
			Help:      "Request duration in milliseconds (non-workload traffic)",
			Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
			ConstLabels: prometheus.Labels{
				"primus_lens_cluster": clusterName,
			},
		},
		MetricLabels[:len(MetricLabels)-1],
	)

	e.requestBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "gateway",
			Name:      "request_bytes_total",
			Help:      "Total bytes of requests (non-workload traffic)",
			ConstLabels: prometheus.Labels{
				"primus_lens_cluster": clusterName,
			},
		},
		MetricLabels[:len(MetricLabels)-1],
	)

	e.responseBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "gateway",
			Name:      "response_bytes_total",
			Help:      "Total bytes of responses (non-workload traffic)",
			ConstLabels: prometheus.Labels{
				"primus_lens_cluster": clusterName,
			},
		},
		MetricLabels[:len(MetricLabels)-1],
	)

	// Initialize GPU workload gateway traffic metrics (primus_lens_workload_gateway_*)
	e.workloadRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "workload_gateway",
			Name:      "requests_total",
			Help:      "Total number of requests processed by gateway for GPU workloads",
			ConstLabels: prometheus.Labels{
				"primus_lens_cluster": clusterName,
			},
		},
		WorkloadMetricLabels[:len(WorkloadMetricLabels)-1],
	)

	e.workloadRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "primus_lens",
			Subsystem: "workload_gateway",
			Name:      "request_duration_milliseconds",
			Help:      "Request duration in milliseconds for GPU workloads",
			Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
			ConstLabels: prometheus.Labels{
				"primus_lens_cluster": clusterName,
			},
		},
		WorkloadMetricLabels[:len(WorkloadMetricLabels)-1],
	)

	e.workloadRequestBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "workload_gateway",
			Name:      "request_bytes_total",
			Help:      "Total bytes of requests for GPU workloads",
			ConstLabels: prometheus.Labels{
				"primus_lens_cluster": clusterName,
			},
		},
		WorkloadMetricLabels[:len(WorkloadMetricLabels)-1],
	)

	e.workloadResponseBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "workload_gateway",
			Name:      "response_bytes_total",
			Help:      "Total bytes of responses for GPU workloads",
			ConstLabels: prometheus.Labels{
				"primus_lens_cluster": clusterName,
			},
		},
		WorkloadMetricLabels[:len(WorkloadMetricLabels)-1],
	)

	// Initialize collector metrics
	e.scrapeTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "gateway_exporter",
			Name:      "scrapes_total",
			Help:      "Total number of scrapes performed",
		},
	)

	e.scrapeDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "primus_lens",
			Subsystem: "gateway_exporter",
			Name:      "scrape_duration_seconds",
			Help:      "Duration of scrapes in seconds",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
	)

	e.scrapeErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "gateway_exporter",
			Name:      "scrape_errors_total",
			Help:      "Total number of scrape errors",
		},
	)

	return e
}

// Register registers the exporter's routes and metrics
func (e *Exporter) Register() {
	// Register metrics with registry
	e.registry.MustRegister(
		// General gateway metrics
		e.requestsTotal,
		e.requestDuration,
		e.requestBytes,
		e.responseBytes,
		// Workload gateway metrics
		e.workloadRequestsTotal,
		e.workloadRequestDuration,
		e.workloadRequestBytes,
		e.workloadResponseBytes,
		// Collector metrics
		e.scrapeTotal,
		e.scrapeDuration,
		e.scrapeErrors,
	)

	// Register HTTP routes
	router.RegisterGroup(func(group *gin.RouterGroup) error {
		g := group.Group("/gateway")
		{
			g.GET("/collectors", e.handleListCollectors)
			g.GET("/health", e.handleHealthCheck)
			g.GET("/cache-stats", e.handleCacheStats)
		}
		return nil
	})
}

// Collect collects metrics from all collectors
func (e *Exporter) Collect(ctx context.Context) error {
	startTime := time.Now()
	e.scrapeTotal.Inc()

	// Collect from all collectors
	rawMetrics, err := e.manager.CollectAll(ctx)
	if err != nil {
		e.scrapeErrors.Inc()
		return err
	}

	// Enrich metrics
	enrichedMetrics, err := e.enricher.Enrich(ctx, rawMetrics)
	if err != nil {
		e.scrapeErrors.Inc()
		return err
	}

	// Update prometheus metrics
	e.updatePrometheusMetrics(enrichedMetrics)

	// Update cache
	e.mu.Lock()
	e.metricsCache = enrichedMetrics
	e.lastCollected = time.Now()
	e.mu.Unlock()

	e.scrapeDuration.Observe(time.Since(startTime).Seconds())

	log.Debugf("Collected %d metrics from gateway", len(enrichedMetrics))
	return nil
}

func (e *Exporter) updatePrometheusMetrics(metrics []model.EnrichedTrafficMetric) {
	// Reset counters to avoid stale data
	e.requestsTotal.Reset()
	e.requestBytes.Reset()
	e.responseBytes.Reset()
	e.workloadRequestsTotal.Reset()
	e.workloadRequestBytes.Reset()
	e.workloadResponseBytes.Reset()

	for _, metric := range metrics {
		// Check if this is a GPU workload
		isGpuWorkload := metric.WorkloadInfo.HasGpuWorkload()

		if isGpuWorkload {
			// Use workload-specific metrics
			labels := e.buildWorkloadLabels(metric)
			e.updateWorkloadMetric(metric.Name, metric.Value, labels)
		} else {
			// Use general gateway metrics
			labels := e.buildLabels(metric)
			e.updateGeneralMetric(metric.Name, metric.Value, labels)
		}
	}
}

func (e *Exporter) updateGeneralMetric(metricName string, value float64, labels []string) {
	switch metricName {
	case "istio_requests_total":
		e.requestsTotal.WithLabelValues(labels...).Add(value)
	case "istio_request_bytes_total":
		e.requestBytes.WithLabelValues(labels...).Add(value)
	case "istio_response_bytes_total":
		e.responseBytes.WithLabelValues(labels...).Add(value)
	case "envoy_cluster_upstream_rq":
		e.requestsTotal.WithLabelValues(labels...).Add(value)
	default:
		if strings.HasPrefix(metricName, "envoy_cluster_upstream_rq_") ||
			strings.HasPrefix(metricName, "envoy_http_downstream_rq_") {
			e.requestsTotal.WithLabelValues(labels...).Add(value)
		}
	}
}

func (e *Exporter) updateWorkloadMetric(metricName string, value float64, labels []string) {
	switch metricName {
	case "istio_requests_total":
		e.workloadRequestsTotal.WithLabelValues(labels...).Add(value)
	case "istio_request_bytes_total":
		e.workloadRequestBytes.WithLabelValues(labels...).Add(value)
	case "istio_response_bytes_total":
		e.workloadResponseBytes.WithLabelValues(labels...).Add(value)
	case "envoy_cluster_upstream_rq":
		e.workloadRequestsTotal.WithLabelValues(labels...).Add(value)
	default:
		if strings.HasPrefix(metricName, "envoy_cluster_upstream_rq_") ||
			strings.HasPrefix(metricName, "envoy_http_downstream_rq_") {
			e.workloadRequestsTotal.WithLabelValues(labels...).Add(value)
		}
	}
}

// buildLabels builds labels for general gateway metrics (no GPU workload)
func (e *Exporter) buildLabels(metric model.EnrichedTrafficMetric) []string {
	labels := make([]string, len(MetricLabels)-1) // exclude primus_lens_cluster

	// Gateway info
	labels[0] = metric.GatewayType
	labels[1] = metric.GatewayInstance

	// Routing info
	if metric.RoutingInfo != nil {
		labels[2] = metric.RoutingInfo.Host
		labels[3] = metric.RoutingInfo.Path
		labels[4] = metric.RoutingInfo.Method
		labels[5] = metric.RoutingInfo.ResponseCode
	}

	// Service and pod info
	if metric.WorkloadInfo != nil {
		labels[6] = metric.WorkloadInfo.ServiceName
		labels[7] = metric.WorkloadInfo.ServiceNamespace
		labels[8] = metric.WorkloadInfo.PodName
		labels[9] = metric.WorkloadInfo.NodeName
	}

	return labels
}

// buildWorkloadLabels builds labels for GPU workload gateway metrics
func (e *Exporter) buildWorkloadLabels(metric model.EnrichedTrafficMetric) []string {
	labels := make([]string, len(WorkloadMetricLabels)-1) // exclude primus_lens_cluster

	// Gateway info
	labels[0] = metric.GatewayType
	labels[1] = metric.GatewayInstance

	// Routing info
	if metric.RoutingInfo != nil {
		labels[2] = metric.RoutingInfo.Host
		labels[3] = metric.RoutingInfo.Path
		labels[4] = metric.RoutingInfo.Method
		labels[5] = metric.RoutingInfo.ResponseCode
	}

	// Service and workload info
	if metric.WorkloadInfo != nil {
		labels[6] = metric.WorkloadInfo.ServiceName
		labels[7] = metric.WorkloadInfo.ServiceNamespace
		labels[8] = metric.WorkloadInfo.PodName
		labels[9] = metric.WorkloadInfo.NodeName
		labels[10] = metric.WorkloadInfo.WorkloadName
		labels[11] = metric.WorkloadInfo.WorkloadUID
		labels[12] = metric.WorkloadInfo.WorkloadOwner
	}

	return labels
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
			log.Info("Stopping gateway exporter collection loop")
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
	gatewayMetrics, err := e.registry.Gather()
	if err != nil {
		return nil, err
	}
	result = append(result, gatewayMetrics...)

	return result, nil
}
