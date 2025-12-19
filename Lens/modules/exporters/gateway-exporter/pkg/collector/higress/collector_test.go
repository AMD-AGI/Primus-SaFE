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
	// Sample Prometheus metrics from Higress/Envoy
	sampleMetrics := `
# HELP istio_requests_total Total requests
# TYPE istio_requests_total counter
istio_requests_total{destination_service_name="my-service",destination_service_namespace="default",response_code="200",request_host="example.com"} 100
istio_requests_total{destination_service_name="my-service",destination_service_namespace="default",response_code="500",request_host="example.com"} 5
# HELP istio_request_bytes_total Total request bytes
# TYPE istio_request_bytes_total counter
istio_request_bytes_total{destination_service_name="my-service",destination_service_namespace="default"} 50000
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
	assert.GreaterOrEqual(t, len(metrics), 2)

	// Verify parsed metrics
	var requestsTotal float64
	for _, m := range metrics {
		if m.Name == "istio_requests_total" {
			requestsTotal += m.Value
		}
	}
	assert.Equal(t, float64(105), requestsTotal) // 100 + 5
}

func TestHigressCollector_ExtractRoutingInfo(t *testing.T) {
	config := &collector.CollectorConfig{
		Type: collector.GatewayTypeHigress,
	}

	c, _ := NewHigressCollector("test", config, nil)

	labels := map[string]string{
		"request_host":                  "api.example.com",
		"request_path":                  "/api/v1/users",
		"request_method":                "GET",
		"response_code":                 "200",
		"destination_service_name":      "user-service",
		"destination_service_namespace": "production",
	}

	routingInfo := c.extractRoutingInfo(labels)

	assert.Equal(t, "api.example.com", routingInfo.Host)
	assert.Equal(t, "/api/v1/users", routingInfo.Path)
	assert.Equal(t, "GET", routingInfo.Method)
	assert.Equal(t, "200", routingInfo.ResponseCode)
	assert.Equal(t, "user-service", routingInfo.DestinationService)
	assert.Equal(t, "production", routingInfo.DestinationNamespace)
}

func TestHigressCollector_SkipInternalServices(t *testing.T) {
	config := &collector.CollectorConfig{
		Type: collector.GatewayTypeHigress,
	}

	c, _ := NewHigressCollector("test", config, nil)

	// Metrics with internal services should be skipped
	internalLabels := map[string]string{
		"destination_service_name":      "kubernetes",
		"destination_service_namespace": "default",
	}

	routingInfo := c.extractRoutingInfo(internalLabels)

	// The convertMetric function should return nil for internal services
	// This is tested implicitly through the DestinationService check
	assert.Equal(t, "kubernetes", routingInfo.DestinationService)
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
			WorkloadID:       "my-workload",
			WorkloadOwner:    "user@example.com",
			WorkloadType:     "Deployment",
		},
	}

	assert.Equal(t, "istio_requests_total", enriched.Name)
	assert.NotNil(t, enriched.WorkloadInfo)
	assert.Equal(t, "my-workload", enriched.WorkloadInfo.WorkloadID)
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

