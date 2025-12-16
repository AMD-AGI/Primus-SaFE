package detection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// TaskCreator responsible for creating metadata collection tasks after framework detection completes
type TaskCreator struct {
	taskFacade     database.WorkloadTaskFacadeInterface
	instanceID     string
	autoCreateTask bool // whether to auto create tasks
}

// NewTaskCreator creates task creator
func NewTaskCreator(instanceID string) *TaskCreator {
	return &TaskCreator{
		taskFacade:     database.NewWorkloadTaskFacade(),
		instanceID:     instanceID,
		autoCreateTask: true, // auto creation enabled by default
	}
}

// SetAutoCreateTask sets whether to auto create tasks
func (tc *TaskCreator) SetAutoCreateTask(auto bool) {
	tc.autoCreateTask = auto
}

// OnDetectionCompleted called when detection completes
// Creates metadata collection task and profiler collection task based on detection result
func (tc *TaskCreator) OnDetectionCompleted(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	if !tc.autoCreateTask {
		log.Debugf("Auto task creation disabled, skipping task creation for workload %s", workloadUID)
		return nil
	}

	log.Infof("Processing detection for workload %s (frameworks: %v, status: %s)",
		workloadUID, detection.Frameworks, detection.Status)

	// 1. Try to create profiler collection task early (less strict requirements)
	// Only requires: PyTorch framework + Training workload
	// Don't wait for detection status to be confirmed/verified to avoid missing early profiler files
	if err := tc.createProfilerCollectionTask(ctx, workloadUID, detection); err != nil {
		log.Warnf("Failed to create profiler collection task for workload %s: %v", workloadUID, err)
		// Continue - don't block metadata collection task creation
	}

	// 2. Create metadata collection task (requires confirmed/verified status)
	// Only create tasks for confirmed or verified detections
	if detection.Status != coreModel.DetectionStatusConfirmed &&
		detection.Status != coreModel.DetectionStatusVerified {
		log.Debugf("Detection status is %s (not confirmed/verified), skipping metadata collection task for workload %s",
			detection.Status, workloadUID)
		return nil
	}

	// Only create metadata collection tasks for training workloads
	if !tc.isTrainingWorkload(detection) {
		log.Debugf("Workload %s is not a training task, skipping metadata collection task", workloadUID)
		return nil
	}

	log.Infof("Creating metadata collection task for workload %s (frameworks: %v)",
		workloadUID, detection.Frameworks)

	if err := tc.createMetadataCollectionTask(ctx, workloadUID, detection); err != nil {
		return fmt.Errorf("failed to create metadata collection task: %w", err)
	}

	return nil
}

// createMetadataCollectionTask creates metadata collection task
func (tc *TaskCreator) createMetadataCollectionTask(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	// Note: workload-related specific info (pod, node, etc.) is stored in ai_workload_metadata table
	// ext here only stores task execution context
	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    constant.TaskTypeMetadataCollection,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			// Task execution configuration
			"auto_restart":        true,
			"priority":            100,
			"max_retries":         3,
			"retry_count":         0,
			"timeout":             30, // 30 second timeout
			"include_tensorboard": true,
			"include_metrics":     true,

			// Task metadata
			"created_by":   "detection_manager",
			"created_at":   time.Now().Format(time.RFC3339),
			"triggered_by": "framework_detection",

			// Detection summary info (for logging and debugging)
			"detection_frameworks": detection.Frameworks,
			"detection_confidence": detection.Confidence,
		},
	}

	if err := tc.taskFacade.UpsertTask(ctx, task); err != nil {
		return fmt.Errorf("failed to upsert task: %w", err)
	}

	log.Infof("Metadata collection task created for workload %s", workloadUID)
	return nil
}

