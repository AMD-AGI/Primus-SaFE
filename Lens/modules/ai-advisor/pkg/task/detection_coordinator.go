package task

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
)

const (
	// Default configuration values
	DefaultInitialDelay      = 30 * time.Second
	DefaultRetryInterval     = 30 * time.Second
	DefaultMaxRetryInterval  = 60 * time.Second
	DefaultConfirmThreshold  = 0.70 // Lowered from 0.85 to allow single-source confirmation
	DefaultMinPodAge         = 30 * time.Second
	DefaultMaxAttemptCount   = 5
	DefaultSubTaskTimeout    = 60 * time.Second
)

// CollectionPlan represents a plan to collect evidence from a specific source
type CollectionPlan struct {
	TaskType   string                 // Task type to create
	Source     string                 // Detection source (process, log, image, label)
	Priority   int                    // Higher priority = execute first
	Params     map[string]interface{} // Task parameters
}

// DetectionCoordinator coordinates framework detection for a workload
// It manages the state machine and orchestrates sub-tasks for evidence collection
type DetectionCoordinator struct {
	coreTask.BaseExecutor

	podProber          *common.PodProber
	coverageFacade     database.DetectionCoverageFacadeInterface
	taskFacade         database.WorkloadTaskFacadeInterface
	detectionFacade    database.WorkloadDetectionFacadeInterface
	evidenceFacade     database.WorkloadDetectionEvidenceFacadeInterface
	evidenceAggregator *detection.EvidenceAggregator
	taskCreator        *detection.TaskCreator
}

// NewDetectionCoordinator creates a new DetectionCoordinator
func NewDetectionCoordinator(collector *metadata.Collector, instanceID string) *DetectionCoordinator {
	return &DetectionCoordinator{
		podProber:          common.NewPodProber(collector),
		coverageFacade:     database.NewDetectionCoverageFacade(),
		taskFacade:         database.NewWorkloadTaskFacade(),
		detectionFacade:    database.NewWorkloadDetectionFacade(),
		evidenceFacade:     database.NewWorkloadDetectionEvidenceFacade(),
		evidenceAggregator: detection.NewEvidenceAggregator(),
		taskCreator:        detection.NewTaskCreator(instanceID),
	}
}

// NewDetectionCoordinatorWithDeps creates coordinator with custom dependencies
func NewDetectionCoordinatorWithDeps(
	podProber *common.PodProber,
	coverageFacade database.DetectionCoverageFacadeInterface,
	taskFacade database.WorkloadTaskFacadeInterface,
	detectionFacade database.WorkloadDetectionFacadeInterface,
	evidenceFacade database.WorkloadDetectionEvidenceFacadeInterface,
	evidenceAggregator *detection.EvidenceAggregator,
	taskCreator *detection.TaskCreator,
) *DetectionCoordinator {
	return &DetectionCoordinator{
		podProber:          podProber,
		coverageFacade:     coverageFacade,
		taskFacade:         taskFacade,
		detectionFacade:    detectionFacade,
		evidenceFacade:     evidenceFacade,
		evidenceAggregator: evidenceAggregator,
		taskCreator:        taskCreator,
	}
}

// GetTaskType returns the task type
func (c *DetectionCoordinator) GetTaskType() string {
	return constant.TaskTypeDetectionCoordinator
}

