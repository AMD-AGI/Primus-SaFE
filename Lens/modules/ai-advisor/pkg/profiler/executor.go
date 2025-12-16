package profiler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
)

// ProfilerCollectionExecutor Profiler file collection task executor
type ProfilerCollectionExecutor struct {
	coreTask.BaseExecutor

	collector   *Collector
	metadataMgr *MetadataManager
}

// NewProfilerCollectionExecutor creates profiler collection executor
func NewProfilerCollectionExecutor(
	collector *Collector,
	metadataMgr *MetadataManager,
) *ProfilerCollectionExecutor {
	return &ProfilerCollectionExecutor{
		collector:   collector,
		metadataMgr: metadataMgr,
	}
}

// GetTaskType returns task type
func (e *ProfilerCollectionExecutor) GetTaskType() string {
	return constant.TaskTypeProfilerCollection
}

// Validate validates task parameters
func (e *ProfilerCollectionExecutor) Validate(taskState *model.WorkloadTaskState) error {
	// Check required parameters
	if taskState.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}

	return nil
}

// Execute executes profiler collection task
func (e *ProfilerCollectionExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	taskState := execCtx.Task
	workloadUID := taskState.WorkloadUID

	log.Infof("Starting profiler collection for workload: %s", workloadUID)

	// 1. Get task configuration
	config := e.extractTaskConfig(taskState)

	// 2. Select target pod (reuse logic from metadata_collection_executor)
	gpuPod, err := e.selectTargetPod(ctx, workloadUID)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to select target pod: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), nil
	}

	if gpuPod == nil {
		log.Debugf("No pod found for workload %s yet, will retry later", workloadUID)
		// No pod yet, keep task pending for retry
		return &coreTask.ExecutionResult{
			Success:   true,
			UpdateExt: map[string]interface{}{
				"last_check": time.Now().Format(time.RFC3339),
				"status":     "waiting_for_pod",
			},
			NewStatus: constant.TaskStatusPending,
		}, nil
	}

	log.Infof("Selected pod %s/%s for profiler collection", gpuPod.Namespace, gpuPod.Name)

	// 3. Execute collection
	// TODO: Implement CollectProfilerFiles in Collector
	_ = config // Suppress unused variable warning
	// result, err := e.collector.CollectProfilerFiles(ctx, workloadUID, gpuPod.UID, config)
	// if err != nil {
	// 	log.Errorf("Failed to collect profiler files: %v", err)
	// 	return coreTask.FailureResult(err.Error(), map[string]interface{}{
	// 		"last_error":    err.Error(),
	// 		"last_executed": time.Now().Format(time.RFC3339),
	// 		"pod_name":      gpuPod.Name,
	// 		"pod_namespace": gpuPod.Namespace,
	// 	}), nil
	// }

	// 4. Update statistics (placeholder until CollectProfilerFiles is implemented)
	updates := map[string]interface{}{
		"last_executed": time.Now().Format(time.RFC3339),
		// "total_collected": result.TotalFiles,
		// "archived_count":  result.ArchivedFiles,
		// "last_result":     result,
		"pod_name":      gpuPod.Name,
		"pod_namespace": gpuPod.Namespace,
		"node_name":     gpuPod.NodeName,
	}

	// Increment execution count
	executionCount := e.GetExtInt(taskState, "execution_count")
	executionCount++
	updates["execution_count"] = executionCount

	// 5. Check if should continue execution
	if e.shouldContinue(ctx, taskState, executionCount) {
		log.Debugf("Profiler collection will continue for workload %s (execution %d)",
			workloadUID, executionCount)
		// Keep task pending for next execution
		return &coreTask.ExecutionResult{
			Success:   true,
			UpdateExt: updates,
			NewStatus: constant.TaskStatusPending,
		}, nil
	}

	// 6. Task completed
	log.Infof("Profiler collection completed for workload: %s", workloadUID)
	// log.Infof("Profiler collection completed for workload: %s, archived: %d/%d files",
	// 	workloadUID, result.ArchivedFiles, result.TotalFiles)

	return coreTask.SuccessResult(updates), nil
}

// Cancel cancels task
func (e *ProfilerCollectionExecutor) Cancel(
	ctx context.Context,
	taskState *model.WorkloadTaskState,
) error {
	log.Infof("Profiler collection task cancelled for workload %s", taskState.WorkloadUID)
	return nil
}

