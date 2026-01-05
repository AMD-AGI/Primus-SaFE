package higress

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/collector"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HigressCollector collects metrics from Higress Gateway
type HigressCollector struct {
	name       string
	config     *collector.CollectorConfig
	k8sClient  client.Client
	httpClient *http.Client

	// Label mappings for Higress/Envoy metrics
	labelMappings map[string]string

	// Relevant metrics to extract (Envoy format)
	relevantMetrics []string

	// Envoy metrics port (15090 for stats, 15020 for merged metrics)
	useEnvoyStatsPort bool
}

// NewHigressCollector creates a new Higress collector
func NewHigressCollector(name string, config *collector.CollectorConfig, k8sClient client.Client) (*HigressCollector, error) {
	c := &HigressCollector{
		name:       name,
		config:     config,
		k8sClient:  k8sClient,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		labelMappings: map[string]string{
			// Envoy label mappings
			"cluster_name":        "cluster",
			"response_code":       "code",
			"response_code_class": "code_class",
		},
		// Envoy native metrics - we'll parse cluster_name to extract service info
		// Format: outbound|port||service.namespace.svc.cluster.local
		relevantMetrics: []string{
			"envoy_cluster_upstream_rq_completed",
			"envoy_cluster_upstream_rq",
			"envoy_cluster_upstream_rq_time",
			"envoy_cluster_upstream_cx_total",
			"envoy_cluster_upstream_cx_active",
		},
		useEnvoyStatsPort: true,
	}

	// Apply custom label mappings from config
	if config.LabelMappings != nil {
		for k, v := range config.LabelMappings {
			c.labelMappings[k] = v
		}
	}

	return c, nil
}

// Type returns the gateway type
func (c *HigressCollector) Type() collector.GatewayType {
	return collector.GatewayTypeHigress
}

// Name returns the collector name
func (c *HigressCollector) Name() string {
	return c.name
}

// Discover discovers Higress Gateway endpoints
func (c *HigressCollector) Discover(ctx context.Context) ([]collector.GatewayEndpoint, error) {
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(c.config.Namespace),
	}

	// Build label selector
	if len(c.config.LabelSelector) > 0 {
		listOpts = append(listOpts, client.MatchingLabels(c.config.LabelSelector))
	}

	if err := c.k8sClient.List(ctx, podList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list higress gateway pods: %w", err)
	}

	var endpoints []collector.GatewayEndpoint
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Skip pods that are not ready
		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			continue
		}

		metricsPort := c.config.MetricsPort
		if metricsPort == 0 {
			// Use 15090 for Envoy native stats (has cluster-level metrics)
			// Use 15020 for merged Istio telemetry metrics
			if c.useEnvoyStatsPort {
				metricsPort = 15090
			} else {
				metricsPort = 15020
			}
		}

		metricsPath := c.config.MetricsPath
		if metricsPath == "" {
			metricsPath = "/stats/prometheus"
		}

		endpoint := collector.GatewayEndpoint{
			Address:     fmt.Sprintf("%s:%d", pod.Status.PodIP, metricsPort),
			MetricsPath: metricsPath,
			Labels: map[string]string{
				"pod":       pod.Name,
				"namespace": pod.Namespace,
				"node":      pod.Spec.NodeName,
			},
		}
		endpoints = append(endpoints, endpoint)
	}

	log.Debugf("Discovered %d Higress gateway endpoints", len(endpoints))
	return endpoints, nil
}

// Collect collects metrics from all Higress Gateway endpoints
func (c *HigressCollector) Collect(ctx context.Context) ([]model.RawTrafficMetric, error) {
	endpoints, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}

	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no higress gateway endpoints discovered")
	}

	var allMetrics []model.RawTrafficMetric
	for _, endpoint := range endpoints {
		metrics, err := c.scrapeEndpoint(ctx, endpoint)
		if err != nil {
			log.Warnf("Failed to scrape Higress endpoint %s: %v", endpoint.Address, err)
			continue
		}
		allMetrics = append(allMetrics, metrics...)
	}

	return allMetrics, nil
}