// Validate validates task parameters
func (c *DetectionCoordinator) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Execute executes the detection coordinator state machine
func (c *DetectionCoordinator) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task
	workloadUID := task.WorkloadUID

	// Get current state
	state := c.GetExtString(task, "coordinator_state")
	if state == "" {
		state = constant.CoordinatorStateInit
	}

	log.Infof("DetectionCoordinator executing for workload %s, state=%s", workloadUID, state)

	updates := map[string]interface{}{
		"last_execution_at": time.Now().Format(time.RFC3339),
	}

	// Execute state machine
	var nextState string
	var err error

	switch state {
	case constant.CoordinatorStateInit:
		nextState, err = c.handleInitState(ctx, task, updates)

	case constant.CoordinatorStateWaiting:
		nextState, err = c.handleWaitingState(ctx, task, updates)

	case constant.CoordinatorStateProbing:
		nextState, err = c.handleProbingState(ctx, task, updates)

	case constant.CoordinatorStateAnalyzing:
		nextState, err = c.handleAnalyzingState(ctx, task, updates)

	case constant.CoordinatorStateConfirmed:
		nextState, err = c.handleConfirmedState(ctx, task, updates)

	case constant.CoordinatorStateCompleted:
		// Task is done
		log.Infof("DetectionCoordinator completed for workload %s", workloadUID)
		updates["coordinator_state"] = constant.CoordinatorStateCompleted
		return coreTask.SuccessResult(updates), nil

	default:
		err = fmt.Errorf("unknown coordinator state: %s", state)
	}

	if err != nil {
		log.Warnf("DetectionCoordinator error for workload %s: %v", workloadUID, err)
		updates["last_error"] = err.Error()
		updates["error_at"] = time.Now().Format(time.RFC3339)
	}

	// Update state
	if nextState != "" && nextState != state {
		updates["coordinator_state"] = nextState
		updates["state_changed_at"] = time.Now().Format(time.RFC3339)
		log.Infof("DetectionCoordinator state transition for workload %s: %s -> %s", workloadUID, state, nextState)
	}

	// Determine result based on next state
	if nextState == constant.CoordinatorStateCompleted {
		return coreTask.SuccessResult(updates), nil
	}

	// Continue running - return reschedule result so task will be re-picked by scheduler
	// This allows the state machine to progress on the next execution cycle
	return coreTask.RescheduleResult(updates), nil
}

// handleInitState handles the INIT state
// Initializes coverage records and transitions to WAITING
func (c *DetectionCoordinator) handleInitState(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	log.Infof("Initializing detection coverage for workload %s", workloadUID)

	// Initialize coverage records for all sources
	if err := c.coverageFacade.InitializeCoverageForWorkload(ctx, workloadUID); err != nil {
		log.Warnf("Failed to initialize coverage (may already exist): %v", err)
		// Not a fatal error - coverage might already exist
	}

	updates["initialized_at"] = time.Now().Format(time.RFC3339)

	// Calculate next schedule time (initial delay)
	initialDelay := c.getInitialDelay(task)
	nextSchedule := time.Now().Add(initialDelay)
	updates["next_schedule_at"] = nextSchedule.Format(time.RFC3339)

	return constant.CoordinatorStateWaiting, nil
}

// handleWaitingState handles the WAITING state
// Checks if it's time to schedule and generates collection plans
func (c *DetectionCoordinator) handleWaitingState(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Check if we should schedule now
	if !c.shouldScheduleNow(task) {
		log.Debugf("Not yet time to schedule for workload %s", workloadUID)
		return constant.CoordinatorStateWaiting, nil
	}

	// Check workload status - if terminated, complete the task
	if c.isWorkloadTerminated(ctx, workloadUID) {
		log.Infof("Workload %s terminated, completing coordinator", workloadUID)
		updates["terminated_reason"] = "workload_terminated"
		return constant.CoordinatorStateCompleted, nil
	}

	// Generate collection plans
	plans := c.planCollectionTasks(ctx, workloadUID, task)

	if len(plans) == 0 {
		// Check if we have any evidence collected - if so, move to ANALYZING
		if c.hasAnyEvidenceCollected(ctx, workloadUID) {
			log.Infof("No more collection plans but evidence exists for workload %s, moving to ANALYZING", workloadUID)
			return constant.CoordinatorStateAnalyzing, nil
		}
		log.Debugf("No collection plans for workload %s, staying in WAITING", workloadUID)
		// Update next schedule time
		c.updateNextSchedule(task, updates)
		return constant.CoordinatorStateWaiting, nil
	}

	// Schedule sub-tasks
	scheduledCount := 0
	for _, plan := range plans {
		if err := c.scheduleSubTask(ctx, workloadUID, plan); err != nil {
			log.Warnf("Failed to schedule %s task for workload %s: %v", plan.TaskType, workloadUID, err)
			continue
		}
		scheduledCount++
		log.Infof("Scheduled %s task for workload %s (priority=%d)", plan.TaskType, workloadUID, plan.Priority)
	}

	if scheduledCount > 0 {
		updates["scheduled_tasks"] = scheduledCount
		updates["probing_started_at"] = time.Now().Format(time.RFC3339)
		return constant.CoordinatorStateProbing, nil
	}

	// No tasks scheduled, stay waiting
	c.updateNextSchedule(task, updates)
	return constant.CoordinatorStateWaiting, nil
}