// extractTaskConfig extracts task configuration
func (e *ProfilerCollectionExecutor) extractTaskConfig(taskState *model.WorkloadTaskState) *CollectorConfig {
	return &CollectorConfig{
		AutoCollect: e.GetExtBool(taskState, "auto_collect"),
		// TODO: Add these fields to CollectorConfig if needed
		// MinConfidence: e.GetExtString(taskState, "min_confidence"),
		// MaxFileSize:   int64(e.GetExtInt(taskState, "max_file_size")),
	}
}

// selectTargetPod selects target pod for profiler collection
// Note: This is the same logic as in metadata_collection_executor
// Consider extracting to pkg/task/common/pod_selector.go for reuse
func (e *ProfilerCollectionExecutor) selectTargetPod(
	ctx context.Context,
	workloadUID string,
) (*model.GpuPods, error) {
	// Method 1: Find pod through workload_pod_reference table
	workloadFacade := database.GetFacade().GetWorkload()
	podRefs, err := workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to query workload_pod_reference for workload %s: %v", workloadUID, err)
	}

	var pods []*model.GpuPods
	if len(podRefs) > 0 {
		podUIDs := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			podUIDs = append(podUIDs, ref.PodUID)
		}

		db := database.GetFacade().GetSystemConfig().GetDB()
		err = db.WithContext(ctx).
			Where("uid IN ? AND deleted = ?", podUIDs, false).
			Find(&pods).Error
		if err != nil {
			return nil, fmt.Errorf("failed to query pods by references: %w", err)
		}
	}

	// Method 2: Find pods of child workload (recursive search)
	if len(pods) == 0 {
		childWorkloads, err := workloadFacade.ListChildrenWorkloadByParentUid(ctx, workloadUID)
		if err != nil {
			log.Warnf("Failed to query child workloads for %s: %v", workloadUID, err)
		} else if len(childWorkloads) > 0 {
			for _, child := range childWorkloads {
				childPod, err := e.selectTargetPod(ctx, child.UID)
				if err == nil && childPod != nil {
					return childPod, nil
				}
			}
		}
	}

	if len(pods) == 0 {
		return nil, nil // No pod found yet (not an error, may appear later)
	}

	// Prioritize pods ending with master-0
	for _, pod := range pods {
		if strings.HasSuffix(pod.Name, "master-0") {
			log.Infof("Selected master-0 pod: %s/%s for workload %s",
				pod.Namespace, pod.Name, workloadUID)
			return pod, nil
		}
	}

	// Return first pod if no master-0
	selectedPod := pods[0]
	log.Infof("Selected first pod: %s/%s for workload %s",
		selectedPod.Namespace, selectedPod.Name, workloadUID)
	return selectedPod, nil
}

// shouldContinue checks if task should continue execution
func (e *ProfilerCollectionExecutor) shouldContinue(
	ctx context.Context,
	taskState *model.WorkloadTaskState,
	currentExecutionCount int,
) bool {
	// 1. Check auto_restart configuration
	autoRestart := e.GetExtBool(taskState, "auto_restart")
	if !autoRestart {
		log.Debugf("auto_restart=false, stopping profiler collection")
		return false
	}

	// 2. Check if execution count exceeds limit
	maxExecutions := e.GetExtInt(taskState, "max_executions")
	if maxExecutions > 0 && currentExecutionCount >= maxExecutions {
		log.Infof("Reached max executions (%d), stopping profiler collection", maxExecutions)
		return false
	}

	// 3. Check if training workload is still running
	// TODO: Implement workload status check
	// For now, continue if auto_restart is true and max_executions not reached
	workloadUID := taskState.WorkloadUID
	_ = workloadUID // Suppress unused variable warning
	
	// workloadFacade := database.GetFacade().GetWorkload()
	// workload, err := workloadFacade.GetWorkloadByUID(ctx, workloadUID)
	// if err != nil {
	// 	log.Warnf("Failed to query workload status: %v", err)
	// 	return true // If query fails, continue by default
	// }
	//
	// if workload == nil {
	// 	log.Warnf("Workload %s not found, stopping collection", workloadUID)
	// 	return false
	// }
	//
	// // Check if workload is still active
	// status := strings.ToLower(workload.Status)
	// if status == "succeeded" || status == "failed" || status == "completed" {
	// 	log.Infof("Workload %s is %s, stopping profiler collection", workloadUID, status)
	// 	return false
	// }

	log.Debugf("Workload %s continuing profiler collection (workload status check not yet implemented)", workloadUID)
	return true
}

