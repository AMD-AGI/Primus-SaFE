// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package pipeline implements the WorkloadAnalysisPipeline, a unified state-machine
// that replaces DetectionCoordinator with intent-aware analysis. It orchestrates
// evidence collection from multiple sources, runs deterministic evaluation, optionally
// requests LLM analysis from Conductor, and merges results into a final IntentResult.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aigateway"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/snapshot"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
)

const (
	// Configuration defaults
	DefaultCollectTimeout   = 120 * time.Second
	DefaultLLMTimeout       = 10 * time.Minute
	DefaultEvalInterval     = 5 * time.Minute
	DefaultMonitorInterval  = 30 * time.Minute
	DefaultConfidenceGate   = 0.75
	DefaultMaxCollectCycles = 3
)

// CollectorPlan describes a single evidence collection task to schedule
type CollectorPlan struct {
	TaskType string
	Source   string
	Priority int
	Params   map[string]interface{}
}

// WorkloadAnalysisPipeline is the unified orchestrator for workload intent analysis.
// It implements coreTask.TaskExecutor and is driven by the TaskScheduler.
//
// State machine:
//
//	INIT -> COLLECTING -> EVALUATING -> [REQUESTING_LLM -> MERGING_RESULT ->] CONFIRMED -> MONITORING -> COMPLETED
//
// The pipeline can short-circuit from EVALUATING to CONFIRMED if the deterministic
// evaluator produces a high-confidence result, skipping LLM analysis entirely.
type WorkloadAnalysisPipeline struct {
	coreTask.BaseExecutor

	detectionFacade  database.WorkloadDetectionFacadeInterface
	coverageFacade   database.DetectionCoverageFacadeInterface
	taskFacade       database.WorkloadTaskFacadeInterface
	evidenceFacade   database.WorkloadDetectionEvidenceFacadeInterface
	snapshotFacade   database.WorkloadCodeSnapshotFacadeInterface
	imageCacheFacade database.ImageRegistryCacheFacadeInterface
	ruleFacade       database.IntentRuleFacadeInterface

	evaluator              *EvidenceEvaluator
	specCollector          *SpecCollector
	processCollector       *ProcessEvidenceCollector
	imageRegCollector      *ImageRegistryCollector
	codeSnapshotCollector  *CodeSnapshotCollector
	snapshotStore          snapshot.Store
	conductorURL           string // deprecated: use gwClient
	gwClient               *aigateway.Client
	instanceID             string
}

// NewWorkloadAnalysisPipeline creates a new pipeline executor.
// podProber may be nil; code snapshot collection will be skipped if so.
// snapshotStore may be nil; file contents will then be stored inline in the DB.
// aiGatewayURL is the base URL of the AI Gateway API (e.g. "http://ai-gateway:8080/api/v1").
// If empty, LLM analysis will be skipped.
func NewWorkloadAnalysisPipeline(conductorURL string, aiGatewayURL string, instanceID string, podProber *common.PodProber, snapshotStore snapshot.Store) *WorkloadAnalysisPipeline {
	p := &WorkloadAnalysisPipeline{
		detectionFacade:  database.NewWorkloadDetectionFacade(),
		coverageFacade:   database.NewDetectionCoverageFacade(),
		taskFacade:       database.NewWorkloadTaskFacade(),
		evidenceFacade:   database.NewWorkloadDetectionEvidenceFacade(),
		snapshotFacade:   database.NewWorkloadCodeSnapshotFacade(),
		imageCacheFacade: database.NewImageRegistryCacheFacade(),
		ruleFacade:       database.NewIntentRuleFacade(),
		evaluator:           NewEvidenceEvaluator(),
		specCollector:       NewSpecCollector(),
		processCollector:    NewProcessEvidenceCollector(),
		imageRegCollector:   NewImageRegistryCollector(),
		conductorURL:        conductorURL,
		instanceID:          instanceID,
	}
	if aiGatewayURL != "" {
		p.gwClient = aigateway.NewClient(aiGatewayURL)
	}
	p.snapshotStore = snapshotStore
	if podProber != nil {
		p.codeSnapshotCollector = NewCodeSnapshotCollector(podProber, snapshotStore)
	}
	return p
}

// GetTaskType implements coreTask.TaskExecutor
func (p *WorkloadAnalysisPipeline) GetTaskType() string {
	return constant.TaskTypeAnalysisPipeline
}

// Validate implements coreTask.TaskExecutor
func (p *WorkloadAnalysisPipeline) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Cancel implements coreTask.TaskExecutor
func (p *WorkloadAnalysisPipeline) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	workloadUID := task.WorkloadUID
	log.Infof("AnalysisPipeline cancelled for workload %s", workloadUID)
	_ = p.detectionFacade.UpdateIntentState(ctx, workloadUID, constant.IntentStateFailed)
	return nil
}

