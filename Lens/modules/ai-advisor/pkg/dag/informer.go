// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// WorkloadEventHandler scans for workloads that need intent analysis and
// ensures analysis_pipeline tasks exist for them so that handleEvaluating
// can assemble WorkloadJSON and dispatch to the Python intent-service.
type WorkloadEventHandler struct {
	detectionFacade database.WorkloadDetectionFacadeInterface
	taskFacade      database.WorkloadTaskFacadeInterface
	scheduler       *DAGScheduler
}

func NewWorkloadEventHandler(scheduler *DAGScheduler) *WorkloadEventHandler {
	return &WorkloadEventHandler{
		detectionFacade: database.NewWorkloadDetectionFacade(),
		taskFacade:      database.NewWorkloadTaskFacade(),
		scheduler:       scheduler,
	}
}

// ScanAndTrigger finds workloads that need intent analysis and creates
// analysis_pipeline tasks for them. This covers two cases:
//  1. Confirmed workloads with known framework (normal path)
//  2. Workloads stuck in detection with no framework identified after 5 minutes
//     (creates a detection record to bootstrap intent analysis with LLM exploration)
func (h *WorkloadEventHandler) ScanAndTrigger(ctx context.Context) {
	intentInformerScansTotal.Inc()

	// Case 1: workloads with detection records needing intent analysis
	detections, err := h.detectionFacade.ListNeedingIntentAnalysis(ctx, 50)
	if err != nil {
		log.Warnf("WorkloadEventHandler: ListNeedingIntentAnalysis failed: %v", err)
	} else {
		intentWorkloadsDiscovered.Add(float64(len(detections)))
		created := 0
		for _, d := range detections {
			if h.ensurePipelineTask(ctx, d.WorkloadUID) {
				created++
			}
		}
		if created > 0 {
			intentPipelineTasksCreated.Add(float64(created))
			log.Infof("WorkloadEventHandler: created %d pipeline tasks for detected workloads", created)
		}
	}

	// Case 2: workloads stuck in detection coordinator for >5 minutes
	// with no workload_detection record. Bootstrap a detection record
	// so they can enter the intent analysis pipeline.
	staleUIDs, err := h.detectionFacade.ListStaleUndetectedWorkloads(ctx, 20)
	if err != nil {
		log.Warnf("WorkloadEventHandler: ListStaleUndetectedWorkloads failed: %v", err)
		return
	}

	bootstrapped := 0
	for _, uid := range staleUIDs {
		if h.bootstrapUndetectedWorkload(ctx, uid) {
			bootstrapped++
		}
	}
	if bootstrapped > 0 {
		log.Infof("WorkloadEventHandler: bootstrapped %d undetected workloads into intent analysis", bootstrapped)
	}
}

// bootstrapUndetectedWorkload creates a minimal workload_detection record
// for a workload that the detection coordinator could not identify, so it
// can enter the intent analysis pipeline for LLM exploration.
func (h *WorkloadEventHandler) bootstrapUndetectedWorkload(ctx context.Context, workloadUID string) bool {
	existing, _ := h.detectionFacade.GetDetection(ctx, workloadUID)
	if existing != nil {
		return false
	}

	pending := "pending"
	det := &model.WorkloadDetection{
		WorkloadUID:    workloadUID,
		Status:         "unknown",
		DetectionState: "completed",
		IntentState:    &pending,
	}

	if err := h.detectionFacade.CreateDetection(ctx, det); err != nil {
		log.Warnf("WorkloadEventHandler: failed to bootstrap detection for %s: %v", workloadUID, err)
		return false
	}

	if h.ensurePipelineTask(ctx, workloadUID) {
		log.Infof("WorkloadEventHandler: bootstrapped undetected workload %s into intent pipeline", workloadUID)
		return true
	}
	return false
}

// ensurePipelineTask creates an analysis_pipeline task if one doesn't
// already exist in a running/pending state. Sets initial pipeline_state
// to evaluating so handleEvaluating runs directly.
func (h *WorkloadEventHandler) ensurePipelineTask(ctx context.Context, workloadUID string) bool {
	existing, err := h.taskFacade.GetTask(ctx, workloadUID, constant.TaskTypeAnalysisPipeline)
	if err == nil && existing != nil {
		if existing.Status == constant.TaskStatusRunning ||
			existing.Status == constant.TaskStatusPending {
			return false
		}
	}

	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    constant.TaskTypeAnalysisPipeline,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			"pipeline_state": constant.PipelineStateInit,
			"analysis_mode":  "full",
		},
	}

	if err := h.taskFacade.UpsertTask(ctx, task); err != nil {
		log.Warnf("WorkloadEventHandler: failed to create pipeline task for %s: %v", workloadUID, err)
		return false
	}

	log.Infof("WorkloadEventHandler: created analysis_pipeline task for %s (start at evaluating)",
		workloadUID)
	return true
}


