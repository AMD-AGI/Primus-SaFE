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
	// Verify general gateway labels (without workload info)
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
		"primus_lens_cluster",
	}

	assert.Equal(t, expectedLabels, MetricLabels)
	assert.Len(t, MetricLabels, 11)
}

func TestWorkloadMetricLabels(t *testing.T) {
	// Verify workload gateway labels (with workload info)
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
		"workload_name",
		"workload_uid",
		"workload_owner",
		"primus_lens_cluster",
	}

	assert.Equal(t, expectedLabels, WorkloadMetricLabels)
	assert.Len(t, WorkloadMetricLabels, 14)
}

func TestNewExporter(t *testing.T) {
	t.Run("creates exporter with empty config", func(t *testing.T) {
		config := &gwconfig.GatewayExporterConfig{}
		manager := collector.NewManager(nil)
		enr := enricher.NewEnricher(nil, 60*time.Second, []string{"app"})

		exp := NewExporter(manager, enr, config)

		assert.NotNil(t, exp)
		// General gateway metrics
		assert.NotNil(t, exp.requestsTotal)
		assert.NotNil(t, exp.requestDuration)
		assert.NotNil(t, exp.requestBytes)
		assert.NotNil(t, exp.responseBytes)
		// Workload gateway metrics
		assert.NotNil(t, exp.workloadRequestsTotal)
		assert.NotNil(t, exp.workloadRequestDuration)
		assert.NotNil(t, exp.workloadRequestBytes)
		assert.NotNil(t, exp.workloadResponseBytes)
		// Collector metrics
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

	t.Run("builds general labels without workload info", func(t *testing.T) {
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
				// No WorkloadUID means this is not a GPU workload
			},
		}

		labels := exp.buildLabels(metric)

		assert.Len(t, labels, 10) // MetricLabels - 1 (primus_lens_cluster)
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

		assert.Len(t, labels, 10)
		assert.Equal(t, "higress", labels[0])
		assert.Equal(t, "higress-gateway-0", labels[1])
		// Routing info should be empty
		assert.Equal(t, "", labels[2])
		assert.Equal(t, "", labels[3])
	})
}

func TestExporter_BuildWorkloadLabels(t *testing.T) {
	config := &gwconfig.GatewayExporterConfig{}
	manager := collector.NewManager(nil)
	enr := enricher.NewEnricher(nil, 60*time.Second, []string{"app"})
	exp := NewExporter(manager, enr, config)

	t.Run("builds workload labels with GPU workload info", func(t *testing.T) {
		metric := model.EnrichedTrafficMetric{
			RawTrafficMetric: model.RawTrafficMetric{
				GatewayType:     "higress",
				GatewayInstance: "higress-gateway-0",
				RoutingInfo: &model.RoutingInfo{
					Host:         "inference.example.com",
					Path:         "/v1/predict",
					Method:       "POST",
					ResponseCode: "200",
				},
			},
			WorkloadInfo: &model.WorkloadInfo{
				ServiceName:      "inference-service",
				ServiceNamespace: "ai-workloads",
				PodName:          "inference-service-xyz789",
				NodeName:         "gpu-node-01",
				WorkloadName:     "llama-inference",
				WorkloadUID:      "wl-uid-12345",
				WorkloadOwner:    "ml-team@example.com",
			},
		}

		labels := exp.buildWorkloadLabels(metric)

		assert.Len(t, labels, 13) // WorkloadMetricLabels - 1 (primus_lens_cluster)
		assert.Equal(t, "higress", labels[0])
		assert.Equal(t, "higress-gateway-0", labels[1])
		assert.Equal(t, "inference.example.com", labels[2])
		assert.Equal(t, "/v1/predict", labels[3])
		assert.Equal(t, "POST", labels[4])
		assert.Equal(t, "200", labels[5])
		assert.Equal(t, "inference-service", labels[6])
		assert.Equal(t, "ai-workloads", labels[7])
		assert.Equal(t, "inference-service-xyz789", labels[8])
		assert.Equal(t, "gpu-node-01", labels[9])
		assert.Equal(t, "llama-inference", labels[10])
		assert.Equal(t, "wl-uid-12345", labels[11])
		assert.Equal(t, "ml-team@example.com", labels[12])
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

	t.Run("routes to general metrics when no GPU workload", func(t *testing.T) {
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
					// No WorkloadUID - not a GPU workload
				},
			},
		}

		assert.NotPanics(t, func() {
			exp.updatePrometheusMetrics(metrics)
		})
	})

	t.Run("routes to workload metrics when GPU workload present", func(t *testing.T) {
		metrics := []model.EnrichedTrafficMetric{
			{
				RawTrafficMetric: model.RawTrafficMetric{
					Name:            "istio_requests_total",
					Value:           50,
					GatewayType:     "istio",
					GatewayInstance: "istio-gateway-0",
					RoutingInfo: &model.RoutingInfo{
						Host:         "inference.example.com",
						Path:         "/predict",
						Method:       "POST",
						ResponseCode: "200",
					},
				},
				WorkloadInfo: &model.WorkloadInfo{
					ServiceName:      "inference-service",
					ServiceNamespace: "ai-workloads",
					WorkloadName:     "llama-inference",
					WorkloadUID:      "wl-uid-12345", // Has WorkloadUID - is a GPU workload
					WorkloadOwner:    "ml-team",
				},
			},
		}

		assert.NotPanics(t, func() {
			exp.updatePrometheusMetrics(metrics)
		})
	})

	t.Run("handles envoy format metrics", func(t *testing.T) {
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

func TestWorkloadInfo_HasGpuWorkload(t *testing.T) {
	t.Run("returns false for nil", func(t *testing.T) {
		var info *model.WorkloadInfo
		assert.False(t, info.HasGpuWorkload())
	})

	t.Run("returns false when WorkloadUID is empty", func(t *testing.T) {
		info := &model.WorkloadInfo{
			ServiceName:      "test-service",
			ServiceNamespace: "default",
			WorkloadName:     "my-workload",
			WorkloadUID:      "", // Empty
		}
		assert.False(t, info.HasGpuWorkload())
	})

	t.Run("returns true when WorkloadUID is set", func(t *testing.T) {
		info := &model.WorkloadInfo{
			ServiceName:      "test-service",
			ServiceNamespace: "default",
			WorkloadName:     "my-workload",
			WorkloadUID:      "wl-uid-12345",
		}
		assert.True(t, info.HasGpuWorkload())
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
			WorkloadName:     "llama-inference",
			WorkloadUID:      "uid-456",
			WorkloadOwner:    "team-a",
			WorkloadType:     "Deployment",
		}

		assert.Equal(t, "my-service", info.ServiceName)
		assert.Equal(t, "production", info.ServiceNamespace)
		assert.Equal(t, "my-service-pod-abc", info.PodName)
		assert.Equal(t, "10.0.0.1", info.PodIP)
		assert.Equal(t, "node-1", info.NodeName)
		assert.Equal(t, "llama-inference", info.WorkloadName)
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