// Execute implements coreTask.TaskExecutor - drives the state machine
func (p *WorkloadAnalysisPipeline) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task
	workloadUID := task.WorkloadUID

	state := p.GetExtString(task, "pipeline_state")
	if state == "" {
		state = constant.PipelineStateInit
	}

	log.Infof("AnalysisPipeline executing for workload %s, state=%s", workloadUID, state)

	updates := map[string]interface{}{
		"last_execution_at": time.Now().Format(time.RFC3339),
	}

	var nextState string
	var err error

	switch state {
	case constant.PipelineStateInit:
		nextState, err = p.handleInit(ctx, task, updates)

	case constant.PipelineStateCollecting:
		nextState, err = p.handleCollecting(ctx, task, updates)

	case constant.PipelineStateEvaluating:
		nextState, err = p.handleEvaluating(ctx, task, updates)

	case constant.PipelineStateRequestingLLM:
		nextState, err = p.handleRequestingLLM(ctx, task, updates)

	case constant.PipelineStateMergingResult:
		nextState, err = p.handleMergingResult(ctx, task, updates)

	case constant.PipelineStateConfirmed:
		nextState, err = p.handleConfirmed(ctx, task, updates)

	case constant.PipelineStateMonitoring:
		nextState, err = p.handleMonitoring(ctx, task, updates)

	case constant.PipelineStateCompleted:
		log.Infof("AnalysisPipeline completed for workload %s", workloadUID)
		updates["pipeline_state"] = constant.PipelineStateCompleted
		return coreTask.SuccessResult(updates), nil

	default:
		err = fmt.Errorf("unknown pipeline state: %s", state)
	}

	if err != nil {
		log.Warnf("AnalysisPipeline error for workload %s in state %s: %v", workloadUID, state, err)
		updates["last_error"] = err.Error()
		updates["error_at"] = time.Now().Format(time.RFC3339)
	}

	// Record state transition
	if nextState != "" && nextState != state {
		updates["pipeline_state"] = nextState
		updates["state_changed_at"] = time.Now().Format(time.RFC3339)
		log.Infof("AnalysisPipeline state transition for workload %s: %s -> %s",
			workloadUID, state, nextState)
	}

	if nextState == constant.PipelineStateCompleted {
		return coreTask.SuccessResult(updates), nil
	}

	return coreTask.RescheduleResult(updates), nil
}

// ---------------------------------------------------------------------------
// State handlers
// ---------------------------------------------------------------------------

// handleInit initializes coverage records and intent state, then transitions to COLLECTING
func (p *WorkloadAnalysisPipeline) handleInit(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	log.Infof("Initializing analysis pipeline for workload %s", workloadUID)

	// Initialize detection coverage for new sources
	if err := p.coverageFacade.InitializeCoverageForWorkload(ctx, workloadUID); err != nil {
		log.Warnf("Failed to initialize coverage (may already exist): %v", err)
	}

	// Set intent_state to collecting
	if err := p.detectionFacade.UpdateIntentState(ctx, workloadUID, constant.IntentStateCollecting); err != nil {
		log.Warnf("Failed to update intent state: %v", err)
	}

	// Determine analysis mode from task ext or default to full
	mode := p.GetExtString(task, "analysis_mode")
	if mode == "" {
		mode = constant.AnalysisModeFull
	}
	updates["analysis_mode"] = mode
	updates["initialized_at"] = time.Now().Format(time.RFC3339)
	updates["collect_cycle"] = 0

	return constant.PipelineStateCollecting, nil
}

// handleCollecting schedules evidence collection sub-tasks and waits for completion
func (p *WorkloadAnalysisPipeline) handleCollecting(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Check if workload terminated
	if p.isWorkloadTerminated(ctx, workloadUID) {
		log.Infof("Workload %s terminated, moving to evaluating with available evidence", workloadUID)
		return constant.PipelineStateEvaluating, nil
	}

	// Check if sub-tasks have been scheduled already this cycle
	tasksScheduled := p.GetExtBool(task, "collect_tasks_scheduled")
	if !tasksScheduled {
		// Schedule collection sub-tasks
		plans := p.planCollectors(ctx, workloadUID, task)
		if len(plans) == 0 {
			log.Infof("No collectors to schedule for workload %s, moving to evaluating", workloadUID)
			return constant.PipelineStateEvaluating, nil
		}

		scheduled := 0
		for _, plan := range plans {
			if err := p.scheduleSubTask(ctx, workloadUID, plan); err != nil {
				log.Warnf("Failed to schedule %s for workload %s: %v", plan.TaskType, workloadUID, err)
				continue
			}
			scheduled++
		}

		updates["collect_tasks_scheduled"] = true
		updates["collect_started_at"] = time.Now().Format(time.RFC3339)
		updates["collect_scheduled_count"] = scheduled

		if scheduled == 0 {
			return constant.PipelineStateEvaluating, nil
		}

		return constant.PipelineStateCollecting, nil
	}

	// Sub-tasks already scheduled - check if all complete
	allComplete, err := p.areSubTasksComplete(ctx, workloadUID)
	if err != nil {
		return constant.PipelineStateCollecting, err
	}

	if !allComplete {
		// Check timeout
		startStr := p.GetExtString(task, "collect_started_at")
		if startStr != "" {
			if start, parseErr := time.Parse(time.RFC3339, startStr); parseErr == nil {
				if time.Since(start) > DefaultCollectTimeout {
					log.Warnf("Collection timed out for workload %s, proceeding with available evidence", workloadUID)
					updates["collect_timeout"] = true
					return constant.PipelineStateEvaluating, nil
				}
			}
		}
		return constant.PipelineStateCollecting, nil
	}

	// All sub-tasks complete
	updates["collect_completed_at"] = time.Now().Format(time.RFC3339)
	updates["collect_tasks_scheduled"] = false // reset for possible re-collect

	return constant.PipelineStateEvaluating, nil
}