// handleProbingState handles the PROBING state
// Waits for all sub-tasks to complete
func (c *DetectionCoordinator) handleProbingState(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Check if all sub-tasks are complete
	allComplete, err := c.areSubTasksComplete(ctx, workloadUID)
	if err != nil {
		return constant.CoordinatorStateProbing, err
	}

	if !allComplete {
		// Check for timeout
		probingStartStr := c.GetExtString(task, "probing_started_at")
		if probingStartStr != "" {
			if probingStart, err := time.Parse(time.RFC3339, probingStartStr); err == nil {
				timeout := c.getSubTaskTimeout(task)
				if time.Since(probingStart) > timeout {
					log.Warnf("Sub-tasks timed out for workload %s", workloadUID)
					updates["timeout_at"] = time.Now().Format(time.RFC3339)
					// Move to analyzing anyway
					return constant.CoordinatorStateAnalyzing, nil
				}
			}
		}

		// Still waiting
		return constant.CoordinatorStateProbing, nil
	}

	// All tasks complete, move to analyzing
	updates["probing_completed_at"] = time.Now().Format(time.RFC3339)
	return constant.CoordinatorStateAnalyzing, nil
}

// handleAnalyzingState handles the ANALYZING state
// Aggregates evidence and determines if framework is confirmed
func (c *DetectionCoordinator) handleAnalyzingState(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Aggregate evidence
	result, err := c.evidenceAggregator.AggregateEvidence(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to aggregate evidence for workload %s: %v", workloadUID, err)
		// Not fatal, continue to next cycle
		c.updateNextSchedule(task, updates)
		return constant.CoordinatorStateWaiting, nil
	}

	updates["aggregation_result"] = map[string]interface{}{
		"framework":      result.Framework,
		"confidence":     result.Confidence,
		"status":         string(result.Status),
		"evidence_count": result.EvidenceCount,
		"sources":        result.Sources,
	}

	// Check if confirmed
	confirmThreshold := c.getConfirmThreshold(task)
	if result.Confidence >= confirmThreshold && result.Framework != "" {
		log.Infof("Framework confirmed for workload %s: %s (confidence=%.2f)",
			workloadUID, result.Framework, result.Confidence)

		updates["confirmed_framework"] = result.Framework
		updates["confirmed_confidence"] = result.Confidence
		updates["confirmed_at"] = time.Now().Format(time.RFC3339)

		return constant.CoordinatorStateConfirmed, nil
	}

	// Not confirmed yet
	log.Infof("Framework not yet confirmed for workload %s (confidence=%.2f < %.2f)",
		workloadUID, result.Confidence, confirmThreshold)

	// Increment attempt count
	attemptCount := c.GetExtInt(task, "attempt_count") + 1
	updates["attempt_count"] = attemptCount

	// Update next schedule time with backoff
	c.updateNextScheduleWithBackoff(task, updates, attemptCount)

	return constant.CoordinatorStateWaiting, nil
}

// handleConfirmedState handles the CONFIRMED state
// Creates follow-up tasks and transitions to COMPLETED
func (c *DetectionCoordinator) handleConfirmedState(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Get detection result
	detection, err := c.detectionFacade.GetDetection(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to get detection for workload %s: %v", workloadUID, err)
	}

	// Create follow-up tasks if detection confirmed
	if detection != nil && c.taskCreator != nil {
		// Convert to FrameworkDetection for TaskCreator
		fwDetection := c.convertToFrameworkDetection(detection)
		if err := c.taskCreator.OnDetectionCompleted(ctx, workloadUID, fwDetection); err != nil {
			log.Warnf("Failed to create follow-up tasks for workload %s: %v", workloadUID, err)
		} else {
			updates["followup_tasks_created"] = true
		}
	}

	updates["completed_at"] = time.Now().Format(time.RFC3339)

	return constant.CoordinatorStateCompleted, nil
}

