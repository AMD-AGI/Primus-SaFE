package gpu_pod

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
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GpuPodJob struct {
}

func (g *GpuPodJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "gpu_pod_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	span.SetAttributes(
		attribute.String("job.name", "gpu_pod"),
		attribute.String("cluster.name", clientsets.GetClusterManager().GetCurrentClusterName()),
	)

	// Query active GPU pods
	listSpan, listCtx := trace.StartSpanFromContext(ctx, "listActiveGpuPods")
	startTime := time.Now()
	activePods, err := database.GetFacade().GetPod().ListActiveGpuPods(listCtx)
	duration := time.Since(startTime)

	if err != nil {
		listSpan.RecordError(err)
		listSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		listSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(listSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to list active GPU pods")
		log.Errorf("list active gpu pods: %v", err)
		return stats, err
	}

	listSpan.SetAttributes(
		attribute.Int("active_pods_count", len(activePods)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	listSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(listSpan)

	// Concurrently check each pod
	processSpan, processCtx := trace.StartSpanFromContext(ctx, "processActivePods")
	processSpan.SetAttributes(attribute.Int("pods_count", len(activePods)))

	startTime = time.Now()
	wg := &sync.WaitGroup{}
	for i := range activePods {
		dbPod := activePods[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := g.checkForSinglePod(processCtx, dbPod, clientSets, stats)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Failed to check pod %s/%s: %v", dbPod.Namespace, dbPod.Name, err)
			}
		}()
	}
	wg.Wait()
	duration = time.Since(startTime)

	processSpan.SetAttributes(
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Int64("updated_count", stats.ItemsUpdated),
	)
	processSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(processSpan)

	stats.RecordsProcessed = int64(len(activePods))
	stats.AddCustomMetric("active_pods_count", len(activePods))
	stats.AddMessage("GPU pod status updated successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Int("active_pods_count", len(activePods)),
		attribute.Int64("records_processed", stats.RecordsProcessed),
		attribute.Int64("items_updated", stats.ItemsUpdated),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)
	span.SetStatus(codes.Ok, "")
	return stats, nil
}

func (g *GpuPodJob) Schedule() string {
	return "@every 5s"
}

func (g *GpuPodJob) checkForSinglePod(ctx context.Context, dbPod *dbModel.GpuPods, clientSets *clientsets.K8SClientSet, stats *common.ExecutionStats) error {
	span, ctx := trace.StartSpanFromContext(ctx, "checkForSinglePod")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("pod.namespace", dbPod.Namespace),
		attribute.String("pod.name", dbPod.Name),
		attribute.String("pod.phase_before", dbPod.Phase),
	)

	// Get pod information from K8s
	getPodSpan, getPodCtx := trace.StartSpanFromContext(ctx, "getPodFromK8s")
	getPodSpan.SetAttributes(
		attribute.String("pod.namespace", dbPod.Namespace),
		attribute.String("pod.name", dbPod.Name),
	)

	pod := &corev1.Pod{}
	changed := false
	startTime := time.Now()
	err := clientSets.ControllerRuntimeClient.Get(getPodCtx, types.NamespacedName{
		Namespace: dbPod.Namespace,
		Name:      dbPod.Name,
	}, pod)
	duration := time.Since(startTime)

	getPodSpan.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))

	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			getPodSpan.RecordError(err)
			getPodSpan.SetAttributes(attribute.String("error.message", err.Error()))
			getPodSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(getPodSpan)

			span.RecordError(err)
			span.SetAttributes(attribute.String("error.message", err.Error()))
			span.SetStatus(codes.Error, "Failed to get pod from K8s")
			log.Errorf("Failed to get pod %s/%s: %v", dbPod.Namespace, dbPod.Name, err)
			return err
		}
		// Pod not found, mark it as deleted
		getPodSpan.SetAttributes(
			attribute.Bool("pod.not_found", true),
			attribute.Bool("pod.marking_deleted", true),
		)
		getPodSpan.SetStatus(codes.Ok, "Pod not found, marking as deleted")
		trace.FinishSpan(getPodSpan)

		dbPod.Phase = string(corev1.PodSucceeded)
		dbPod.Running = false
		dbPod.Deleted = true
		changed = true

		span.SetAttributes(
			attribute.Bool("pod.not_found", true),
			attribute.Bool("pod.marked_deleted", true),
			attribute.String("pod.phase_after", dbPod.Phase),
		)
	} else {
		getPodSpan.SetAttributes(
			attribute.String("pod.phase", string(pod.Status.Phase)),
			attribute.Bool("pod.found", true),
		)
		getPodSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(getPodSpan)

		if dbPod.Phase != string(pod.Status.Phase) {
			log.Infof("Pod %s/%s phase changed from %s to %s", dbPod.Namespace, dbPod.Name, dbPod.Phase, pod.Status.Phase)
			dbPod.Phase = string(pod.Status.Phase)
			changed = true

			span.SetAttributes(
				attribute.Bool("pod.phase_changed", true),
				attribute.String("pod.phase_after", dbPod.Phase),
			)
		}
	}

	if changed {
		// Update database
		updateSpan, updateCtx := trace.StartSpanFromContext(ctx, "updatePodInDatabase")
		updateSpan.SetAttributes(
			attribute.String("pod.namespace", dbPod.Namespace),
			attribute.String("pod.name", dbPod.Name),
			attribute.String("pod.phase", dbPod.Phase),
			attribute.Bool("pod.deleted", dbPod.Deleted),
		)

		startTime = time.Now()
		err = database.GetFacade().GetPod().UpdateGpuPods(updateCtx, dbPod)
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
			span.SetAttributes(attribute.String("error.message", err.Error()))
			span.SetStatus(codes.Error, "Failed to update pod in database")
			log.Errorf("Failed to update pod %s/%s: %v", dbPod.Namespace, dbPod.Name, err)
			return err
		}

		updateSpan.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))
		updateSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(updateSpan)

		atomic.AddInt64(&stats.ItemsUpdated, 1)

		span.SetAttributes(attribute.Bool("pod.updated", true))
	} else {
		span.SetAttributes(attribute.Bool("pod.updated", false))
	}

	span.SetStatus(codes.Ok, "")
	return nil
}