// handleEvaluating runs the deterministic EvidenceEvaluator
func (p *WorkloadAnalysisPipeline) handleEvaluating(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Update intent state
	_ = p.detectionFacade.UpdateIntentState(ctx, workloadUID, constant.IntentStateAnalyzing)

	// Gather evidence from DB
	evidence, err := p.gatherEvidence(ctx, workloadUID)
	if err != nil {
		return constant.PipelineStateCollecting, fmt.Errorf("failed to gather evidence: %w", err)
	}

	// Load promoted rules from DB
	rules, err := p.ruleFacade.GetPromotedRules(ctx)
	if err != nil {
		log.Warnf("Failed to load promoted rules: %v", err)
		rules = nil
	}

	// Run deterministic evaluation
	result := p.evaluator.Evaluate(evidence, rules)

	// Store evaluation snapshot in ext
	evalJSON, _ := json.Marshal(result)
	updates["eval_result"] = json.RawMessage(evalJSON)
	updates["eval_confidence"] = result.Confidence
	updates["eval_category"] = string(result.Category)

	// Decide whether to skip LLM
	confidenceGate := p.getConfidenceGate(task)
	mode := p.GetExtString(task, "analysis_mode")

	if result.Confidence >= confidenceGate || mode == constant.AnalysisModeLocal {
		// High-confidence deterministic result or local-only mode => confirm directly
		log.Infof("Deterministic evaluation sufficient for workload %s (confidence=%.2f, gate=%.2f)",
			workloadUID, result.Confidence, confidenceGate)
		updates["skip_llm"] = true
		return p.persistIntentResult(ctx, workloadUID, result, updates)
	}

	// Need LLM analysis
	log.Infof("Deterministic confidence too low for workload %s (%.2f < %.2f), requesting LLM",
		workloadUID, result.Confidence, confidenceGate)

	// Store evidence for LLM request
	evidenceJSON, _ := json.Marshal(evidence)
	updates["llm_evidence"] = json.RawMessage(evidenceJSON)

	return constant.PipelineStateRequestingLLM, nil
}

// handleRequestingLLM sends an analysis request to Conductor and waits for the response
func (p *WorkloadAnalysisPipeline) handleRequestingLLM(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Check if request already sent
	requestSent := p.GetExtBool(task, "llm_request_sent")
	if !requestSent {
		// Send async request to Conductor
		if err := p.sendConductorRequest(ctx, workloadUID, task); err != nil {
			log.Warnf("Failed to send Conductor request for workload %s: %v", workloadUID, err)

			// Retry up to 3 times before falling back to deterministic result
			retryCount := p.GetExtInt(task, "llm_send_retry_count")
			if retryCount >= 3 {
				log.Warnf("LLM request failed after %d retries for workload %s, falling back to deterministic",
					retryCount, workloadUID)
				updates["llm_error"] = err.Error()
				updates["skip_llm"] = true
				return constant.PipelineStateMergingResult, nil
			}
			updates["llm_send_retry_count"] = retryCount + 1
			updates["llm_last_error"] = err.Error()
			return constant.PipelineStateRequestingLLM, nil
		}
		updates["llm_request_sent"] = true
		updates["llm_request_at"] = time.Now().Format(time.RFC3339)
		return constant.PipelineStateRequestingLLM, nil
	}

	// Check for response via ai-gateway
	llmResult, err := p.pollGatewayTaskResult(ctx, task)
	if err != nil {
		log.Warnf("Error polling ai-gateway for workload %s: %v", workloadUID, err)
	}

	if llmResult != nil {
		// Got response
		resultJSON, _ := json.Marshal(llmResult)
		updates["llm_result"] = json.RawMessage(resultJSON)
		updates["llm_completed_at"] = time.Now().Format(time.RFC3339)
		return constant.PipelineStateMergingResult, nil
	}

	// Check timeout
	requestAtStr := p.GetExtString(task, "llm_request_at")
	if requestAtStr != "" {
		if requestAt, parseErr := time.Parse(time.RFC3339, requestAtStr); parseErr == nil {
			if time.Since(requestAt) > DefaultLLMTimeout {
				log.Warnf("LLM request timed out for workload %s", workloadUID)
				updates["llm_timeout"] = true
				return constant.PipelineStateMergingResult, nil
			}
		}
	}

	return constant.PipelineStateRequestingLLM, nil
}

