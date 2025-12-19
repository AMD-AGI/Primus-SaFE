package enricher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Enricher enriches raw traffic metrics with workload information
type Enricher struct {
	k8sClient client.Client

	// Cache for service -> endpoints mapping
	serviceCache     map[string]*ServiceInfo
	serviceCacheLock sync.RWMutex

	// Configuration
	cacheTTL       time.Duration
	workloadLabels []string
}

// ServiceInfo contains cached service information
type ServiceInfo struct {
	Name      string
	Namespace string
	Endpoints []EndpointInfo
	UpdatedAt time.Time
}

// EndpointInfo contains endpoint information
type EndpointInfo struct {
	PodName  string
	PodIP    string
	NodeName string
	Port     int
	Labels   map[string]string
}

// NewEnricher creates a new enricher
func NewEnricher(k8sClient client.Client, cacheTTL time.Duration, workloadLabels []string) *Enricher {
	return &Enricher{
		k8sClient:      k8sClient,
		serviceCache:   make(map[string]*ServiceInfo),
		cacheTTL:       cacheTTL,
		workloadLabels: workloadLabels,
	}
}

// Enrich adds workload information to raw traffic metrics
func (e *Enricher) Enrich(ctx context.Context, metrics []model.RawTrafficMetric) ([]model.EnrichedTrafficMetric, error) {
	result := make([]model.EnrichedTrafficMetric, 0, len(metrics))

	for _, metric := range metrics {
		enriched := model.EnrichedTrafficMetric{
			RawTrafficMetric: metric,
		}

		if metric.RoutingInfo != nil && metric.RoutingInfo.DestinationService != "" {
			workloadInfo, err := e.resolveWorkload(ctx,
				metric.RoutingInfo.DestinationService,
				metric.RoutingInfo.DestinationNamespace)
			if err == nil {
				enriched.WorkloadInfo = workloadInfo
			}
		}

		result = append(result, enriched)
	}

	return result, nil
}

func (e *Enricher) resolveWorkload(ctx context.Context, serviceName, namespace string) (*model.WorkloadInfo, error) {
	cacheKey := fmt.Sprintf("%s/%s", namespace, serviceName)

	// Check cache first
	e.serviceCacheLock.RLock()
	cached, ok := e.serviceCache[cacheKey]
	e.serviceCacheLock.RUnlock()

	if ok && time.Since(cached.UpdatedAt) < e.cacheTTL {
		return e.buildWorkloadInfo(cached), nil
	}

	// Fetch from API
	serviceInfo, err := e.fetchServiceInfo(ctx, serviceName, namespace)
	if err != nil {
		return nil, err
	}

	// Update cache
	e.serviceCacheLock.Lock()
	e.serviceCache[cacheKey] = serviceInfo
	e.serviceCacheLock.Unlock()

	return e.buildWorkloadInfo(serviceInfo), nil
}

func (e *Enricher) fetchServiceInfo(ctx context.Context, serviceName, namespace string) (*ServiceInfo, error) {
	// Get service
	svc := &corev1.Service{}
	if err := e.k8sClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, svc); err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	// Get endpoints
	endpoints := &corev1.Endpoints{}
	if err := e.k8sClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, endpoints); err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	serviceInfo := &ServiceInfo{
		Name:      serviceName,
		Namespace: namespace,
		UpdatedAt: time.Now(),
	}

	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			ep := EndpointInfo{
				PodIP: addr.IP,
			}

			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
				ep.PodName = addr.TargetRef.Name
			}

			if addr.NodeName != nil {
				ep.NodeName = *addr.NodeName
			}

			// Get pod labels if we have pod reference
			if ep.PodName != "" {
				pod := &corev1.Pod{}
				if err := e.k8sClient.Get(ctx, client.ObjectKey{
					Name:      ep.PodName,
					Namespace: namespace,
				}, pod); err == nil {
					ep.Labels = pod.Labels
				}
			}

			for _, port := range subset.Ports {
				epWithPort := ep
				epWithPort.Port = int(port.Port)
				serviceInfo.Endpoints = append(serviceInfo.Endpoints, epWithPort)
			}
		}
	}

	return serviceInfo, nil
}

func (e *Enricher) buildWorkloadInfo(serviceInfo *ServiceInfo) *model.WorkloadInfo {
	if serviceInfo == nil || len(serviceInfo.Endpoints) == 0 {
		return nil
	}

	// Use the first endpoint
	ep := serviceInfo.Endpoints[0]

	workloadInfo := &model.WorkloadInfo{
		ServiceName:      serviceInfo.Name,
		ServiceNamespace: serviceInfo.Namespace,
		PodName:          ep.PodName,
		PodIP:            ep.PodIP,
		NodeName:         ep.NodeName,
	}

	// Extract workload labels
	if ep.Labels != nil {
		for _, labelKey := range e.workloadLabels {
			if value, ok := ep.Labels[labelKey]; ok {
				switch labelKey {
				case "primus-safe.workload.id":
					workloadInfo.WorkloadID = value
				case "primus-safe.user.name":
					workloadInfo.WorkloadOwner = value
				}
			}
		}
	}

	return workloadInfo
}

// ClearCache clears the service cache
func (e *Enricher) ClearCache() {
	e.serviceCacheLock.Lock()
	defer e.serviceCacheLock.Unlock()
	e.serviceCache = make(map[string]*ServiceInfo)
}

// CacheStats returns cache statistics
func (e *Enricher) CacheStats() (size int, oldestEntry time.Time) {
	e.serviceCacheLock.RLock()
	defer e.serviceCacheLock.RUnlock()

	size = len(e.serviceCache)
	for _, info := range e.serviceCache {
		if oldestEntry.IsZero() || info.UpdatedAt.Before(oldestEntry) {
			oldestEntry = info.UpdatedAt
		}
	}

	return
}

