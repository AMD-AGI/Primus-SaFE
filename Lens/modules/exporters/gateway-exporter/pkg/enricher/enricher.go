// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package enricher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
)

// Enricher enriches raw traffic metrics with workload information from database
type Enricher struct {
	// In-memory cache for faster lookups
	servicePodCache     map[string]*CachedServiceInfo
	servicePodCacheLock sync.RWMutex

	// Cache refresh interval
	cacheRefreshInterval time.Duration
	lastCacheRefresh     time.Time

	// Workload labels to extract
	workloadLabels []string
}

// CachedServiceInfo stores service to pod mapping from database
type CachedServiceInfo struct {
	ServiceName      string
	ServiceNamespace string
	ServiceUID       string
	Pods             []CachedPodInfo
	UpdatedAt        time.Time
}

// CachedPodInfo stores pod information from database
type CachedPodInfo struct {
	PodName       string
	PodUID        string
	PodIP         string
	NodeName      string
	Namespace     string
	Labels        map[string]string
	WorkloadID    string
	WorkloadUID   string
	WorkloadOwner string
	WorkloadType  string
}

// NewEnricher creates a new enricher
func NewEnricher(_ interface{}, cacheRefreshInterval time.Duration, workloadLabels []string) *Enricher {
	return &Enricher{
		servicePodCache:      make(map[string]*CachedServiceInfo),
		cacheRefreshInterval: cacheRefreshInterval,
		workloadLabels:       workloadLabels,
	}
}

// RefreshCache loads service-pod mappings from database into memory cache
func (e *Enricher) RefreshCache(ctx context.Context) error {
	log.Info("Refreshing enricher cache from database")

	// Get all service-pod references from database
	servicePodRefs, err := database.GetFacade().GetK8sService().GetAllServicePodReferences(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service-pod references: %w", err)
	}

	// Build pod_uid -> workload_uid mapping from workload_pod_reference table
	podUIDToWorkloadUID := make(map[string]string)
	workloadPodRefs, err := database.GetFacade().GetWorkload().GetAllWorkloadPodReferences(ctx)
	if err != nil {
		log.Warnf("Failed to get workload-pod references for workload_uid mapping: %v", err)
		// Continue without workload_uid mapping
	} else {
		for _, wpr := range workloadPodRefs {
			podUIDToWorkloadUID[wpr.PodUID] = wpr.WorkloadUID
		}
	}

	// Build new cache
	newCache := make(map[string]*CachedServiceInfo)

	for _, ref := range servicePodRefs {
		cacheKey := fmt.Sprintf("%s/%s", ref.ServiceNamespace, ref.ServiceName)

		if _, ok := newCache[cacheKey]; !ok {
			newCache[cacheKey] = &CachedServiceInfo{
				ServiceName:      ref.ServiceName,
				ServiceNamespace: ref.ServiceNamespace,
				ServiceUID:       ref.ServiceUID,
				Pods:             []CachedPodInfo{},
				UpdatedAt:        time.Now(),
			}
		}

		// Extract labels from ref.PodLabels
		labels := make(map[string]string)
		if ref.PodLabels != nil {
			for k, v := range ref.PodLabels {
				if str, ok := v.(string); ok {
					labels[k] = str
				}
			}
		}

		cachedPod := CachedPodInfo{
			PodName:      ref.PodName,
			PodUID:       ref.PodUID,
			PodIP:        ref.PodIP,
			NodeName:     ref.NodeName,
			Namespace:    ref.ServiceNamespace,
			Labels:       labels,
			WorkloadID:   ref.WorkloadID,
			WorkloadUID:  podUIDToWorkloadUID[ref.PodUID], // Get workload_uid from mapping
			WorkloadType: ref.WorkloadType,
		}

		// Extract workload owner from labels
		if owner, ok := labels["primus-safe.user.name"]; ok {
			cachedPod.WorkloadOwner = owner
		}

		newCache[cacheKey].Pods = append(newCache[cacheKey].Pods, cachedPod)
	}

	// Swap cache atomically
	e.servicePodCacheLock.Lock()
	e.servicePodCache = newCache
	e.lastCacheRefresh = time.Now()
	e.servicePodCacheLock.Unlock()

	log.Infof("Enricher cache refreshed, loaded %d services", len(newCache))
	return nil
}

// StartCacheRefreshLoop starts the periodic cache refresh
func (e *Enricher) StartCacheRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(e.cacheRefreshInterval)
	defer ticker.Stop()

	// Initial refresh
	if err := e.RefreshCache(ctx); err != nil {
		log.Errorf("Initial cache refresh failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.RefreshCache(ctx); err != nil {
				log.Errorf("Cache refresh failed: %v", err)
			}
		}
	}
}

// Enrich adds workload information to raw traffic metrics using database cache
func (e *Enricher) Enrich(ctx context.Context, metrics []model.RawTrafficMetric) ([]model.EnrichedTrafficMetric, error) {
	result := make([]model.EnrichedTrafficMetric, 0, len(metrics))

	for _, metric := range metrics {
		enriched := model.EnrichedTrafficMetric{
			RawTrafficMetric: metric,
		}

		if metric.RoutingInfo != nil && metric.RoutingInfo.DestinationService != "" {
			workloadInfo := e.resolveWorkloadFromCache(
				metric.RoutingInfo.DestinationService,
				metric.RoutingInfo.DestinationNamespace)
			if workloadInfo != nil {
				enriched.WorkloadInfo = workloadInfo
			}
		}

		result = append(result, enriched)
	}

	return result, nil
}

// resolveWorkloadFromCache looks up service-pod mapping from in-memory cache
func (e *Enricher) resolveWorkloadFromCache(serviceName, namespace string) *model.WorkloadInfo {
	cacheKey := fmt.Sprintf("%s/%s", namespace, serviceName)

	e.servicePodCacheLock.RLock()
	cached, ok := e.servicePodCache[cacheKey]
	e.servicePodCacheLock.RUnlock()

	if !ok || len(cached.Pods) == 0 {
		return nil
	}

	// Use the first available pod
	pod := cached.Pods[0]

	return &model.WorkloadInfo{
		ServiceName:      cached.ServiceName,
		ServiceNamespace: cached.ServiceNamespace,
		PodName:          pod.PodName,
		PodIP:            pod.PodIP,
		NodeName:         pod.NodeName,
		WorkloadName:     pod.WorkloadID, // WorkloadID from service_pod_reference is the workload name
		WorkloadUID:      pod.WorkloadUID,
		WorkloadOwner:    pod.WorkloadOwner,
		WorkloadType:     pod.WorkloadType,
	}
}

// ClearCache clears the service cache
func (e *Enricher) ClearCache() {
	e.servicePodCacheLock.Lock()
	defer e.servicePodCacheLock.Unlock()
	e.servicePodCache = make(map[string]*CachedServiceInfo)
}

// CacheStats returns cache statistics
func (e *Enricher) CacheStats() (size int, oldestEntry time.Time) {
	e.servicePodCacheLock.RLock()
	defer e.servicePodCacheLock.RUnlock()

	size = len(e.servicePodCache)
	for _, info := range e.servicePodCache {
		if oldestEntry.IsZero() || info.UpdatedAt.Before(oldestEntry) {
			oldestEntry = info.UpdatedAt
		}
	}

	return
}