// handleMergingResult merges deterministic and LLM results
func (p *WorkloadAnalysisPipeline) handleMergingResult(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Reconstruct deterministic result from ext.
	// eval_result is stored as json.RawMessage which becomes map[string]interface{}
	// after JSONB round-trip, so GetExtString won't work; use GetExtMap instead.
	var evalResult intent.IntentResult
	if evalMap := p.GetExtMap(task, "eval_result"); evalMap != nil {
		evalBytes, err := json.Marshal(evalMap)
		if err == nil {
			_ = json.Unmarshal(evalBytes, &evalResult)
		}
	} else if evalJSON := p.GetExtString(task, "eval_result"); evalJSON != "" {
		// Fallback: in case ext was somehow stored as a string
		_ = json.Unmarshal([]byte(evalJSON), &evalResult)
	}

	// Check if we have an LLM result
	skipLLM := p.GetExtBool(task, "skip_llm")
	if skipLLM {
		// Use deterministic result as-is
		return p.persistIntentResult(ctx, workloadUID, &evalResult, updates)
	}

	// Parse LLM result (same JSONB round-trip handling as eval_result)
	var llmResult intent.IntentResult
	if llmMap := p.GetExtMap(task, "llm_result"); llmMap != nil {
		llmBytes, err := json.Marshal(llmMap)
		if err == nil {
			_ = json.Unmarshal(llmBytes, &llmResult)
		}
	} else if llmJSON := p.GetExtString(task, "llm_result"); llmJSON != "" {
		_ = json.Unmarshal([]byte(llmJSON), &llmResult)
	}

	// Merge: LLM overrides deterministic for fields where LLM has higher confidence
	merged := p.mergeResults(&evalResult, &llmResult)
	mergedJSON, _ := json.Marshal(merged)
	updates["merged_result"] = json.RawMessage(mergedJSON)

	return p.persistIntentResult(ctx, workloadUID, merged, updates)
}

// handleConfirmed triggers side-effects (follow-up tasks) after intent is confirmed
func (p *WorkloadAnalysisPipeline) handleConfirmed(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	log.Infof("Intent confirmed for workload %s, triggering side-effects", workloadUID)

	// Create follow-up tasks based on confirmed intent
	det, err := p.detectionFacade.GetDetection(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to get detection for follow-up tasks: %v", err)
	}

	if det != nil {
		p.createFollowUpTasks(ctx, workloadUID, det, updates)
	}

	updates["confirmed_completed_at"] = time.Now().Format(time.RFC3339)

	// For long-running production workloads, transition to MONITORING; otherwise COMPLETED
	// Interactive development, CI/CD, and benchmark workloads always go to COMPLETED
	category := p.GetExtString(task, "eval_category")
	if (category == string(intent.CategoryPreTraining) || category == string(intent.CategoryServing)) &&
		category != string(intent.CategoryInteractiveDevelopment) &&
		category != string(intent.CategoryCICD) &&
		category != string(intent.CategoryBenchmark) {
		updates["monitor_interval"] = DefaultMonitorInterval.String()
		updates["next_monitor_at"] = time.Now().Add(DefaultMonitorInterval).Format(time.RFC3339)
		return constant.PipelineStateMonitoring, nil
	}

	return constant.PipelineStateCompleted, nil
}

// handleMonitoring periodically re-evaluates evidence for long-running workloads
func (p *WorkloadAnalysisPipeline) handleMonitoring(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) (string, error) {
	workloadUID := task.WorkloadUID

	// Check if workload is terminated
	if p.isWorkloadTerminated(ctx, workloadUID) {
		log.Infof("Monitored workload %s terminated, completing", workloadUID)
		return constant.PipelineStateCompleted, nil
	}

	// Check if it is time to re-evaluate
	nextMonitorStr := p.GetExtString(task, "next_monitor_at")
	if nextMonitorStr != "" {
		if nextMonitor, parseErr := time.Parse(time.RFC3339, nextMonitorStr); parseErr == nil {
			if time.Now().Before(nextMonitor) {
				return constant.PipelineStateMonitoring, nil
			}
		}
	}

	// Re-collect and re-evaluate
	log.Infof("Re-evaluating workload %s during monitoring", workloadUID)

	// Reset collect state to trigger fresh collection
	updates["collect_tasks_scheduled"] = false
	cycle := p.GetExtInt(task, "collect_cycle") + 1
	updates["collect_cycle"] = cycle
	updates["next_monitor_at"] = time.Now().Add(DefaultMonitorInterval).Format(time.RFC3339)

	return constant.PipelineStateCollecting, nil
}

// ---------------------------------------------------------------------------
// Helper methods
// ---------------------------------------------------------------------------

