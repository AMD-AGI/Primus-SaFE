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

// ScanAndTrigger finds confirmed workloads that have no intent_workload_json
// yet and creates analysis_pipeline tasks so the TaskScheduler drives
// handleEvaluating to assemble WorkloadJSON.
func (h *WorkloadEventHandler) ScanAndTrigger(ctx context.Context) {
	detections, err := h.detectionFacade.ListNeedingIntentAnalysis(ctx, 50)
	if err != nil {
		log.Warnf("WorkloadEventHandler: query failed: %v", err)
		return
	}

	created := 0
	for _, d := range detections {
		if h.ensurePipelineTask(ctx, d.WorkloadUID) {
			created++
		}
	}

	if created > 0 {
		log.Infof("WorkloadEventHandler: created %d analysis_pipeline tasks", created)
	}
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
			"pipeline_state": constant.PipelineStateEvaluating,
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
