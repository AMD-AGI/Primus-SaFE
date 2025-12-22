package model

import "time"

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// RawTrafficMetric represents a raw traffic metric from gateway
type RawTrafficMetric struct {
	// Basic metric info
	Name      string     // metric name, e.g., "istio_requests_total"
	Value     float64    // metric value
	Type      MetricType // counter, gauge, histogram
	Timestamp time.Time

	// Gateway source info
	GatewayType     string // higress, nginx, istio
	GatewayInstance string // pod name of gateway

	// Original labels from gateway
	OriginalLabels map[string]string

	// Extracted routing info
	RoutingInfo *RoutingInfo

	// For histogram metrics
	Buckets []HistogramBucket
}

// RoutingInfo contains routing information extracted from gateway metrics
type RoutingInfo struct {
	Host                 string // request host, e.g., "tw325.primus-safe.amd.com"
	Path                 string // request path
	Method               string // HTTP method
	ResponseCode         string // HTTP response code
	DestinationService   string // destination service name
	DestinationNamespace string // destination namespace
	DestinationPort      string // destination port (as string for flexibility)
}

// HistogramBucket represents a histogram bucket
type HistogramBucket struct {
	UpperBound float64
	Count      uint64
}

// EnrichedTrafficMetric is the enriched version with workload info
type EnrichedTrafficMetric struct {
	RawTrafficMetric

	// Enriched workload info
	WorkloadInfo *WorkloadInfo
}

// WorkloadInfo contains workload information
type WorkloadInfo struct {
	ServiceName      string
	ServiceNamespace string
	PodName          string
	PodIP            string
	NodeName         string
	WorkloadName     string // primus-safe workload name (from gpu_workload table)
	WorkloadUID      string // primus-safe workload uid (from workload_pod_reference)
	WorkloadOwner    string // workload owner
	WorkloadType     string // deployment, statefulset, etc.
}

// HasGpuWorkload returns true if this metric is associated with a GPU workload in Primus
func (w *WorkloadInfo) HasGpuWorkload() bool {
	return w != nil && w.WorkloadUID != ""
}

