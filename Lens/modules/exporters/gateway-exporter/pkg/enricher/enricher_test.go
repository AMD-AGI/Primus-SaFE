package enricher

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestNewEnricher(t *testing.T) {
	workloadLabels := []string{"primus-safe.workload.id", "primus-safe.user.name"}
	e := NewEnricher(nil, 60*time.Second, workloadLabels)

	assert.NotNil(t, e)
	assert.NotNil(t, e.servicePodCache)
	assert.Equal(t, 60*time.Second, e.cacheRefreshInterval)
	assert.Equal(t, workloadLabels, e.workloadLabels)
}

func TestEnricher_ResolveWorkloadFromCache(t *testing.T) {
	e := NewEnricher(nil, 60*time.Second, nil)

	// Populate cache
	e.servicePodCache["default/my-service"] = &CachedServiceInfo{
		ServiceName:      "my-service",
		ServiceNamespace: "default",
		ServiceUID:       "svc-uid-123",
		Pods: []CachedPodInfo{
			{
				PodName:       "my-service-abc123",
				PodUID:        "pod-uid-123",
				PodIP:         "10.0.0.1",
				NodeName:      "node-1",
				WorkloadID:    "my-workload-id",
				WorkloadOwner: "testuser",
				WorkloadType:  "Deployment",
			},
		},
		UpdatedAt: time.Now(),
	}

	// Test cache hit
	workloadInfo := e.resolveWorkloadFromCache("my-service", "default")
	assert.NotNil(t, workloadInfo)
	assert.Equal(t, "my-service", workloadInfo.ServiceName)
	assert.Equal(t, "default", workloadInfo.ServiceNamespace)
	assert.Equal(t, "my-service-abc123", workloadInfo.PodName)
	assert.Equal(t, "10.0.0.1", workloadInfo.PodIP)
	assert.Equal(t, "my-workload-id", workloadInfo.WorkloadID)
	assert.Equal(t, "testuser", workloadInfo.WorkloadOwner)

	// Test cache miss
	workloadInfo = e.resolveWorkloadFromCache("non-existent", "default")
	assert.Nil(t, workloadInfo)
}

func TestEnricher_Enrich(t *testing.T) {
	e := NewEnricher(nil, 60*time.Second, nil)

	// Populate cache
	e.servicePodCache["production/api-service"] = &CachedServiceInfo{
		ServiceName:      "api-service",
		ServiceNamespace: "production",
		Pods: []CachedPodInfo{
			{
				PodName:       "api-service-pod-1",
				PodIP:         "10.0.0.5",
				NodeName:      "worker-node-1",
				WorkloadID:    "api-workload",
				WorkloadOwner: "api-team",
				WorkloadType:  "Deployment",
			},
		},
		UpdatedAt: time.Now(),
	}

	rawMetrics := []model.RawTrafficMetric{
		{
			Name:  "istio_requests_total",
			Value: 100,
			Type:  model.MetricTypeCounter,
			RoutingInfo: &model.RoutingInfo{
				DestinationService:   "api-service",
				DestinationNamespace: "production",
			},
		},
		{
			Name:  "istio_requests_total",
			Value: 50,
			Type:  model.MetricTypeCounter,
			RoutingInfo: &model.RoutingInfo{
				DestinationService:   "unknown-service",
				DestinationNamespace: "production",
			},
		},
	}

	enrichedMetrics, err := e.Enrich(context.Background(), rawMetrics)
	assert.NoError(t, err)
	assert.Len(t, enrichedMetrics, 2)

	// First metric should have workload info
	assert.NotNil(t, enrichedMetrics[0].WorkloadInfo)
	assert.Equal(t, "api-workload", enrichedMetrics[0].WorkloadInfo.WorkloadID)

	// Second metric should not have workload info (cache miss)
	assert.Nil(t, enrichedMetrics[1].WorkloadInfo)
}

func TestEnricher_CacheStats(t *testing.T) {
	e := NewEnricher(nil, 60*time.Second, nil)

	// Empty cache
	size, oldest := e.CacheStats()
	assert.Equal(t, 0, size)
	assert.True(t, oldest.IsZero())

	// Add some entries
	now := time.Now()
	e.servicePodCache["ns1/svc1"] = &CachedServiceInfo{
		ServiceName: "svc1",
		UpdatedAt:   now.Add(-10 * time.Minute),
	}
	e.servicePodCache["ns2/svc2"] = &CachedServiceInfo{
		ServiceName: "svc2",
		UpdatedAt:   now.Add(-5 * time.Minute),
	}

	size, oldest = e.CacheStats()
	assert.Equal(t, 2, size)
	assert.WithinDuration(t, now.Add(-10*time.Minute), oldest, time.Second)
}

func TestEnricher_ClearCache(t *testing.T) {
	e := NewEnricher(nil, 60*time.Second, nil)

	// Add entries
	e.servicePodCache["ns/svc"] = &CachedServiceInfo{
		ServiceName: "svc",
		UpdatedAt:   time.Now(),
	}

	size, _ := e.CacheStats()
	assert.Equal(t, 1, size)

	// Clear
	e.ClearCache()

	size, _ = e.CacheStats()
	assert.Equal(t, 0, size)
}

func TestCachedServiceInfo(t *testing.T) {
	info := &CachedServiceInfo{
		ServiceName:      "test-service",
		ServiceNamespace: "test-namespace",
		ServiceUID:       "uid-123",
		Pods: []CachedPodInfo{
			{
				PodName:  "pod-1",
				PodIP:    "10.0.0.1",
				NodeName: "node-1",
			},
			{
				PodName:  "pod-2",
				PodIP:    "10.0.0.2",
				NodeName: "node-2",
			},
		},
		UpdatedAt: time.Now(),
	}

	assert.Equal(t, "test-service", info.ServiceName)
	assert.Len(t, info.Pods, 2)
}

func TestCachedPodInfo(t *testing.T) {
	pod := CachedPodInfo{
		PodName:       "my-pod",
		PodUID:        "pod-uid",
		PodIP:         "10.0.0.1",
		NodeName:      "node-1",
		Namespace:     "default",
		Labels:        map[string]string{"app": "test"},
		WorkloadID:    "workload-123",
		WorkloadOwner: "owner@test.com",
		WorkloadType:  "Deployment",
	}

	assert.Equal(t, "my-pod", pod.PodName)
	assert.Equal(t, "workload-123", pod.WorkloadID)
	assert.Equal(t, "Deployment", pod.WorkloadType)
}

