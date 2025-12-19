package exporter

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/collector"
	gwconfig "github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/enricher"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestMetricLabels(t *testing.T) {
	// Verify all expected labels are present
	expectedLabels := []string{
		"gateway_type",
		"gateway_instance",
		"host",
		"path",
		"method",
		"response_code",
		"service_name",
		"service_namespace",
		"pod_name",
		"node_name",
		"workload_id",
		"workload_uid",
		"workload_owner",
		"primus_lens_cluster",
	}

	assert.Equal(t, expectedLabels, MetricLabels)
	assert.Len(t, MetricLabels, 14)
}

func TestNewExporter(t *testing.T) {
	t.Run("creates exporter with empty config", func(t *testing.T) {
		config := &gwconfig.GatewayExporterConfig{}
		manager := collector.NewManager(nil)
		enr := enricher.NewEnricher(nil, 60*time.Second, []string{"app"})

		exp := NewExporter(manager, enr, config)

		assert.NotNil(t, exp)
		assert.NotNil(t, exp.requestsTotal)
		assert.NotNil(t, exp.requestDuration)
		assert.NotNil(t, exp.requestBytes)
		assert.NotNil(t, exp.responseBytes)
		assert.NotNil(t, exp.scrapeTotal)
		assert.NotNil(t, exp.scrapeDuration)
		assert.NotNil(t, exp.scrapeErrors)
		assert.NotNil(t, exp.registry)
	})

	t.Run("creates exporter with cluster name", func(t *testing.T) {
		config := &gwconfig.GatewayExporterConfig{
			Metrics: gwconfig.MetricsConfig{
				StaticLabels: map[string]string{
					"primus_lens_cluster": "test-cluster",
				},
			},
		}
		manager := collector.NewManager(nil)
		enr := enricher.NewEnricher(nil, 60*time.Second, []string{"app"})

		exp := NewExporter(manager, enr, config)

		assert.NotNil(t, exp)
	})
}

func TestExporter_BuildLabels(t *testing.T) {
	config := &gwconfig.GatewayExporterConfig{}
	manager := collector.NewManager(nil)
	enr := enricher.NewEnricher(nil, 60*time.Second, []string{"app"})
	exp := NewExporter(manager, enr, config)

	t.Run("builds labels with all fields", func(t *testing.T) {
		metric := model.EnrichedTrafficMetric{
			RawTrafficMetric: model.RawTrafficMetric{
				GatewayType:     "higress",
				GatewayInstance: "higress-gateway-0",
				RoutingInfo: &model.RoutingInfo{
					Host:         "api.example.com",
					Path:         "/v1/users",
					Method:       "GET",
					ResponseCode: "200",
				},
			},
			WorkloadInfo: &model.WorkloadInfo{
				ServiceName:      "user-service",
				ServiceNamespace: "default",
				PodName:          "user-service-abc123",
				NodeName:         "node-1",
				WorkloadID:       "workload-123",
				WorkloadUID:      "uid-456",
				WorkloadOwner:    "user@example.com",
			},
		}

		labels := exp.buildLabels(metric)

		assert.Len(t, labels, 13) // MetricLabels - 1 (primus_lens_cluster)
		assert.Equal(t, "higress", labels[0])
		assert.Equal(t, "higress-gateway-0", labels[1])
		assert.Equal(t, "api.example.com", labels[2])
		assert.Equal(t, "/v1/users", labels[3])
		assert.Equal(t, "GET", labels[4])
		assert.Equal(t, "200", labels[5])
		assert.Equal(t, "user-service", labels[6])
		assert.Equal(t, "default", labels[7])
		assert.Equal(t, "user-service-abc123", labels[8])
		assert.Equal(t, "node-1", labels[9])
		assert.Equal(t, "workload-123", labels[10])
		assert.Equal(t, "uid-456", labels[11])
		assert.Equal(t, "user@example.com", labels[12])
	})

	t.Run("builds labels with nil routing info", func(t *testing.T) {
		metric := model.EnrichedTrafficMetric{
			RawTrafficMetric: model.RawTrafficMetric{
				GatewayType:     "higress",
				GatewayInstance: "higress-gateway-0",
				RoutingInfo:     nil,
			},
			WorkloadInfo: nil,
		}

		labels := exp.buildLabels(metric)

		assert.Len(t, labels, 13)
		assert.Equal(t, "higress", labels[0])
		assert.Equal(t, "higress-gateway-0", labels[1])
		// Routing info should be empty
		assert.Equal(t, "", labels[2])
		assert.Equal(t, "", labels[3])
	})
}

func TestExporter_Register(t *testing.T) {
	config := &gwconfig.GatewayExporterConfig{}
	manager := collector.NewManager(nil)
	enr := enricher.NewEnricher(nil, 60*time.Second, []string{"app"})
	exp := NewExporter(manager, enr, config)

	// Register should not panic
	assert.NotPanics(t, func() {
		exp.Register()
	})
}