// planCollectionTasks generates collection plans based on coverage status
func (c *DetectionCoordinator) planCollectionTasks(
	ctx context.Context,
	workloadUID string,
	task *model.WorkloadTaskState,
) []*CollectionPlan {
	plans := []*CollectionPlan{}

	// Check process evidence
	processCoverage, _ := c.coverageFacade.GetCoverage(ctx, workloadUID, constant.DetectionSourceProcess)
	if c.shouldCollectProcess(ctx, workloadUID, processCoverage, task) {
		plans = append(plans, &CollectionPlan{
			TaskType: constant.TaskTypeProcessProbe,
			Source:   constant.DetectionSourceProcess,
			Priority: 100,
			Params:   map[string]interface{}{},
		})
	}

	// Check image evidence
	imageCoverage, _ := c.coverageFacade.GetCoverage(ctx, workloadUID, constant.DetectionSourceImage)
	if c.shouldCollectImage(imageCoverage) {
		plans = append(plans, &CollectionPlan{
			TaskType: constant.TaskTypeImageProbe,
			Source:   constant.DetectionSourceImage,
			Priority: 60,
			Params:   map[string]interface{}{},
		})
	}

	// Check label evidence
	labelCoverage, _ := c.coverageFacade.GetCoverage(ctx, workloadUID, constant.DetectionSourceLabel)
	if c.shouldCollectLabel(labelCoverage) {
		plans = append(plans, &CollectionPlan{
			TaskType: constant.TaskTypeLabelProbe,
			Source:   constant.DetectionSourceLabel,
			Priority: 40,
			Params:   map[string]interface{}{},
		})
	}

	// Check log evidence (backfill mode)
	logCoverage, _ := c.coverageFacade.GetCoverage(ctx, workloadUID, constant.DetectionSourceLog)
	if window := c.findUnscannedLogWindow(logCoverage); window != nil {
		plans = append(plans, &CollectionPlan{
			TaskType: constant.TaskTypeLogDetection,
			Source:   constant.DetectionSourceLog,
			Priority: 80,
			Params: map[string]interface{}{
				"from": window.From.Format(time.RFC3339),
				"to":   window.To.Format(time.RFC3339),
				"mode": "backfill",
			},
		})
	}

	// Sort by priority (descending)
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Priority > plans[j].Priority
	})

	return plans
}

// shouldCollectProcess determines if process info should be collected
func (c *DetectionCoordinator) shouldCollectProcess(
	ctx context.Context,
	workloadUID string,
	coverage *model.DetectionCoverage,
	task *model.WorkloadTaskState,
) bool {
	// Already collected with evidence
	if coverage != nil && coverage.Status == constant.DetectionStatusCollected && coverage.EvidenceCount > 0 {
		return false
	}

	// Currently collecting
	if coverage != nil && coverage.Status == constant.DetectionStatusCollecting {
		return false
	}

	// Too many attempts
	if coverage != nil && coverage.AttemptCount >= DefaultMaxAttemptCount {
		return false
	}

	// Check pod readiness
	pod, err := c.podProber.SelectTargetPod(ctx, workloadUID)
	if err != nil || pod == nil {
		return false
	}

	if !c.podProber.IsPodReady(ctx, pod) {
		return false
	}

	// Check pod age
	minPodAge := c.getMinPodAge(task)
	if c.podProber.GetPodAge(ctx, pod) < minPodAge {
		return false
	}

	return true
}

// shouldCollectImage determines if image info should be collected
func (c *DetectionCoordinator) shouldCollectImage(coverage *model.DetectionCoverage) bool {
	// Only collect once
	if coverage == nil {
		return true
	}
	return coverage.Status == constant.DetectionStatusPending
}

// shouldCollectLabel determines if label info should be collected
func (c *DetectionCoordinator) shouldCollectLabel(coverage *model.DetectionCoverage) bool {
	// Only collect once
	if coverage == nil {
		return true
	}
	return coverage.Status == constant.DetectionStatusPending
}

// hasAnyEvidenceCollected checks if any source has collected evidence
func (c *DetectionCoordinator) hasAnyEvidenceCollected(ctx context.Context, workloadUID string) bool {
	sources := []string{
		constant.DetectionSourceProcess,
		constant.DetectionSourceImage,
		constant.DetectionSourceLabel,
		constant.DetectionSourceLog,
	}

	for _, source := range sources {
		coverage, err := c.coverageFacade.GetCoverage(ctx, workloadUID, source)
		if err != nil {
			continue
		}
		if coverage != nil && coverage.Status == constant.DetectionStatusCollected {
			// Found collected evidence
			return true
		}
	}

	// Also check if there's any evidence in the evidence table
	evidences, err := c.evidenceFacade.ListEvidenceByWorkload(ctx, workloadUID)
	if err == nil && len(evidences) > 0 {
		return true
	}

	return false
}