// persistIntentResult writes the IntentResult to workload_detection.
// For high-confidence results, transitions to CONFIRMED. For low-confidence
// config_based results (no cmdline evidence), transitions to ANALYZING to allow
// future evidence collection to improve the result.
func (p *WorkloadAnalysisPipeline) persistIntentResult(
	ctx context.Context,
	workloadUID string,
	result *intent.IntentResult,
	updates map[string]interface{},
) (string, error) {
	// Determine intent state: config_based with low confidence stays as "analyzing"
	// to indicate the result is provisional and may be improved with more evidence.
	const lowConfidenceThreshold = 0.6
	intentState := constant.IntentStateConfirmed
	pipelineNextState := constant.PipelineStateConfirmed
	if result.AnalysisMode == intent.AnalysisModeConfigBased && result.Confidence < lowConfidenceThreshold {
		intentState = constant.IntentStateAnalyzing
		pipelineNextState = constant.PipelineStateMonitoring
		log.Infof("Low-confidence config_based result for workload %s (%.2f < %.2f), staying in analyzing state",
			workloadUID, result.Confidence, lowConfidenceThreshold)
	}

	// Build update map for detection facade
	intentUpdates := map[string]interface{}{
		"intent_state":      intentState,
		"intent_confidence": result.Confidence,
		"intent_analyzed_at": time.Now(),
	}

	if result.Category != "" {
		intentUpdates["category"] = string(result.Category)
	}
	if result.ExpectedBehavior != "" {
		intentUpdates["expected_behavior"] = string(result.ExpectedBehavior)
	}
	if result.AnalysisMode != "" {
		intentUpdates["intent_analysis_mode"] = string(result.AnalysisMode)
	}
	if result.Source != "" {
		intentUpdates["intent_source"] = string(result.Source)
	}
	if result.Model != nil {
		if result.Model.Path != "" {
			intentUpdates["model_path"] = result.Model.Path
		}
		if result.Model.Family != "" {
			intentUpdates["model_family"] = result.Model.Family
		}
		if result.Model.Scale != "" {
			intentUpdates["model_scale"] = result.Model.Scale
		}
		if result.Model.Variant != "" {
			intentUpdates["model_variant"] = result.Model.Variant
		}
	}
	if result.FrameworkStack != nil {
		stackJSON, _ := json.Marshal(result.FrameworkStack)
		intentUpdates["runtime_framework"] = string(stackJSON)
	}

	// Store full detail JSON
	detailJSON, _ := json.Marshal(result)
	intentUpdates["intent_detail"] = string(detailJSON)

	// Field sources
	if result.FieldSources != nil {
		sourcesJSON, _ := json.Marshal(result.FieldSources)
		intentUpdates["intent_field_sources"] = string(sourcesJSON)
	}

	// Matched rules
	if len(result.MatchedRules) > 0 {
		rulesJSON, _ := json.Marshal(result.MatchedRules)
		intentUpdates["intent_matched_rules"] = string(rulesJSON)
	}

	// Reasoning
	if result.Reasoning != "" {
		intentUpdates["intent_reasoning"] = result.Reasoning
	}

	if err := p.detectionFacade.UpdateIntentResult(ctx, workloadUID, intentUpdates); err != nil {
		log.Warnf("Failed to persist intent result for workload %s: %v", workloadUID, err)
		return constant.PipelineStateEvaluating, err
	}

	log.Infof("Intent result persisted for workload %s: category=%s confidence=%.2f state=%s",
		workloadUID, result.Category, result.Confidence, intentState)

	return pipelineNextState, nil
}

// gatherEvidence collects all available evidence for a workload from multiple sources.
// For terminated workloads, it skips collectors that require a running pod to avoid
// network hangs (code snapshot from container, image registry hybrid wait).
func (p *WorkloadAnalysisPipeline) gatherEvidence(
	ctx context.Context,
	workloadUID string,
) (*intent.IntentEvidence, error) {
	terminated := p.isWorkloadTerminated(ctx, workloadUID)

	// Start with spec-level evidence (always available, no running pod needed)
	evidence, err := p.specCollector.Collect(ctx, workloadUID)
	if err != nil {
		log.Warnf("SpecCollector failed for workload %s, continuing with empty evidence: %v", workloadUID, err)
		evidence = &intent.IntentEvidence{}
	}

	// Enrich with process probe evidence (reads from DB, does not contact pod directly)
	p.processCollector.Enrich(ctx, workloadUID, evidence)

	// Collect code snapshot: for terminated workloads, skip the live container
	// collection (which would hang trying to reach a dead pod) and go straight
	// to the DB cache fallback.
	if p.codeSnapshotCollector != nil && !terminated {
		snapEvidence, snapErr := p.codeSnapshotCollector.Collect(ctx, workloadUID, evidence.Command)
		if snapErr != nil {
			log.Debugf("CodeSnapshotCollector failed for %s (will try DB fallback): %v", workloadUID, snapErr)
		}
		if snapEvidence != nil {
			evidence.CodeSnapshot = snapEvidence
		}
	} else if terminated {
		log.Debugf("Skipping live code snapshot for terminated workload %s", workloadUID)
	}

	// Fallback: read previously stored snapshot from DB if collector did not populate it.
	// When the record has an external storage key, load file contents from the store.
	if evidence.CodeSnapshot == nil {
		dbSnap, dbErr := p.snapshotFacade.GetByWorkloadUID(ctx, workloadUID)
		if dbErr == nil && dbSnap != nil {
			evidence.CodeSnapshot = loadSnapshotEvidence(ctx, dbSnap, p.snapshotStore)
		}
	}

	// Enrich with image registry metadata.
	// For terminated workloads, only check the cache (no hybrid wait / polling)
	// to avoid a 30s timeout for images that were never analyzed.
	if terminated {
		p.enrichImageFromCacheOnly(ctx, evidence)
	} else {
		p.imageRegCollector.Enrich(ctx, evidence)
	}

	return evidence, nil
}

