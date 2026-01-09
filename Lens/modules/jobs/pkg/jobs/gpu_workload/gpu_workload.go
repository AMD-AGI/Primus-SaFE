// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package gpu_workload

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/k8sUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GpuWorkloadJob struct {
}

func (g *GpuWorkloadJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "gpu_workload_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	span.SetAttributes(
		attribute.String("job.name", "gpu_workload"),
		attribute.String("cluster.name", clusterName),
	)

	// Get unfinished workloads
	getWorkloadsSpan, getWorkloadsCtx := trace.StartSpanFromContext(ctx, "getWorkloadNotEnd")
	getWorkloadsSpan.SetAttributes(attribute.String("cluster.name", clusterName))

	startTime := time.Now()
	workloadsNotEnd, err := database.GetFacadeForCluster(clusterName).GetWorkload().GetWorkloadNotEnd(getWorkloadsCtx)
	duration := time.Since(startTime)

	if err != nil {
		getWorkloadsSpan.RecordError(err)
		getWorkloadsSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		getWorkloadsSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getWorkloadsSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get workloads")
		return stats, err
	}

	getWorkloadsSpan.SetAttributes(
		attribute.Int("workloads_count", len(workloadsNotEnd)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	getWorkloadsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getWorkloadsSpan)

	// Concurrently check each workload
	processWorkloadsSpan, processCtx := trace.StartSpanFromContext(ctx, "processWorkloads")
	processWorkloadsSpan.SetAttributes(attribute.Int("workloads_count", len(workloadsNotEnd)))

	startTime = time.Now()
	wg := &sync.WaitGroup{}
	for i := range workloadsNotEnd {
		workload := workloadsNotEnd[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := g.checkWorkload(processCtx, clusterName, workload, clientSets)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Failed to check workload %s: %v", workload.Name, err)
			} else {
				atomic.AddInt64(&stats.ItemsUpdated, 1)
			}
		}()
	}
	wg.Wait()
	duration = time.Since(startTime)

	processWorkloadsSpan.SetAttributes(
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Int64("items_updated", stats.ItemsUpdated),
	)
	processWorkloadsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(processWorkloadsSpan)

	stats.RecordsProcessed = int64(len(workloadsNotEnd))
	stats.AddCustomMetric("workloads_count", len(workloadsNotEnd))
	stats.AddMessage("GPU workload status updated successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Int("workloads_count", len(workloadsNotEnd)),
		attribute.Int64("records_processed", stats.RecordsProcessed),
		attribute.Int64("items_updated", stats.ItemsUpdated),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)
	span.SetStatus(codes.Ok, "")
	return stats, nil
}

