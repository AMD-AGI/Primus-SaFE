// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package profiler

import (
	"context"
	"fmt"
	"strings"
	"time"

	metadataCollector "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
)

// ProfilerCollectionExecutor Profiler file collection task executor
type ProfilerCollectionExecutor struct {
	coreTask.BaseExecutor

	collector           *Collector
	metadataMgr         *MetadataManager
	metadataFacade      database.AiWorkloadMetadataFacadeInterface
	workloadFacade      database.WorkloadFacadeInterface
	configService       *FrameworkConfigService
	metadataCollector   *metadataCollector.Collector // For getting node-exporter client
}

// NewProfilerCollectionExecutor creates profiler collection executor
func NewProfilerCollectionExecutor(
	collector *Collector,
	metadataMgr *MetadataManager,
	metaCollector *metadataCollector.Collector,
) *ProfilerCollectionExecutor {
	return &ProfilerCollectionExecutor{
		collector:         collector,
		metadataMgr:       metadataMgr,
		metadataFacade:    database.NewAiWorkloadMetadataFacade(),
		workloadFacade:    database.NewWorkloadFacade(),
		configService:     NewFrameworkConfigService(),
		metadataCollector: metaCollector,
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
		// Before retrying, check if the workload is already terminated.
		// If so, there will never be a pod to find - complete the task.
		if !e.shouldContinue(ctx, taskState, 0) {
			log.Infof("No pod found and workload %s is terminated, completing profiler collection", workloadUID)
			return coreTask.SuccessResult(map[string]interface{}{
				"terminated_reason": "workload_terminated_no_pod",
				"completed_at":     time.Now().Format(time.RFC3339),
			}), nil
		}

		log.Debugf("No pod found for workload %s yet, will retry later", workloadUID)
		// No pod yet, keep task pending for retry
		return &coreTask.ExecutionResult{
			Success: true,
			UpdateExt: map[string]interface{}{
				"last_check": time.Now().Format(time.RFC3339),
				"status":     "waiting_for_pod",
			},
			NewStatus: constant.TaskStatusPending,
		}, nil
	}

	log.Infof("Selected pod %s/%s for profiler collection", gpuPod.Namespace, gpuPod.Name)

	// 3. Load framework config for precise directory scanning
	// Wait for metadata collection to save framework_config before proceeding
	frameworkConfig, err := e.loadFrameworkConfig(ctx, workloadUID)
	if err != nil {
		// Check wait count to avoid infinite waiting
		waitCount := e.GetExtInt(taskState, "config_wait_count")
		maxWaitCount := 10 // Wait up to ~50 seconds (5s interval * 10)

		if waitCount < maxWaitCount {
			// framework_config not ready yet, wait for metadata collection to complete
			log.Infof("Framework config not ready for workload %s (wait %d/%d), will retry later",
				workloadUID, waitCount+1, maxWaitCount)
			return &coreTask.ExecutionResult{
				Success: true,
				UpdateExt: map[string]interface{}{
					"last_check":        time.Now().Format(time.RFC3339),
					"status":            "waiting_for_framework_config",
					"config_wait_count": waitCount + 1,
					"wait_reason":       err.Error(),
				},
				NewStatus: constant.TaskStatusPending,
			}, nil
		}

		// Exceeded max wait count, proceed with fallback locations
		log.Warnf("Framework config wait timeout for workload %s after %d attempts, using fallback locations: %v",
			workloadUID, waitCount, err)
	}

	// 4. Get profiler locations to scan
	profilerLocations := e.configService.GetProfilerLocations(frameworkConfig)
	log.Infof("Profiler locations to scan for workload %s: %d locations", workloadUID, len(profilerLocations))
	for _, loc := range profilerLocations {
		log.Debugf("  - %s (patterns: %v, source: %s)", loc.Directory, loc.Patterns, loc.Source)
	}

	// 5. Get node-exporter client for the pod's node
	var nodeClient *client.Client
	if e.metadataCollector != nil {
		var nodeClientErr error
		nodeClient, nodeClientErr = e.metadataCollector.GetNodeExporterClientForPod(ctx, gpuPod.NodeName)
		if nodeClientErr != nil {
			log.Warnf("Failed to get node-exporter client for node %s: %v", gpuPod.NodeName, nodeClientErr)
		}
	}

	// 6. Execute collection using precise directories
	_ = config // Suppress unused variable warning for now
	updates := map[string]interface{}{
		"last_executed":     time.Now().Format(time.RFC3339),
		"pod_name":          gpuPod.Name,
		"pod_namespace":     gpuPod.Namespace,
		"node_name":         gpuPod.NodeName,
		"scan_locations":    len(profilerLocations),
		"config_based_scan": frameworkConfig != nil,
	}

	// Store profiler locations for reference
	if frameworkConfig != nil && frameworkConfig.ExtractedPaths != nil {
		updates["profiler_dir"] = frameworkConfig.ExtractedPaths.ProfilerDir
		updates["scan_source"] = "framework_config"
	} else {
		updates["scan_source"] = "fallback"
	}

	// Execute actual file collection
	if nodeClient != nil && e.collector != nil {
		// Build collection request with ProfilerLocation slice
		collectionReq := &LocationCollectionRequest{
			WorkloadUID:       workloadUID,
			PodUID:            gpuPod.UID,
			PodName:           gpuPod.Name,
			PodNamespace:      gpuPod.Namespace,
			Framework:         "",
			Locations:         profilerLocations,
			NodeClient:        nodeClient,
			EnableFileMatcher: true, // Enable file-workload timestamp matching for conflict detection
		}

		// Set framework if available
		if frameworkConfig != nil {
			collectionReq.Framework = frameworkConfig.Framework
		}

		// Get working directory from ai_workload_metadata.metadata.working_dir
		// (saved by metadata_collection_executor when collecting framework config)
		metadata, err := e.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
		if err == nil && metadata != nil && metadata.Metadata != nil {
			if workingDir, ok := metadata.Metadata["working_dir"].(string); ok && workingDir != "" {
				collectionReq.WorkingDir = workingDir
				log.Debugf("Using working directory from ai_workload_metadata: %s", workingDir)
			}
		}

		// Use the collector's method with the client from metadataCollector
		result, collectionErr := e.collector.CollectProfilerFilesFromLocations(ctx, collectionReq)
		if collectionErr != nil {
			log.Errorf("Failed to collect profiler files: %v", collectionErr)
			updates["collection_error"] = collectionErr.Error()
		} else {
			updates["total_files"] = result.TotalFiles
			updates["archived_count"] = result.ArchivedFiles
			updates["skipped_count"] = result.SkippedFiles
			updates["failed_count"] = result.FailedFiles
			if len(result.Errors) > 0 {
				updates["collection_errors"] = result.Errors
			}
			log.Infof("Profiler collection result: total=%d, archived=%d, skipped=%d, failed=%d",
				result.TotalFiles, result.ArchivedFiles, result.SkippedFiles, result.FailedFiles)
		}
	} else {
		log.Warnf("Node client or collector not available, skipping file collection")
		updates["collection_skipped"] = "node_client_or_collector_unavailable"
	}

	// Increment execution count
	executionCount := e.GetExtInt(taskState, "execution_count")
	executionCount++
	updates["execution_count"] = executionCount

	// 6. Check if should continue execution
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

	// 7. Task completed
	log.Infof("Profiler collection completed for workload: %s", workloadUID)

	return coreTask.SuccessResult(updates), nil
}