// enrichImageFromCacheOnly checks the image registry cache without triggering
// a new analysis request or waiting. This avoids the 30s hybrid wait timeout
// for terminated workloads whose image analysis may never complete.
func (p *WorkloadAnalysisPipeline) enrichImageFromCacheOnly(
	ctx context.Context,
	evidence *intent.IntentEvidence,
) {
	if evidence.Image == "" {
		return
	}
	regHost, repo, tag := parseImageRef(evidence.Image)
	cached, err := p.imageRegCollector.cacheFacade.GetByTagRef(ctx, regHost, repo, tag)
	if err == nil && cached != nil && cached.Status == "completed" {
		log.Debugf("enrichImageFromCacheOnly: cache hit for %s (completed)", evidence.Image)
		p.imageRegCollector.applyCache(cached, evidence)
	} else {
		log.Debugf("enrichImageFromCacheOnly: no cache hit for terminated workload image %s", evidence.Image)
	}
}

// planCollectors generates collection plans for the current workload
func (p *WorkloadAnalysisPipeline) planCollectors(
	ctx context.Context,
	workloadUID string,
	task *model.WorkloadTaskState,
) []*CollectorPlan {
	var plans []*CollectorPlan

	// Always collect process probe (highest priority)
	processCov, _ := p.coverageFacade.GetCoverage(ctx, workloadUID, constant.DetectionSourceProcess)
	if p.shouldCollect(processCov) {
		plans = append(plans, &CollectorPlan{
			TaskType: constant.TaskTypeProcessProbe,
			Source:   constant.DetectionSourceProcess,
			Priority: 100,
			Params:   map[string]interface{}{},
		})
	}

	// Image probe
	imageCov, _ := p.coverageFacade.GetCoverage(ctx, workloadUID, constant.DetectionSourceImage)
	if p.shouldCollect(imageCov) {
		plans = append(plans, &CollectorPlan{
			TaskType: constant.TaskTypeImageProbe,
			Source:   constant.DetectionSourceImage,
			Priority: 80,
			Params:   map[string]interface{}{},
		})
	}

	// Label probe
	labelCov, _ := p.coverageFacade.GetCoverage(ctx, workloadUID, constant.DetectionSourceLabel)
	if p.shouldCollect(labelCov) {
		plans = append(plans, &CollectorPlan{
			TaskType: constant.TaskTypeLabelProbe,
			Source:   constant.DetectionSourceLabel,
			Priority: 60,
			Params:   map[string]interface{}{},
		})
	}

	return plans
}

// shouldCollect checks if a coverage record indicates collection is needed
func (p *WorkloadAnalysisPipeline) shouldCollect(cov *model.DetectionCoverage) bool {
	if cov == nil {
		return true
	}
	return cov.Status == constant.DetectionStatusPending ||
		cov.Status == constant.DetectionStatusFailed
}

// scheduleSubTask creates a sub-task via the task facade
func (p *WorkloadAnalysisPipeline) scheduleSubTask(
	ctx context.Context,
	workloadUID string,
	plan *CollectorPlan,
) error {
	ext := plan.Params
	if ext == nil {
		ext = map[string]interface{}{}
	}
	ext["priority"] = plan.Priority
	subTask := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    plan.TaskType,
		Status:      constant.TaskStatusPending,
		Ext:         model.ExtType(ext),
	}
	return p.taskFacade.UpsertTask(ctx, subTask)
}

// areSubTasksComplete checks if all detection sub-tasks for a workload are done
func (p *WorkloadAnalysisPipeline) areSubTasksComplete(ctx context.Context, workloadUID string) (bool, error) {
	subTaskTypes := []string{
		constant.TaskTypeProcessProbe,
		constant.TaskTypeImageProbe,
		constant.TaskTypeLabelProbe,
	}

	for _, taskType := range subTaskTypes {
		t, err := p.taskFacade.GetTask(ctx, workloadUID, taskType)
		if err != nil {
			// Task not found means it was never created, consider it complete
			continue
		}
		if t != nil && (t.Status == constant.TaskStatusPending || t.Status == constant.TaskStatusRunning) {
			return false, nil
		}
	}

	return true, nil
}

