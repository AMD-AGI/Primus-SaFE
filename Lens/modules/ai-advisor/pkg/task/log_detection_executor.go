// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package task

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
)

// LogDetectionExecutor scans logs for framework detection patterns
type LogDetectionExecutor struct {
	coreTask.BaseExecutor

	evidenceStore  *detection.EvidenceStore
	coverageFacade database.DetectionCoverageFacadeInterface
}

// NewLogDetectionExecutor creates a new LogDetectionExecutor
func NewLogDetectionExecutor() *LogDetectionExecutor {
	return &LogDetectionExecutor{
		evidenceStore:  detection.NewEvidenceStore(),
		coverageFacade: database.NewDetectionCoverageFacade(),
	}
}

// NewLogDetectionExecutorWithDeps creates executor with custom dependencies
func NewLogDetectionExecutorWithDeps(
	evidenceStore *detection.EvidenceStore,
	coverageFacade database.DetectionCoverageFacadeInterface,
) *LogDetectionExecutor {
	return &LogDetectionExecutor{
		evidenceStore:  evidenceStore,
		coverageFacade: coverageFacade,
	}
}

// GetTaskType returns the task type
func (e *LogDetectionExecutor) GetTaskType() string {
	return constant.TaskTypeLogDetection
}

// Validate validates task parameters
func (e *LogDetectionExecutor) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Execute executes log detection
func (e *LogDetectionExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task
	workloadUID := task.WorkloadUID

	log.Infof("Starting log detection for workload %s", workloadUID)

	updates := map[string]interface{}{
		"started_at": time.Now().Format(time.RFC3339),
	}

	// Get time window from task parameters
	var fromTime, toTime time.Time
	if fromStr := e.GetExtString(task, "from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			fromTime = t
		}
	}
	if toStr := e.GetExtString(task, "to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			toTime = t
		}
	}

	// Get mode (realtime or backfill)
	mode := e.GetExtString(task, "mode")
	if mode == "" {
		mode = "backfill"
	}
	updates["mode"] = mode

	// Mark coverage as collecting
	if err := e.coverageFacade.MarkCollecting(ctx, workloadUID, constant.DetectionSourceLog); err != nil {
		log.Warnf("Failed to mark log coverage as collecting: %v", err)
	}

	// For backfill mode, we need to query logs from storage
	// For now, this is a placeholder - in production, this would query
	// log storage (e.g., Loki, Elasticsearch, or internal log store)
	if mode == "backfill" {
		updates["from"] = fromTime.Format(time.RFC3339)
		updates["to"] = toTime.Format(time.RFC3339)

		result, err := e.scanLogsForPatterns(ctx, workloadUID, fromTime, toTime)
		if err != nil {
			errMsg := fmt.Sprintf("failed to scan logs: %v", err)
			log.Warnf("Log detection failed for workload %s: %s", workloadUID, errMsg)
			e.coverageFacade.MarkFailed(ctx, workloadUID, constant.DetectionSourceLog, errMsg)
			updates["error"] = errMsg
			return coreTask.FailureResult(errMsg, updates), err
		}

		updates["logs_scanned"] = result.LogsScanned
		updates["matches_found"] = result.MatchesFound
		updates["evidence_count"] = result.EvidenceCount

		// Update covered time range
		if !fromTime.IsZero() && !toTime.IsZero() {
			if err := e.coverageFacade.UpdateCoveredTimeRange(ctx, workloadUID, fromTime, toTime); err != nil {
				log.Warnf("Failed to update covered time range: %v", err)
			}
		}
	} else {
		// Realtime mode - just update the coverage status
		// Actual realtime detection is handled by telemetry-processor
		updates["note"] = "realtime mode handled by telemetry-processor"
	}

	updates["completed_at"] = time.Now().Format(time.RFC3339)

	// Get evidence count from stored evidence
	evidenceCount := 0
	if ec, ok := updates["evidence_count"].(int); ok {
		evidenceCount = ec
	}

	// Mark coverage as collected
	if err := e.coverageFacade.MarkCollected(ctx, workloadUID, constant.DetectionSourceLog, int32(evidenceCount)); err != nil {
		log.Warnf("Failed to mark log coverage as collected: %v", err)
	}

	log.Infof("Log detection completed for workload %s", workloadUID)
	return coreTask.SuccessResult(updates), nil
}

// LogScanResult holds the result of log scanning
type LogScanResult struct {
	LogsScanned   int
	MatchesFound  int
	EvidenceCount int
	Frameworks    []string
}

// scanLogsForPatterns scans logs and applies pattern matching
func (e *LogDetectionExecutor) scanLogsForPatterns(
	ctx context.Context,
	workloadUID string,
	from, to time.Time,
) (*LogScanResult, error) {
	result := &LogScanResult{
		Frameworks: []string{},
	}

	// NOTE: This is a placeholder implementation
	// In production, this would:
	// 1. Query logs from log storage (Loki, ES, etc.) for the given time range
	// 2. Apply pattern matching to each log line
	// 3. Store detected frameworks as evidence

	// For now, we just mark the coverage as checked
	// The actual log detection happens in telemetry-processor via passive detection

	log.Debugf("Log scan for workload %s from %v to %v (placeholder)", workloadUID, from, to)

	// If we have a pattern matcher, we could use it here
	// Example patterns that would be matched:
	// - "Loading Megatron model"
	// - "Initializing DeepSpeed"
	// - "Primus training started"
	// - vLLM startup messages
	// - etc.

	result.LogsScanned = 0
	result.MatchesFound = 0
	result.EvidenceCount = 0

	return result, nil
}

// storeLogEvidence stores evidence from log detection
func (e *LogDetectionExecutor) storeLogEvidence(
	ctx context.Context,
	workloadUID string,
	framework string,
	confidence float64,
	matchedPattern string,
	logTimestamp time.Time,
) error {
	req := &detection.StoreEvidenceRequest{
		WorkloadUID:  workloadUID,
		Source:       constant.DetectionSourceLog,
		SourceType:   "passive",
		Framework:    framework,
		WorkloadType: "training",
		Confidence:   confidence,
		Evidence: map[string]interface{}{
			"pattern_matched": matchedPattern,
			"log_timestamp":   logTimestamp.Format(time.RFC3339),
			"method":          "log_pattern",
		},
	}

	return e.evidenceStore.StoreEvidence(ctx, req)
}

// Cancel cancels the task
func (e *LogDetectionExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("Log detection task cancelled for workload %s", task.WorkloadUID)
	return nil
}