// createProfilerCollectionTask creates profiler collection task
// Requirements: PyTorch framework + Training workload (no detection status check)
// This allows early profiler file collection to avoid missing training startup files
func (tc *TaskCreator) createProfilerCollectionTask(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	// Check 1: Must be PyTorch framework
	if !tc.isPyTorchFramework(detection) {
		log.Debugf("Workload %s is not PyTorch, skipping profiler collection", workloadUID)
		return nil
	}

	// Check 2: Must be training workload
	if !tc.isTrainingWorkload(detection) {
		log.Debugf("Workload %s is not a training task, skipping profiler collection", workloadUID)
		return nil
	}

	log.Infof("Creating profiler collection task for PyTorch training workload %s (early trigger, detection status: %s)",
		workloadUID, detection.Status)

	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    constant.TaskTypeProfilerCollection,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			// Task execution configuration
			"auto_restart":        true,
			"priority":            50, // Lower than metadata collection
			"max_retries":         3,
			"retry_count":         0,
			"timeout":             600, // 10 minutes
			"collection_interval": 300, // 5 minutes
			"max_executions":      0,   // Unlimited, until training stops
			"execution_count":     0,

			// Collection configuration
			"auto_collect":   true,
			"min_confidence": "medium",
			"max_file_size":  1073741824, // 1GB

			// Task metadata
			"created_by":   "detection_manager",
			"created_at":   time.Now().Format(time.RFC3339),
			"triggered_by": "framework_detection_early", // Early trigger (no status check)

			// Detection summary info (for logging and debugging)
			"detection_frameworks": detection.Frameworks,
			"detection_confidence": detection.Confidence,
			"detection_status":     string(detection.Status), // Record detection status for reference

			// Note: Task created early (before detection confirmed) to avoid missing startup profiler files
			"early_trigger": true,
		},
	}

	if err := tc.taskFacade.UpsertTask(ctx, task); err != nil {
		return fmt.Errorf("failed to upsert task: %w", err)
	}

	log.Infof("Profiler collection task created successfully for workload %s", workloadUID)
	return nil
}

// isTrainingWorkload determines if it's a training workload
func (tc *TaskCreator) isTrainingWorkload(detection *coreModel.FrameworkDetection) bool {
	// Check TaskType
	for _, source := range detection.Sources {
		// If any source is marked as training, consider it a training task
		if source.Type == "training" || source.Type == "" {
			return true
		}
	}

	// Default to training task (unless explicitly marked as inference)
	return true
}

// isPyTorchFramework checks if detection contains PyTorch framework
func (tc *TaskCreator) isPyTorchFramework(detection *coreModel.FrameworkDetection) bool {
	if detection == nil {
		return false
	}

	for _, framework := range detection.Frameworks {
		fw := strings.ToLower(framework)
		if fw == "pytorch" || strings.Contains(fw, "torch") {
			return true
		}
	}

	return false
}

// extractSourceNames extracts detection source names
func (tc *TaskCreator) extractSourceNames(detection *coreModel.FrameworkDetection) []string {
	sources := []string{}
	seen := make(map[string]bool)

	for _, source := range detection.Sources {
		if !seen[source.Source] {
			sources = append(sources, source.Source)
			seen[source.Source] = true
		}
	}

	return sources
}

// RegisterWithDetectionManager registers TaskCreator with DetectionManager
// as a detection event listener
func RegisterTaskCreatorWithDetectionManager(
	detectionMgr *framework.FrameworkDetectionManager,
	instanceID string,
) *TaskCreator {
	taskCreator := NewTaskCreator(instanceID)

	// Create an adapter to convert DetectionEvent to TaskCreator calls
	listener := &detectionEventAdapter{
		taskCreator: taskCreator,
	}

	detectionMgr.RegisterListener(listener)

	log.Info("TaskCreator registered with DetectionManager as event listener")
	return taskCreator
}

// detectionEventAdapter adapts DetectionEvent to TaskCreator
type detectionEventAdapter struct {
	taskCreator *TaskCreator
}

// OnDetectionEvent implements DetectionEventListener interface
func (a *detectionEventAdapter) OnDetectionEvent(
	ctx context.Context,
	event *framework.DetectionEvent,
) error {
	// Only handle completed and updated events
	if event.Type != framework.DetectionEventTypeUpdated &&
		event.Type != framework.DetectionEventTypeCompleted {
		return nil
	}

	if event.Detection == nil {
		return nil
	}

	// Call TaskCreator to create task
	return a.taskCreator.OnDetectionCompleted(ctx, event.WorkloadUID, event.Detection)
}
