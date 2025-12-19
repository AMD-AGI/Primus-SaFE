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

	// Relevant metrics to extract
	relevantMetrics []string
}

// NewHigressCollector creates a new Higress collector
func NewHigressCollector(name string, config *collector.CollectorConfig, k8sClient client.Client) (*HigressCollector, error) {
	c := &HigressCollector{
		name:       name,
		config:     config,
		k8sClient:  k8sClient,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		labelMappings: map[string]string{
			// Envoy/Istio standard label mappings
			"request_host":                  "host",
			"destination_service_name":      "service",
			"destination_service_namespace": "namespace",
			"response_code":                 "code",
			"request_path":                  "path",
			"request_method":                "method",
		},
		relevantMetrics: []string{
			"istio_requests_total",
			"istio_request_duration_milliseconds",
			"istio_request_bytes_total",
			"istio_response_bytes_total",
		},
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
			metricsPort = 15020 // default Higress/Istio metrics port
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

	for _, metricName := range c.relevantMetrics {
		family, ok := metricFamilies[metricName]
		if !ok {
			continue
		}

		for _, metric := range family.Metric {
			rawMetric := c.convertMetric(family, metric, endpoint)
			if rawMetric != nil {
				result = append(result, *rawMetric)
			}
		}
	}

	return result, nil
}

func (c *HigressCollector) convertMetric(family *dto.MetricFamily, metric *dto.Metric, endpoint collector.GatewayEndpoint) *model.RawTrafficMetric {
	labels := make(map[string]string)
	for _, label := range metric.Label {
		labels[label.GetName()] = label.GetValue()
	}

	// Extract routing info from labels
	routingInfo := c.extractRoutingInfo(labels)

	// Skip if no destination service (internal traffic)
	if routingInfo.DestinationService == "" {
		return nil
	}

	// Skip internal Kubernetes services
	if strings.HasSuffix(routingInfo.DestinationService, "kubernetes") ||
		strings.HasSuffix(routingInfo.DestinationService, "kube-dns") {
		return nil
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
		// For histogram sum
		value = metric.Histogram.GetSampleSum()
		metricType = model.MetricTypeHistogram
	default:
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

func (c *HigressCollector) extractRoutingInfo(labels map[string]string) *model.RoutingInfo {
	return &model.RoutingInfo{
		Host:                 labels["request_host"],
		Path:                 labels["request_path"],
		Method:               labels["request_method"],
		ResponseCode:         labels["response_code"],
		DestinationService:   labels["destination_service_name"],
		DestinationNamespace: labels["destination_service_namespace"],
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

