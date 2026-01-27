// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package stale_pod_cleanup provides a job that cleans up stale "Running" pods in the database
// that no longer exist in Kubernetes. This handles cases where the exporter's reconcile loop
// misses pod deletion events, leaving ghost "Running" entries in the database.
package stale_pod_cleanup

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StalePodCleanupJob cleans up stale "Running" pods that no longer exist in Kubernetes
type StalePodCleanupJob struct{}

// NewStalePodCleanupJob creates a new StalePodCleanupJob instance
func NewStalePodCleanupJob() *StalePodCleanupJob {
	return &StalePodCleanupJob{}
}

// Run executes the stale pod cleanup job
func (j *StalePodCleanupJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	span, ctx := trace.StartSpanFromContext(ctx, "stale_pod_cleanup_job.Run")
	defer trace.FinishSpan(span)

	jobStartTime := time.Now()
	stats := common.NewExecutionStats()

	span.SetAttributes(
		attribute.String("job.name", "stale_pod_cleanup"),
		attribute.String("cluster.name", clientsets.GetClusterManager().GetCurrentClusterName()),
	)

	// Query pods marked as "Running" in the database
	listSpan, listCtx := trace.StartSpanFromContext(ctx, "listRunningPods")
	startTime := time.Now()
	runningPods, err := database.GetFacade().GetPod().ListRunningGpuPods(listCtx)
	duration := time.Since(startTime)

	if err != nil {
		listSpan.RecordError(err)
		listSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(listSpan)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to list running GPU pods")
		log.Errorf("Failed to list running GPU pods: %v", err)
		return stats, err
	}

	listSpan.SetAttributes(
		attribute.Int("running_pods_count", len(runningPods)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	listSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(listSpan)

	if len(runningPods) == 0 {
		stats.AddMessage("No running pods to check")
		span.SetStatus(codes.Ok, "No running pods")
		return stats, nil
	}

	// Group pods by namespace
	podsByNamespace := make(map[string][]*dbModel.GpuPods)
	for i := range runningPods {
		ns := runningPods[i].Namespace
		podsByNamespace[ns] = append(podsByNamespace[ns], runningPods[i])
	}

	log.Infof("Checking %d running pods across %d namespaces", len(runningPods), len(podsByNamespace))

	// Process each namespace concurrently
	processSpan, processCtx := trace.StartSpanFromContext(ctx, "processNamespaces")
	processSpan.SetAttributes(
		attribute.Int("pods_count", len(runningPods)),
		attribute.Int("namespace_count", len(podsByNamespace)),
	)

	startTime = time.Now()
	wg := &sync.WaitGroup{}
	for namespace, pods := range podsByNamespace {
		wg.Add(1)
		go func(ns string, nsPods []*dbModel.GpuPods) {
			defer wg.Done()
			err := j.checkNamespace(processCtx, ns, nsPods, clientSets, stats)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Failed to check namespace %s: %v", ns, err)
			}
		}(namespace, pods)
	}
	wg.Wait()
	duration = time.Since(startTime)

	processSpan.SetAttributes(
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Int64("cleaned_count", stats.ItemsUpdated),
	)
	processSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(processSpan)

	stats.RecordsProcessed = int64(len(runningPods))
	stats.AddCustomMetric("running_pods_checked", len(runningPods))
	stats.AddCustomMetric("namespaces_checked", len(podsByNamespace))
	stats.AddMessage("Stale pod cleanup completed")

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Int("running_pods_count", len(runningPods)),
		attribute.Int64("records_processed", stats.RecordsProcessed),
		attribute.Int64("items_cleaned", stats.ItemsUpdated),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)
	span.SetStatus(codes.Ok, "")

	if stats.ItemsUpdated > 0 {
		log.Infof("Cleaned up %d stale running pods", stats.ItemsUpdated)
	}

	return stats, nil
}

// Schedule returns the cron schedule for this job (every 5 minutes)
func (j *StalePodCleanupJob) Schedule() string {
	return "@every 5m"
}

