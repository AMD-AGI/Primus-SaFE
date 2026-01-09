// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package higress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/collector"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHigressCollector(t *testing.T) {
	config := &collector.CollectorConfig{
		Type:          collector.GatewayTypeHigress,
		Enabled:       true,
		Namespace:     "higress-system",
		LabelSelector: map[string]string{"app": "higress-gateway"},
		MetricsPort:   15020,
		MetricsPath:   "/stats/prometheus",
	}

	c, err := NewHigressCollector("test-higress", config, nil)
	require.NoError(t, err)
	assert.Equal(t, "test-higress", c.Name())
	assert.Equal(t, collector.GatewayTypeHigress, c.Type())
}

func TestHigressCollector_ParseMetrics(t *testing.T) {
	// Sample Prometheus metrics from Higress/Envoy (native format)
	sampleMetrics := `
# HELP envoy_cluster_upstream_rq_completed Total completed requests
# TYPE envoy_cluster_upstream_rq_completed counter
envoy_cluster_upstream_rq_completed{cluster_name="outbound|8080||my-service.default.svc.cluster.local"} 100
envoy_cluster_upstream_rq_completed{cluster_name="outbound|8080||another-service.production.svc.cluster.local"} 50
envoy_cluster_upstream_rq_completed{cluster_name="agent"} 1000
# HELP envoy_cluster_upstream_rq Upstream requests by response code class
# TYPE envoy_cluster_upstream_rq counter
envoy_cluster_upstream_rq{response_code_class="2xx",cluster_name="outbound|8080||my-service.default.svc.cluster.local"} 95
envoy_cluster_upstream_rq{response_code_class="5xx",cluster_name="outbound|8080||my-service.default.svc.cluster.local"} 5
`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(sampleMetrics))
	}))
	defer server.Close()

	config := &collector.CollectorConfig{
		Type:        collector.GatewayTypeHigress,
		MetricsPath: "/stats/prometheus",
	}

	c, err := NewHigressCollector("test", config, nil)
	require.NoError(t, err)

	// Override HTTP client timeout
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Create endpoint pointing to test server
	endpoint := collector.GatewayEndpoint{
		Address:     server.Listener.Addr().String(),
		MetricsPath: "/stats/prometheus",
		Labels: map[string]string{
			"pod":       "higress-gateway-xxx",
			"namespace": "higress-system",
		},
	}

	metrics, err := c.scrapeEndpoint(context.Background(), endpoint)
	require.NoError(t, err)
	// Should have parsed outbound cluster metrics but skipped internal "agent" cluster
	assert.GreaterOrEqual(t, len(metrics), 2)

	// Verify parsed metrics have correct routing info
	for _, m := range metrics {
		if m.RoutingInfo != nil {
			assert.NotEmpty(t, m.RoutingInfo.DestinationService)
			assert.NotEmpty(t, m.RoutingInfo.DestinationNamespace)
		}
	}
}

func TestHigressCollector_ParseClusterName(t *testing.T) {
	config := &collector.CollectorConfig{
		Type: collector.GatewayTypeHigress,
	}

	c, _ := NewHigressCollector("test", config, nil)

	// Test valid outbound cluster name
	clusterName := "outbound|8080||user-service.production.svc.cluster.local"
	routingInfo := c.parseClusterName(clusterName)

	require.NotNil(t, routingInfo)
	assert.Equal(t, "user-service", routingInfo.DestinationService)
	assert.Equal(t, "production", routingInfo.DestinationNamespace)
	assert.Equal(t, "8080", routingInfo.DestinationPort)

	// Test with subset
	clusterNameWithSubset := "outbound|8080|v1|user-service.production.svc.cluster.local"
	routingInfoSubset := c.parseClusterName(clusterNameWithSubset)
	require.NotNil(t, routingInfoSubset)
	assert.Equal(t, "user-service", routingInfoSubset.DestinationService)
}

func TestHigressCollector_SkipInternalServices(t *testing.T) {
	config := &collector.CollectorConfig{
		Type: collector.GatewayTypeHigress,
	}

	c, _ := NewHigressCollector("test", config, nil)

	// Internal clusters should return nil
	internalClusters := []string{
		"agent",
		"prometheus_stats",
		"xds-grpc",
		"outbound|443||kubernetes.default.svc.cluster.local",
		"outbound|53||kube-dns.kube-system.svc.cluster.local",
	}

	for _, clusterName := range internalClusters {
		routingInfo := c.parseClusterName(clusterName)
		assert.Nil(t, routingInfo, "Expected nil for cluster: %s", clusterName)
	}

	// Valid service should not be nil
	validCluster := "outbound|8080||my-service.default.svc.cluster.local"
	routingInfo := c.parseClusterName(validCluster)
	assert.NotNil(t, routingInfo)
	assert.Equal(t, "my-service", routingInfo.DestinationService)
}

func TestRawTrafficMetricModel(t *testing.T) {
	metric := model.RawTrafficMetric{
		Name:            "istio_requests_total",
		Value:           100,
		Type:            model.MetricTypeCounter,
		Timestamp:       time.Now(),
		GatewayType:     "higress",
		GatewayInstance: "higress-gateway-xxx",
		OriginalLabels: map[string]string{
			"response_code": "200",
		},
		RoutingInfo: &model.RoutingInfo{
			Host:                 "example.com",
			ResponseCode:         "200",
			DestinationService:   "my-service",
			DestinationNamespace: "default",
		},
	}

	assert.Equal(t, "istio_requests_total", metric.Name)
	assert.Equal(t, float64(100), metric.Value)
	assert.Equal(t, model.MetricTypeCounter, metric.Type)
	assert.NotNil(t, metric.RoutingInfo)
	assert.Equal(t, "my-service", metric.RoutingInfo.DestinationService)
}

func TestEnrichedTrafficMetricModel(t *testing.T) {
	enriched := model.EnrichedTrafficMetric{
		RawTrafficMetric: model.RawTrafficMetric{
			Name:  "istio_requests_total",
			Value: 100,
		},
		WorkloadInfo: &model.WorkloadInfo{
			ServiceName:      "my-service",
			ServiceNamespace: "default",
			PodName:          "my-service-xxx",
			PodIP:            "10.0.0.1",
			NodeName:         "node-1",
			WorkloadName:     "my-workload",
			WorkloadOwner:    "user@example.com",
			WorkloadType:     "Deployment",
		},
	}

	assert.Equal(t, "istio_requests_total", enriched.Name)
	assert.NotNil(t, enriched.WorkloadInfo)
	assert.Equal(t, "my-workload", enriched.WorkloadInfo.WorkloadName)
	assert.Equal(t, "user@example.com", enriched.WorkloadInfo.WorkloadOwner)
}

func TestNewHigressCollectorFactory(t *testing.T) {
	factory := NewHigressCollectorFactory()
	require.NotNil(t, factory)

	config := &collector.CollectorConfig{
		Type:      collector.GatewayTypeHigress,
		Namespace: "test-ns",
	}

	c, err := factory("test-collector", config, nil)
	require.NoError(t, err)
	assert.Equal(t, collector.GatewayTypeHigress, c.Type())
	assert.Equal(t, "test-collector", c.Name())
}