func (g *GpuWorkloadJob) checkWorkload(ctx context.Context, clusterName string, dbWorkload *dbModel.GpuWorkload, clientSets *clientsets.K8SClientSet) error {
	span, ctx := trace.StartSpanFromContext(ctx, "checkWorkload")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("workload.name", dbWorkload.Name),
		attribute.String("workload.namespace", dbWorkload.Namespace),
		attribute.String("workload.kind", dbWorkload.Kind),
		attribute.String("workload.status_before", string(dbWorkload.Status)),
	)

	// Check weather already end - get workload from K8s
	getWorkloadSpan, getWorkloadCtx := trace.StartSpanFromContext(ctx, "getWorkloadFromK8s")
	getWorkloadSpan.SetAttributes(
		attribute.String("workload.group_version", dbWorkload.GroupVersion),
		attribute.String("workload.kind", dbWorkload.Kind),
		attribute.String("workload.namespace", dbWorkload.Namespace),
		attribute.String("workload.name", dbWorkload.Name),
	)

	startTime := time.Now()
	_, err := k8sUtil.GetObjectByGvk(getWorkloadCtx, dbWorkload.GroupVersion, dbWorkload.Kind, dbWorkload.Namespace, dbWorkload.Name, clientSets.ControllerRuntimeClient)
	duration := time.Since(startTime)

	getWorkloadSpan.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))

	if err != nil {
		// Check if this is a "no kind match" error (CRD not installed or unknown Kind)
		// This can happen when other plugins insert workloads with custom types
		if isNoKindMatchError(err) {
			getWorkloadSpan.SetAttributes(
				attribute.Bool("no_kind_match", true),
				attribute.String("error.message", err.Error()),
			)
			getWorkloadSpan.SetStatus(codes.Ok, "No kind match, skipping")
			trace.FinishSpan(getWorkloadSpan)

			span.SetAttributes(attribute.Bool("no_kind_match", true))
			span.SetStatus(codes.Ok, "Skipped due to no kind match")
			return nil
		}

		if client.IgnoreNotFound(err) != nil {
			getWorkloadSpan.RecordError(err)
			getWorkloadSpan.SetAttributes(attribute.String("error.message", err.Error()))
			getWorkloadSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(getWorkloadSpan)

			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to get workload from K8s")
			return err
		}

		// Workload not found, mark as deleted
		getWorkloadSpan.SetAttributes(attribute.Bool("workload.not_found", true))
		getWorkloadSpan.SetStatus(codes.Ok, "Workload not found, marking as deleted")
		trace.FinishSpan(getWorkloadSpan)

		dbWorkload.Status = metadata.WorkloadStatusDeleted
		dbWorkload.EndAt = dbWorkload.UpdatedAt

		span.SetAttributes(
			attribute.Bool("workload.not_found", true),
			attribute.String("workload.status_after", string(metadata.WorkloadStatusDeleted)),
		)
	} else {
		getWorkloadSpan.SetAttributes(attribute.Bool("workload.found", true))
		getWorkloadSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(getWorkloadSpan)
	}

	if dbWorkload.Status != metadata.WorkloadStatusDeleted {
		podCount, err := g.fillCurrentGpuCount(ctx, clusterName, dbWorkload, clientSets)
		if err != nil {
			span.RecordError(err)
			span.SetAttributes(attribute.String("error.message", err.Error()))
			span.SetStatus(codes.Error, "Failed to fill GPU count")
			return err
		}

		if podCount != 0 {
			dbWorkload.Status = metadata.WorkloadStatusRunning
		} else {
			dbWorkload.Status = metadata.WorkloadStatusDone
		}

		span.SetAttributes(
			attribute.Int("workload.pod_count", podCount),
			attribute.Int("workload.gpu_request", int(dbWorkload.GpuRequest)),
			attribute.String("workload.status_after", string(dbWorkload.Status)),
		)
	}

	// Update workload in database
	updateSpan, updateCtx := trace.StartSpanFromContext(ctx, "updateWorkloadInDatabase")
	updateSpan.SetAttributes(
		attribute.String("workload.name", dbWorkload.Name),
		attribute.String("workload.namespace", dbWorkload.Namespace),
		attribute.String("workload.status", string(dbWorkload.Status)),
		attribute.Int("workload.gpu_request", int(dbWorkload.GpuRequest)),
	)

	startTime = time.Now()
	err = database.GetFacadeForCluster(clusterName).GetWorkload().UpdateGpuWorkload(updateCtx, dbWorkload)
	duration = time.Since(startTime)

	if err != nil {
		updateSpan.RecordError(err)
		updateSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		updateSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(updateSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to update workload in database")
		return err
	}

	updateSpan.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))
	updateSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(updateSpan)

	span.SetStatus(codes.Ok, "")
	return nil
}

// isNoKindMatchError checks if the error is a "no kind match" error
// This happens when the Kind (CRD) is not installed or doesn't exist in the cluster
func isNoKindMatchError(err error) bool {
	if err == nil {
		return false
	}

	// Check error message for common patterns indicating Kind not found
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "with unknown kind") ||
		strings.Contains(errMsg, "no matches for kind")
}