// TimeWindow represents a time window for log scanning
type TimeWindow struct {
	From time.Time
	To   time.Time
}

// findUnscannedLogWindow finds uncovered log time window
func (c *DetectionCoordinator) findUnscannedLogWindow(coverage *model.DetectionCoverage) *TimeWindow {
	if coverage == nil {
		return nil
	}

	// No logs available yet
	if coverage.LogAvailableTo.IsZero() {
		return nil
	}

	// Case 1: Never scanned
	if coverage.CoveredTo.IsZero() {
		return &TimeWindow{
			From: coverage.LogAvailableFrom,
			To:   coverage.LogAvailableTo,
		}
	}

	// Case 2: New logs available
	if coverage.LogAvailableTo.After(coverage.CoveredTo) {
		return &TimeWindow{
			From: coverage.CoveredTo,
			To:   coverage.LogAvailableTo,
		}
	}

	// Case 3: Early logs missed
	if !coverage.LogAvailableFrom.IsZero() && coverage.CoveredFrom.After(coverage.LogAvailableFrom) {
		return &TimeWindow{
			From: coverage.LogAvailableFrom,
			To:   coverage.CoveredFrom,
		}
	}

	return nil
}

// scheduleSubTask creates and schedules a sub-task
func (c *DetectionCoordinator) scheduleSubTask(
	ctx context.Context,
	workloadUID string,
	plan *CollectionPlan,
) error {
	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    plan.TaskType,
		Status:      constant.TaskStatusPending,
		Ext:         model.ExtType(plan.Params),
	}

	// Add common metadata
	if task.Ext == nil {
		task.Ext = model.ExtType{}
	}
	task.Ext["source"] = plan.Source
	task.Ext["priority"] = plan.Priority
	task.Ext["created_by"] = "detection_coordinator"
	task.Ext["created_at"] = time.Now().Format(time.RFC3339)

	return c.taskFacade.UpsertTask(ctx, task)
}

// areSubTasksComplete checks if all scheduled sub-tasks are complete
func (c *DetectionCoordinator) areSubTasksComplete(ctx context.Context, workloadUID string) (bool, error) {
	subTaskTypes := []string{
		constant.TaskTypeProcessProbe,
		constant.TaskTypeLogDetection,
		constant.TaskTypeImageProbe,
		constant.TaskTypeLabelProbe,
	}

	for _, taskType := range subTaskTypes {
		task, err := c.taskFacade.GetTask(ctx, workloadUID, taskType)
		if err != nil {
			return false, err
		}

		// If task exists and is still running or pending
		if task != nil {
			if task.Status == constant.TaskStatusRunning || task.Status == constant.TaskStatusPending {
				return false, nil
			}
		}
	}

	return true, nil
}

// isWorkloadTerminated checks if workload is terminated
func (c *DetectionCoordinator) isWorkloadTerminated(ctx context.Context, workloadUID string) bool {
	workloadFacade := database.GetFacade().GetWorkload()
	workload, err := workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil || workload == nil {
		return true // Assume terminated if can't find
	}

	// Check status
	terminatedStatuses := map[string]bool{
		"Completed": true,
		"Failed":    true,
		"Succeeded": true,
		"Stopped":   true,
	}

	// Check if DeletedAt is set (soft delete)
	isDeleted := workload.DeletedAt.Valid

	return isDeleted || terminatedStatuses[string(workload.Status)]
}

// shouldScheduleNow checks if it's time to schedule
func (c *DetectionCoordinator) shouldScheduleNow(task *model.WorkloadTaskState) bool {
	nextScheduleStr := c.GetExtString(task, "next_schedule_at")
	if nextScheduleStr == "" {
		return true
	}

	nextSchedule, err := time.Parse(time.RFC3339, nextScheduleStr)
	if err != nil {
		return true
	}

	return time.Now().After(nextSchedule)
}

// updateNextSchedule updates the next schedule time
func (c *DetectionCoordinator) updateNextSchedule(task *model.WorkloadTaskState, updates map[string]interface{}) {
	interval := DefaultRetryInterval
	nextSchedule := time.Now().Add(interval)
	updates["next_schedule_at"] = nextSchedule.Format(time.RFC3339)
}