func TestExporter_Gather(t *testing.T) {
	config := &gwconfig.GatewayExporterConfig{}
	manager := collector.NewManager(nil)
	enr := enricher.NewEnricher(nil, 60*time.Second, []string{"app"})
	exp := NewExporter(manager, enr, config)
	exp.Register()

	t.Run("gathers metrics without error", func(t *testing.T) {
		metrics, err := exp.Gather()
		assert.NoError(t, err)
		assert.NotNil(t, metrics)
	})
}

func TestExporter_UpdatePrometheusMetrics(t *testing.T) {
	config := &gwconfig.GatewayExporterConfig{}
	manager := collector.NewManager(nil)
	enr := enricher.NewEnricher(nil, 60*time.Second, []string{"app"})
	exp := NewExporter(manager, enr, config)
	exp.Register()

	t.Run("updates metrics for istio format", func(t *testing.T) {
		metrics := []model.EnrichedTrafficMetric{
			{
				RawTrafficMetric: model.RawTrafficMetric{
					Name:            "istio_requests_total",
					Value:           100,
					GatewayType:     "istio",
					GatewayInstance: "istio-gateway-0",
					RoutingInfo: &model.RoutingInfo{
						Host:         "api.example.com",
						Path:         "/api",
						Method:       "GET",
						ResponseCode: "200",
					},
				},
				WorkloadInfo: &model.WorkloadInfo{
					ServiceName:      "api-service",
					ServiceNamespace: "default",
				},
			},
		}

		// Should not panic
		assert.NotPanics(t, func() {
			exp.updatePrometheusMetrics(metrics)
		})
	})

	t.Run("updates metrics for envoy format", func(t *testing.T) {
		metrics := []model.EnrichedTrafficMetric{
			{
				RawTrafficMetric: model.RawTrafficMetric{
					Name:            "envoy_cluster_upstream_rq",
					Value:           50,
					GatewayType:     "envoy",
					GatewayInstance: "envoy-0",
				},
			},
			{
				RawTrafficMetric: model.RawTrafficMetric{
					Name:            "envoy_cluster_upstream_rq_200",
					Value:           45,
					GatewayType:     "envoy",
					GatewayInstance: "envoy-0",
				},
			},
		}

		assert.NotPanics(t, func() {
			exp.updatePrometheusMetrics(metrics)
		})
	})
}

func TestModel_RawTrafficMetric(t *testing.T) {
	t.Run("can create RawTrafficMetric with all fields", func(t *testing.T) {
		metric := model.RawTrafficMetric{
			Name:            "test_metric",
			Value:           42.5,
			Type:            model.MetricTypeCounter,
			Timestamp:       time.Now(),
			GatewayType:     "higress",
			GatewayInstance: "higress-0",
			OriginalLabels: map[string]string{
				"label1": "value1",
			},
			RoutingInfo: &model.RoutingInfo{
				Host:   "example.com",
				Path:   "/api",
				Method: "POST",
			},
		}

		assert.Equal(t, "test_metric", metric.Name)
		assert.Equal(t, 42.5, metric.Value)
		assert.Equal(t, model.MetricTypeCounter, metric.Type)
		assert.Equal(t, "higress", metric.GatewayType)
		assert.NotNil(t, metric.RoutingInfo)
	})
}

func TestModel_WorkloadInfo(t *testing.T) {
	t.Run("can create WorkloadInfo with all fields", func(t *testing.T) {
		info := model.WorkloadInfo{
			ServiceName:      "my-service",
			ServiceNamespace: "production",
			PodName:          "my-service-pod-abc",
			PodIP:            "10.0.0.1",
			NodeName:         "node-1",
			WorkloadID:       "wl-123",
			WorkloadUID:      "uid-456",
			WorkloadOwner:    "team-a",
			WorkloadType:     "Deployment",
		}

		assert.Equal(t, "my-service", info.ServiceName)
		assert.Equal(t, "production", info.ServiceNamespace)
		assert.Equal(t, "my-service-pod-abc", info.PodName)
		assert.Equal(t, "10.0.0.1", info.PodIP)
		assert.Equal(t, "node-1", info.NodeName)
		assert.Equal(t, "wl-123", info.WorkloadID)
		assert.Equal(t, "uid-456", info.WorkloadUID)
		assert.Equal(t, "team-a", info.WorkloadOwner)
		assert.Equal(t, "Deployment", info.WorkloadType)
	})
}

func TestModel_MetricTypes(t *testing.T) {
	assert.Equal(t, model.MetricType("counter"), model.MetricTypeCounter)
	assert.Equal(t, model.MetricType("gauge"), model.MetricTypeGauge)
	assert.Equal(t, model.MetricType("histogram"), model.MetricTypeHistogram)
}