func (g *GpuWorkloadJob) fillCurrentGpuCount(ctx context.Context, clusterName string, dbWorkload *dbModel.GpuWorkload, clientSets *clientsets.K8SClientSet) (podCount int, err error) {
	span, ctx := trace.StartSpanFromContext(ctx, "fillCurrentGpuCount")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("workload.name", dbWorkload.Name),
		attribute.String("workload.namespace", dbWorkload.Namespace),
		attribute.String("workload.uid", dbWorkload.UID),
	)

	// Check all exist pods - get from database
	getPodsSpan, getPodsCtx := trace.StartSpanFromContext(ctx, "getActivePodsByWorkloadUid")
	getPodsSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("workload.uid", dbWorkload.UID),
	)

	startTime := time.Now()
	dBpods, err := workload.GetActivePodsByWorkloadUid(getPodsCtx, clusterName, dbWorkload.UID)
	duration := time.Since(startTime)

	if err != nil {
		getPodsSpan.RecordError(err)
		getPodsSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		getPodsSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getPodsSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get active pods")
		return 0, err
	}

	getPodsSpan.SetAttributes(
		attribute.Int("db_pods_count", len(dBpods)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	getPodsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getPodsSpan)

	if len(dBpods) == 0 {
		span.SetAttributes(
			attribute.Int("pod_count", 0),
			attribute.Int("gpu_allocated", 0),
		)
		span.SetStatus(codes.Ok, "")
		return 0, nil
	}

	// Verify pods exist in K8s
	verifyPodsSpan, verifyCtx := trace.StartSpanFromContext(ctx, "verifyPodsInK8s")
	verifyPodsSpan.SetAttributes(attribute.Int("db_pods_count", len(dBpods)))

	startTime = time.Now()
	pods := []corev1.Pod{}
	notFoundCount := 0
	for i := range dBpods {
		dbPod := dBpods[i]
		pod := &corev1.Pod{}
		err = clientSets.ControllerRuntimeClient.Get(verifyCtx, types.NamespacedName{Namespace: dbPod.Namespace, Name: dbPod.Name}, pod)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				verifyPodsSpan.RecordError(err)
				verifyPodsSpan.SetAttributes(
					attribute.String("error.message", err.Error()),
					attribute.String("pod.namespace", dbPod.Namespace),
					attribute.String("pod.name", dbPod.Name),
				)
				verifyPodsSpan.SetStatus(codes.Error, err.Error())
				trace.FinishSpan(verifyPodsSpan)

				span.RecordError(err)
				span.SetStatus(codes.Error, "Failed to verify pod in K8s")
				return 0, err
			}
			notFoundCount++
			continue
		}
		pods = append(pods, *pod)
	}
	duration = time.Since(startTime)

	verifyPodsSpan.SetAttributes(
		attribute.Int("k8s_pods_found", len(pods)),
		attribute.Int("pods_not_found", notFoundCount),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	verifyPodsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(verifyPodsSpan)

	// Calculate GPU allocation
	calculateSpan, calculateCtx := trace.StartSpanFromContext(ctx, "calculateGpuAllocation")
	calculateSpan.SetAttributes(
		attribute.Int("pods_count", len(pods)),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	startTime = time.Now()
	podCount = len(pods)
	allocated := gpu.GetAllocatedGpuResourceFromPods(calculateCtx, pods, metadata.GetResourceName(metadata.GpuVendorAMD))
	dbWorkload.GpuRequest = int32(allocated)
	duration = time.Since(startTime)

	calculateSpan.SetAttributes(
		attribute.Int("gpu_allocated", allocated),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	calculateSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(calculateSpan)

	span.SetAttributes(
		attribute.Int("pod_count", podCount),
		attribute.Int("gpu_allocated", allocated),
		attribute.Int("pods_not_found", notFoundCount),
	)
	span.SetStatus(codes.Ok, "")
	return podCount, nil
}

func (g *GpuWorkloadJob) Schedule() string {
	return "@every 20s"
}