// updateNextScheduleWithBackoff updates next schedule with exponential backoff
func (c *DetectionCoordinator) updateNextScheduleWithBackoff(
	task *model.WorkloadTaskState,
	updates map[string]interface{},
	attemptCount int,
) {
	baseInterval := DefaultRetryInterval
	maxInterval := DefaultMaxRetryInterval

	// Calculate interval with backoff: base * 2^(attempt-1), capped at max
	multiplier := 1 << uint(attemptCount-1) // 2^(attempt-1)
	interval := time.Duration(multiplier) * baseInterval

	if interval > maxInterval {
		interval = maxInterval
	}

	nextSchedule := time.Now().Add(interval)
	updates["next_schedule_at"] = nextSchedule.Format(time.RFC3339)
	updates["retry_interval"] = interval.String()
}

// getInitialDelay gets the initial delay from config
func (c *DetectionCoordinator) getInitialDelay(task *model.WorkloadTaskState) time.Duration {
	if delaySeconds := c.GetExtInt(task, "initial_delay"); delaySeconds > 0 {
		return time.Duration(delaySeconds) * time.Second
	}
	return DefaultInitialDelay
}

// getConfirmThreshold gets the confirmation threshold
func (c *DetectionCoordinator) getConfirmThreshold(task *model.WorkloadTaskState) float64 {
	if threshold, ok := task.Ext["confirm_threshold"].(float64); ok && threshold > 0 {
		return threshold
	}
	return DefaultConfirmThreshold
}

// getMinPodAge gets minimum pod age requirement
func (c *DetectionCoordinator) getMinPodAge(task *model.WorkloadTaskState) time.Duration {
	if ageSeconds := c.GetExtInt(task, "min_pod_age"); ageSeconds > 0 {
		return time.Duration(ageSeconds) * time.Second
	}
	return DefaultMinPodAge
}

// getSubTaskTimeout gets sub-task timeout
func (c *DetectionCoordinator) getSubTaskTimeout(task *model.WorkloadTaskState) time.Duration {
	if timeoutSeconds := c.GetExtInt(task, "sub_task_timeout"); timeoutSeconds > 0 {
		return time.Duration(timeoutSeconds) * time.Second
	}
	return DefaultSubTaskTimeout
}

// convertToFrameworkDetection converts DB model to TaskCreator's expected format
func (c *DetectionCoordinator) convertToFrameworkDetection(
	det *model.WorkloadDetection,
) *coreModel.FrameworkDetection {
	if det == nil {
		return nil
	}

	fd := &coreModel.FrameworkDetection{
		Frameworks:       []string{},
		Type:             det.WorkloadType,
		Confidence:       det.Confidence,
		Status:           coreModel.DetectionStatus(det.Status),
		FrameworkLayer:   det.FrameworkLayer,
		WrapperFramework: det.WrapperFramework,
		BaseFramework:    det.BaseFramework,
	}

	// Parse frameworks from ExtJSON
	if len(det.Frameworks) > 0 {
		var frameworks []string
		if err := det.Frameworks.UnmarshalTo(&frameworks); err == nil {
			fd.Frameworks = frameworks
		}
	}

	if len(fd.Frameworks) == 0 && det.Framework != "" {
		fd.Frameworks = []string{det.Framework}
	}

	return fd
}

// Cancel cancels the coordinator task
func (c *DetectionCoordinator) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("DetectionCoordinator cancelled for workload %s", task.WorkloadUID)

	// Cancel any running sub-tasks
	workloadUID := task.WorkloadUID
	subTaskTypes := []string{
		constant.TaskTypeProcessProbe,
		constant.TaskTypeLogDetection,
		constant.TaskTypeImageProbe,
		constant.TaskTypeLabelProbe,
	}

	for _, taskType := range subTaskTypes {
		subTask, err := c.taskFacade.GetTask(ctx, workloadUID, taskType)
		if err != nil || subTask == nil {
			continue
		}
		if subTask.Status == constant.TaskStatusRunning || subTask.Status == constant.TaskStatusPending {
			c.taskFacade.UpdateTaskStatus(ctx, workloadUID, taskType, constant.TaskStatusCancelled)
		}
	}

	return nil
}