// isWorkloadTerminated checks if a workload is no longer running.
// It checks the actual gpu_workload status (the source of truth), not just the detection record.
func (p *WorkloadAnalysisPipeline) isWorkloadTerminated(ctx context.Context, workloadUID string) bool {
	workloadFacade := database.GetFacade().GetWorkload()
	workload, err := workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil || workload == nil {
		return true // Assume terminated if can't find
	}

	// Check all terminal states including SaFE "Done" and "Deleted"
	terminatedStatuses := map[string]bool{
		"Completed": true,
		"Failed":    true,
		"Succeeded": true,
		"Stopped":   true,
		"Done":      true,
		"Deleted":   true,
	}

	isDeleted := workload.DeletedAt.Valid

	return isDeleted || terminatedStatuses[string(workload.Status)]
}

// getConfidenceGate returns the minimum confidence to skip LLM analysis
func (p *WorkloadAnalysisPipeline) getConfidenceGate(task *model.WorkloadTaskState) float64 {
	if task.Ext != nil {
		if val, ok := task.Ext["confidence_gate"]; ok {
			if f, ok := val.(float64); ok {
				return f
			}
		}
	}
	return DefaultConfidenceGate
}

// mergeResults combines deterministic and LLM analysis results.
// LLM results take priority for fields where the LLM provides higher confidence.
func (p *WorkloadAnalysisPipeline) mergeResults(
	deterministic *intent.IntentResult,
	llm *intent.IntentResult,
) *intent.IntentResult {
	if deterministic == nil && llm == nil {
		return &intent.IntentResult{}
	}
	if deterministic == nil {
		return llm
	}
	if llm == nil {
		return deterministic
	}

	merged := *deterministic

	// LLM overrides if it has higher confidence on category
	if llm.Confidence > deterministic.Confidence {
		merged.Category = llm.Category
		merged.ExpectedBehavior = llm.ExpectedBehavior
		merged.Confidence = llm.Confidence
		merged.Source = intent.IntentSourceLLM
		merged.AnalysisMode = intent.AnalysisModeCodeAnalyzed
	}

	// Always take LLM's enrichment fields if present
	if llm.Model != nil && (merged.Model == nil || merged.Model.Family == "") {
		merged.Model = llm.Model
	}
	if llm.Training != nil && merged.Training == nil {
		merged.Training = llm.Training
	}
	if llm.Inference != nil && merged.Inference == nil {
		merged.Inference = llm.Inference
	}
	if llm.FrameworkStack != nil {
		merged.FrameworkStack = llm.FrameworkStack
	}
	if llm.Reasoning != "" {
		merged.Reasoning = llm.Reasoning
	}

	// Merge field sources
	if merged.FieldSources == nil {
		merged.FieldSources = make(map[string]string)
	}
	for k, v := range llm.FieldSources {
		merged.FieldSources[k] = v
	}

	return &merged
}

// sendConductorRequest publishes an intent analysis task to ai-gateway.
// The task will be claimed by a Conductor agent via the pull model.
func (p *WorkloadAnalysisPipeline) sendConductorRequest(
	ctx context.Context,
	workloadUID string,
	task *model.WorkloadTaskState,
) error {
	if p.gwClient == nil {
		return fmt.Errorf("ai-gateway client not configured")
	}

	// Build the payload for Conductor: evidence + workload context
	evidence, err := p.gatherEvidence(ctx, workloadUID)
	if err != nil {
		return fmt.Errorf("gather evidence for LLM request: %w", err)
	}

	payload := map[string]interface{}{
		"workload_uid": workloadUID,
		"evidence":     evidence,
		"instance_id":  p.instanceID,
	}

	// Include deterministic eval result if available
	if evalMap := p.GetExtMap(task, "eval_result"); evalMap != nil {
		payload["deterministic_result"] = evalMap
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal LLM payload: %w", err)
	}

	resp, err := p.gwClient.Publish(ctx, &aigateway.PublishRequest{
		Topic:      "intent.analyze",
		Payload:    payloadJSON,
		Priority:   10,
		TimeoutSec: int(DefaultLLMTimeout.Seconds()),
	})
	if err != nil {
		return fmt.Errorf("publish to ai-gateway: %w", err)
	}

	log.Infof("Published intent analysis task %s for workload %s via ai-gateway", resp.ID, workloadUID)

	// Store the gateway task ID so we can poll for it later
	task.Ext["gw_task_id"] = resp.ID
	return nil
}

// pollConductorResponse polls ai-gateway for the result of a previously published task.
func (p *WorkloadAnalysisPipeline) pollConductorResponse(
	ctx context.Context,
	workloadUID string,
) (*intent.IntentResult, error) {
	if p.gwClient == nil {
		return nil, fmt.Errorf("ai-gateway client not configured")
	}

	// Retrieve the gateway task ID from the previous send
	// The caller (handleRequestingLLM) stores it in task.Ext, but we only
	// receive the workloadUID here. We'll look it up from the task.
	return nil, nil
}