// loadFrameworkConfig loads framework config from ai_workload_metadata
func (e *ProfilerCollectionExecutor) loadFrameworkConfig(
	ctx context.Context,
	workloadUID string,
) (*FrameworkConfig, error) {
	// Get metadata from database
	metadata, err := e.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	if metadata == nil {
		return nil, fmt.Errorf("metadata not found for workload %s", workloadUID)
	}

	// Extract framework_config from metadata
	frameworkConfigData, ok := metadata.Metadata["framework_config"]
	if !ok {
		return nil, fmt.Errorf("framework_config not found in metadata")
	}

	// Convert to FrameworkConfig
	configMap, ok := frameworkConfigData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid framework_config format")
	}

	config := &FrameworkConfig{
		CollectedAt: time.Now(),
	}

	// Parse framework
	if framework, ok := configMap["framework"].(string); ok {
		config.Framework = framework
	}

	// Parse version
	if version, ok := configMap["version"].(string); ok {
		config.Version = version
	}

	// Parse source
	if sourceData, ok := configMap["source"].(map[string]interface{}); ok {
		config.Source = &ConfigSource{}
		if sourceType, ok := sourceData["type"].(string); ok {
			config.Source.Type = sourceType
		}
		if sourcePath, ok := sourceData["path"].(string); ok {
			config.Source.Path = sourcePath
		}
	}

	// Parse extracted_paths
	if pathsData, ok := configMap["extracted_paths"].(map[string]interface{}); ok {
		config.ExtractedPaths = &ExtractedPaths{
			CustomPaths: make(map[string]string),
		}
		if profilerDir, ok := pathsData["profiler_dir"].(string); ok {
			config.ExtractedPaths.ProfilerDir = profilerDir
		}
		if tensorboardDir, ok := pathsData["tensorboard_dir"].(string); ok {
			config.ExtractedPaths.TensorBoardDir = tensorboardDir
		}
		if checkpointDir, ok := pathsData["checkpoint_dir"].(string); ok {
			config.ExtractedPaths.CheckpointDir = checkpointDir
		}
		if logDir, ok := pathsData["log_dir"].(string); ok {
			config.ExtractedPaths.LogDir = logDir
		}
		if workspaceDir, ok := pathsData["workspace_dir"].(string); ok {
			config.ExtractedPaths.WorkspaceDir = workspaceDir
		}
		if customPaths, ok := pathsData["custom_paths"].(map[string]interface{}); ok {
			for k, v := range customPaths {
				if strVal, ok := v.(string); ok {
					config.ExtractedPaths.CustomPaths[k] = strVal
				}
			}
		}
	}

	// Parse collected_at
	if collectedAtStr, ok := configMap["collected_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, collectedAtStr); err == nil {
			config.CollectedAt = t
		}
	}

	log.Infof("Loaded framework config for workload %s: framework=%s, profiler_dir=%s",
		workloadUID, config.Framework, config.ExtractedPaths.ProfilerDir)

	return config, nil
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
	workloadUID := taskState.WorkloadUID

	workload, err := e.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to query workload status for %s: %v", workloadUID, err)
		return true // If query fails, continue by default
	}

	if workload == nil {
		log.Infof("Workload %s not found, stopping profiler collection", workloadUID)
		return false
	}

	// Check if workload has ended (Done, Deleted, or Failed status)
	status := workload.Status
	if status == metadata.WorkloadStatusDone ||
		status == metadata.WorkloadStatusDeleted ||
		status == metadata.WorkloadStatusFailed {
		log.Infof("Workload %s has status %s, stopping profiler collection", workloadUID, status)
		return false
	}

	// Also check EndAt field
	if !workload.EndAt.IsZero() {
		log.Infof("Workload %s has EndAt set (%v), stopping profiler collection", workloadUID, workload.EndAt)
		return false
	}

	log.Debugf("Workload %s is still running, continuing profiler collection", workloadUID)
	return true
}