func (c *HigressCollector) scrapeEndpoint(ctx context.Context, endpoint collector.GatewayEndpoint) ([]model.RawTrafficMetric, error) {
	url := fmt.Sprintf("http://%s%s", endpoint.Address, endpoint.MetricsPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return c.parseMetrics(resp.Body, endpoint)
}

func (c *HigressCollector) parseMetrics(reader io.Reader, endpoint collector.GatewayEndpoint) ([]model.RawTrafficMetric, error) {
	parser := expfmt.TextParser{}
	metricFamilies, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	var result []model.RawTrafficMetric

	// Process Envoy cluster metrics
	for _, metricName := range c.relevantMetrics {
		family, ok := metricFamilies[metricName]
		if !ok {
			continue
		}

		for _, metric := range family.Metric {
			rawMetric := c.convertEnvoyMetric(family, metric, endpoint)
			if rawMetric != nil {
				result = append(result, *rawMetric)
			}
		}
	}

	// Also look for metrics with response_code_class label (aggregated by status class)
	if family, ok := metricFamilies["envoy_cluster_upstream_rq"]; ok {
		for _, metric := range family.Metric {
			rawMetric := c.convertEnvoyMetric(family, metric, endpoint)
			if rawMetric != nil {
				result = append(result, *rawMetric)
			}
		}
	}

	return result, nil
}

// convertEnvoyMetric converts Envoy native metrics to our internal format
func (c *HigressCollector) convertEnvoyMetric(family *dto.MetricFamily, metric *dto.Metric, endpoint collector.GatewayEndpoint) *model.RawTrafficMetric {
	labels := make(map[string]string)
	for _, label := range metric.Label {
		labels[label.GetName()] = label.GetValue()
	}

	// Parse cluster_name to extract service info
	// Format: outbound|port||service.namespace.svc.cluster.local
	clusterName := labels["cluster_name"]
	routingInfo := c.parseClusterName(clusterName)

	// Skip if no destination service (internal traffic or non-outbound clusters)
	if routingInfo == nil || routingInfo.DestinationService == "" {
		return nil
	}

	// Add response code class if present
	if codeClass, ok := labels["response_code_class"]; ok {
		routingInfo.ResponseCode = codeClass
	}

	var value float64
	var metricType model.MetricType

	switch family.GetType() {
	case dto.MetricType_COUNTER:
		value = metric.Counter.GetValue()
		metricType = model.MetricTypeCounter
	case dto.MetricType_GAUGE:
		value = metric.Gauge.GetValue()
		metricType = model.MetricTypeGauge
	case dto.MetricType_HISTOGRAM:
		value = metric.Histogram.GetSampleSum()
		metricType = model.MetricTypeHistogram
	default:
		return nil
	}

	// Skip zero-value counters
	if value == 0 {
		return nil
	}

	return &model.RawTrafficMetric{
		Name:            family.GetName(),
		Value:           value,
		Type:            metricType,
		Timestamp:       time.Now(),
		GatewayType:     string(collector.GatewayTypeHigress),
		GatewayInstance: endpoint.Labels["pod"],
		OriginalLabels:  labels,
		RoutingInfo:     routingInfo,
	}
}

// parseClusterName parses Envoy cluster_name to extract service info
// Format: outbound|port||service.namespace.svc.cluster.local
// Also handles: outbound|port|subset|service.namespace.svc.cluster.local
func (c *HigressCollector) parseClusterName(clusterName string) *model.RoutingInfo {
	// Skip non-outbound clusters
	if !strings.HasPrefix(clusterName, "outbound|") {
		return nil
	}

	// Split by |
	parts := strings.Split(clusterName, "|")
	if len(parts) < 4 {
		return nil
	}

	port := parts[1]
	fqdn := parts[3] // service.namespace.svc.cluster.local

	// Skip internal clusters
	internalClusters := []string{"agent", "prometheus_stats", "xds-grpc", "sds-grpc", "zipkin"}
	for _, internal := range internalClusters {
		if strings.Contains(clusterName, internal) {
			return nil
		}
	}

	// Parse FQDN
	// Format: service.namespace.svc.cluster.local
	fqdnParts := strings.Split(fqdn, ".")
	if len(fqdnParts) < 2 {
		return nil
	}

	serviceName := fqdnParts[0]
	namespace := fqdnParts[1]

	// Skip kubernetes internal services
	internalServices := []string{"kubernetes", "kube-dns", "coredns"}
	for _, internal := range internalServices {
		if serviceName == internal {
			return nil
		}
	}

	return &model.RoutingInfo{
		DestinationService:   serviceName,
		DestinationNamespace: namespace,
		DestinationPort:      port,
	}
}

// HealthCheck checks if the collector is healthy
func (c *HigressCollector) HealthCheck(ctx context.Context) error {
	endpoints, err := c.Discover(ctx)
	if err != nil {
		return err
	}

	if len(endpoints) == 0 {
		return fmt.Errorf("no higress gateway endpoints discovered")
	}

	// Try to scrape at least one endpoint
	for _, endpoint := range endpoints {
		url := fmt.Sprintf("http://%s%s", endpoint.Address, endpoint.MetricsPath)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return nil
		}
	}

	return fmt.Errorf("failed to scrape any higress gateway endpoint")
}

// NewHigressCollectorFactory returns a factory for creating Higress collectors
func NewHigressCollectorFactory() collector.CollectorFactory {
	return func(name string, config *collector.CollectorConfig, k8sClient client.Client) (collector.Collector, error) {
		return NewHigressCollector(name, config, k8sClient)
	}
}

// init registers the Higress collector factory
func init() {
	// This will be called when the package is imported
}