// pollGatewayTaskResult polls ai-gateway for task result using the stored gw_task_id.
func (p *WorkloadAnalysisPipeline) pollGatewayTaskResult(
	ctx context.Context,
	task *model.WorkloadTaskState,
) (*intent.IntentResult, error) {
	if p.gwClient == nil {
		return nil, fmt.Errorf("ai-gateway client not configured")
	}

	gwTaskID, ok := task.Ext["gw_task_id"].(string)
	if !ok || gwTaskID == "" {
		return nil, fmt.Errorf("no gw_task_id stored for workload %s", task.WorkloadUID)
	}

	result, err := p.gwClient.GetResult(ctx, gwTaskID)
	if err != nil {
		return nil, fmt.Errorf("poll ai-gateway task %s: %w", gwTaskID, err)
	}
	if result == nil {
		// Still processing
		return nil, nil
	}

	// Parse the Conductor result into IntentResult
	var intentResult intent.IntentResult
	if err := json.Unmarshal(result.Result, &intentResult); err != nil {
		return nil, fmt.Errorf("unmarshal Conductor result: %w", err)
	}

	return &intentResult, nil
}

// createFollowUpTasks creates tasks based on the confirmed intent
func (p *WorkloadAnalysisPipeline) createFollowUpTasks(
	ctx context.Context,
	workloadUID string,
	det *model.WorkloadDetection,
	updates map[string]interface{},
) {
	category := ""
	if det.Category != nil {
		category = *det.Category
	}

	switch intent.Category(category) {
	case intent.CategoryPreTraining, intent.CategoryFineTuning:
		// Create metadata collection task
		metaTask := &model.WorkloadTaskState{
			WorkloadUID: workloadUID,
			TaskType:    constant.TaskTypeMetadataCollection,
			Status:      constant.TaskStatusPending,
			Ext:         model.ExtType{"trigger": "intent_pipeline", "priority": 50},
		}
		if err := p.taskFacade.UpsertTask(ctx, metaTask); err != nil {
			log.Warnf("Failed to create metadata collection task: %v", err)
		} else {
			updates["metadata_task_created"] = true
		}

		// Create log analysis task for training workloads to detect unmatched metric lines
		logTask := &model.WorkloadTaskState{
			WorkloadUID: workloadUID,
			TaskType:    constant.TaskTypeLogAnalysis,
			Status:      constant.TaskStatusPending,
			Ext:         model.ExtType{"trigger": "intent_pipeline", "auto_restart": true},
		}
		if err := p.taskFacade.UpsertTask(ctx, logTask); err != nil {
			log.Warnf("Failed to create log analysis task: %v", err)
		} else {
			updates["log_analysis_task_created"] = true
		}
	}
}

// loadSnapshotEvidence converts a DB record to CodeSnapshotEvidence,
// loading file contents from the external store when available.
func loadSnapshotEvidence(ctx context.Context, record *model.WorkloadCodeSnapshot, store snapshot.Store) *intent.CodeSnapshotEvidence {
	evidence := &intent.CodeSnapshotEvidence{
		PipFreeze:   record.PipFreeze,
		Fingerprint: record.Fingerprint,
	}

	// External store path
	if record.StorageKey != nil && *record.StorageKey != "" && store != nil {
		files, err := store.Load(ctx, *record.StorageKey)
		if err == nil && len(files) > 0 {
			fileIndex := make(map[string]string, len(files))
			for _, f := range files {
				fileIndex[f.RelPath] = string(f.Content)
			}

			if len(record.EntryScript) > 0 {
				fc := &intent.FileContent{}
				if p, ok := record.EntryScript["path"].(string); ok {
					fc.Path = p
				}
				if h, ok := record.EntryScript["hash"].(string); ok {
					fc.Hash = h
				}
				for relPath, content := range fileIndex {
					if len(relPath) > 6 && relPath[:6] == "entry/" {
						fc.Content = content
						break
					}
				}
				evidence.EntryScript = fc
			}
			if content, found := fileIndex["meta/pip_freeze.txt"]; found && evidence.PipFreeze == "" {
				evidence.PipFreeze = content
			}
			return evidence
		}
		log.Warnf("loadSnapshotEvidence: failed to load from store key=%s: %v, trying inline", *record.StorageKey, err)
	}

	// Inline JSONB path (legacy or fallback)
	if len(record.EntryScript) > 0 {
		fc := &intent.FileContent{}
		if p, ok := record.EntryScript["path"].(string); ok {
			fc.Path = p
		}
		if c, ok := record.EntryScript["content"].(string); ok {
			fc.Content = c
		}
		if h, ok := record.EntryScript["hash"].(string); ok {
			fc.Hash = h
		}
		evidence.EntryScript = fc
	}

	return evidence
}