// checkNamespace checks all running pods in a namespace against Kubernetes
func (j *StalePodCleanupJob) checkNamespace(
	ctx context.Context,
	namespace string,
	dbPods []*dbModel.GpuPods,
	clientSets *clientsets.K8SClientSet,
	stats *common.ExecutionStats,
) error {
	span, ctx := trace.StartSpanFromContext(ctx, "checkNamespace")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("namespace", namespace),
		attribute.Int("db_pods_count", len(dbPods)),
	)

	// List all pods in this namespace from K8s
	podList := &corev1.PodList{}
	startTime := time.Now()
	err := clientSets.ControllerRuntimeClient.List(ctx, podList, client.InNamespace(namespace))
	duration := time.Since(startTime)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Errorf("Failed to list pods in namespace %s: %v", namespace, err)
		return err
	}

	span.SetAttributes(
		attribute.Int("k8s_pods_count", len(podList.Items)),
		attribute.Float64("list_duration_ms", float64(duration.Milliseconds())),
	)

	// Build maps for quick lookup
	// Map by UID for exact match
	k8sPodByUID := make(map[string]*corev1.Pod)
	// Map by name for finding replacements
	k8sPodByName := make(map[string]*corev1.Pod)
	for i := range podList.Items {
		pod := &podList.Items[i]
		k8sPodByUID[string(pod.UID)] = pod
		k8sPodByName[pod.Name] = pod
	}

	// Check each DB pod
	cleanedCount := int64(0)
	for _, dbPod := range dbPods {
		// First check by UID (exact match)
		if _, exists := k8sPodByUID[dbPod.UID]; exists {
			// Pod still exists in K8s with same UID, skip
			continue
		}

		// Pod not found by UID - it's either deleted or replaced
		// Check if there's a pod with the same name but different UID
		k8sPod, existsByName := k8sPodByName[dbPod.Name]
		if existsByName && string(k8sPod.UID) != dbPod.UID {
			// Different pod with same name - the old pod was replaced
			log.Infof("Pod %s/%s (UID: %s) was replaced by new pod (UID: %s)",
				dbPod.Namespace, dbPod.Name, dbPod.UID, k8sPod.UID)
		} else {
			// Pod completely gone
			log.Infof("Pod %s/%s (UID: %s) no longer exists in K8s",
				dbPod.Namespace, dbPod.Name, dbPod.UID)
		}

		// Clean up the stale pod
		if err := j.cleanupStalePod(ctx, dbPod); err != nil {
			log.Errorf("Failed to cleanup stale pod %s/%s: %v", dbPod.Namespace, dbPod.Name, err)
			atomic.AddInt64(&stats.ErrorCount, 1)
			continue
		}

		cleanedCount++
	}

	atomic.AddInt64(&stats.ItemsUpdated, cleanedCount)

	if cleanedCount > 0 {
		span.SetAttributes(attribute.Int64("cleaned_count", cleanedCount))
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

// cleanupStalePod marks a stale pod as no longer running and ends its running period
func (j *StalePodCleanupJob) cleanupStalePod(ctx context.Context, dbPod *dbModel.GpuPods) error {
	span, ctx := trace.StartSpanFromContext(ctx, "cleanupStalePod")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("pod.namespace", dbPod.Namespace),
		attribute.String("pod.name", dbPod.Name),
		attribute.String("pod.uid", dbPod.UID),
		attribute.String("pod.phase_before", dbPod.Phase),
	)

	// Use updated_at as the end time (last known state)
	endTime := dbPod.UpdatedAt

	// 1. Update gpu_pods table
	dbPod.Phase = string(corev1.PodSucceeded)
	dbPod.Running = false
	dbPod.Deleted = true
	dbPod.UpdatedAt = time.Now()

	if err := database.GetFacade().GetPod().UpdateGpuPods(ctx, dbPod); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to update gpu_pods")
		return err
	}

	// 2. End running period if exists
	if err := database.GetFacade().GetPodRunningPeriods().EndRunningPeriod(ctx, dbPod.UID, endTime); err != nil {
		// Log but don't fail - running period might not exist
		log.Warnf("Failed to end running period for pod %s: %v", dbPod.UID, err)
		span.SetAttributes(attribute.String("running_period_error", err.Error()))
	} else {
		span.SetAttributes(attribute.Bool("running_period_ended", true))
	}

	span.SetAttributes(
		attribute.String("pod.phase_after", dbPod.Phase),
		attribute.Bool("pod.cleaned", true),
		attribute.String("end_time", endTime.Format(time.RFC3339)),
	)
	span.SetStatus(codes.Ok, "")

	log.Infof("Cleaned up stale pod %s/%s (UID: %s), end_time: %s",
		dbPod.Namespace, dbPod.Name, dbPod.UID, endTime.Format(time.RFC3339))

	return nil
}
