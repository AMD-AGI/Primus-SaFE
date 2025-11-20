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
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	corev1 "k8s.io/api/core/v1"
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

	// Group pods by namespace
	groupSpan, _ := trace.StartSpanFromContext(ctx, "groupPodsByNamespace")
	startTime = time.Now()
	podsByNamespace := make(map[string][]*dbModel.GpuPods)
	for i := range activePods {
		namespace := activePods[i].Namespace
		podsByNamespace[namespace] = append(podsByNamespace[namespace], activePods[i])
	}
	duration = time.Since(startTime)

	groupSpan.SetAttributes(
		attribute.Int("total_pods", len(activePods)),
		attribute.Int("namespace_count", len(podsByNamespace)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	groupSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(groupSpan)

	// Process each namespace concurrently
	processSpan, processCtx := trace.StartSpanFromContext(ctx, "processNamespaces")
	processSpan.SetAttributes(
		attribute.Int("pods_count", len(activePods)),
		attribute.Int("namespace_count", len(podsByNamespace)),
	)

	startTime = time.Now()
	wg := &sync.WaitGroup{}
	for namespace, pods := range podsByNamespace {
		wg.Add(1)
		go func(ns string, nsPods []*dbModel.GpuPods) {
			defer wg.Done()
			err := g.checkPodsInNamespace(processCtx, ns, nsPods, clientSets, stats)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Failed to check pods in namespace %s: %v", ns, err)
			}
		}(namespace, pods)
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

func (g *GpuPodJob) checkPodsInNamespace(ctx context.Context, namespace string, dbPods []*dbModel.GpuPods, clientSets *clientsets.K8SClientSet, stats *common.ExecutionStats) error {
	span, ctx := trace.StartSpanFromContext(ctx, "checkPodsInNamespace")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("namespace", namespace),
		attribute.Int("db_pods_count", len(dbPods)),
	)

	// List all pods in this namespace from K8s
	listSpan, listCtx := trace.StartSpanFromContext(ctx, "listPodsInNamespace")
	listSpan.SetAttributes(attribute.String("namespace", namespace))

	podList := &corev1.PodList{}
	startTime := time.Now()
	err := clientSets.ControllerRuntimeClient.List(listCtx, podList, client.InNamespace(namespace))
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
		span.SetStatus(codes.Error, "Failed to list pods in namespace")
		log.Errorf("Failed to list pods in namespace %s: %v", namespace, err)
		return err
	}

	listSpan.SetAttributes(
		attribute.Int("k8s_pods_count", len(podList.Items)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	listSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(listSpan)

	// Build a map of K8s pods by name for quick lookup
	k8sPodMap := make(map[string]*corev1.Pod)
	for i := range podList.Items {
		pod := &podList.Items[i]
		k8sPodMap[pod.Name] = pod
	}

	// Check each DB pod against K8s pods
	compareSpan, compareCtx := trace.StartSpanFromContext(ctx, "comparePodsWithDB")
	compareSpan.SetAttributes(
		attribute.String("namespace", namespace),
		attribute.Int("db_pods_count", len(dbPods)),
		attribute.Int("k8s_pods_count", len(k8sPodMap)),
	)

	startTime = time.Now()
	for _, dbPod := range dbPods {
		err := g.checkSinglePodWithK8sPod(compareCtx, dbPod, k8sPodMap[dbPod.Name], stats)
		if err != nil {
			log.Errorf("Failed to check pod %s/%s: %v", dbPod.Namespace, dbPod.Name, err)
		}
	}
	duration = time.Since(startTime)

	compareSpan.SetAttributes(
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	compareSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(compareSpan)

	span.SetStatus(codes.Ok, "")
	return nil
}

func (g *GpuPodJob) checkSinglePodWithK8sPod(ctx context.Context, dbPod *dbModel.GpuPods, k8sPod *corev1.Pod, stats *common.ExecutionStats) error {
	span, ctx := trace.StartSpanFromContext(ctx, "checkSinglePodWithK8sPod")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("pod.namespace", dbPod.Namespace),
		attribute.String("pod.name", dbPod.Name),
		attribute.String("pod.phase_before", dbPod.Phase),
	)

	changed := false

	if k8sPod == nil {
		// Pod not found in K8s, mark it as deleted
		span.SetAttributes(
			attribute.Bool("pod.not_found", true),
			attribute.Bool("pod.marking_deleted", true),
		)

		dbPod.Phase = string(corev1.PodSucceeded)
		dbPod.Running = false
		dbPod.Deleted = true
		changed = true

		span.SetAttributes(
			attribute.Bool("pod.marked_deleted", true),
			attribute.String("pod.phase_after", dbPod.Phase),
		)
	} else {
		// Pod found in K8s
		span.SetAttributes(
			attribute.String("pod.phase", string(k8sPod.Status.Phase)),
			attribute.Bool("pod.found", true),
			attribute.String("pod.uid", string(k8sPod.UID)),
			attribute.String("db_pod.uid", dbPod.UID),
		)

		// Check if UID matches - if not, this is a different pod with the same name
		if dbPod.UID != string(k8sPod.UID) {
			log.Infof("Pod %s/%s UID mismatch (DB: %s, K8s: %s), marking old pod as deleted",
				dbPod.Namespace, dbPod.Name, dbPod.UID, k8sPod.UID)
			dbPod.Phase = string(corev1.PodSucceeded)
			dbPod.Running = false
			dbPod.Deleted = true
			changed = true

			span.SetAttributes(
				attribute.Bool("pod.uid_mismatch", true),
				attribute.Bool("pod.marked_deleted", true),
				attribute.String("pod.phase_after", dbPod.Phase),
			)
		} else if dbPod.Phase != string(k8sPod.Status.Phase) {
			log.Infof("Pod %s/%s phase changed from %s to %s", dbPod.Namespace, dbPod.Name, dbPod.Phase, k8sPod.Status.Phase)
			dbPod.Phase = string(k8sPod.Status.Phase)
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

		startTime := time.Now()
		err := database.GetFacade().GetPod().UpdateGpuPods(updateCtx, dbPod)
		duration := time.Since(startTime)

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
