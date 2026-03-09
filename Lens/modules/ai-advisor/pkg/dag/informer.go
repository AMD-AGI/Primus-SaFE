// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// WorkloadEventHandler scans for workloads that need DAG-based intent analysis
// and triggers the scheduler accordingly.
type WorkloadEventHandler struct {
	detectionFacade database.WorkloadDetectionFacadeInterface
	scheduler       *DAGScheduler
}

// NewWorkloadEventHandler creates a handler that bridges detection records to
// the DAG scheduler.
func NewWorkloadEventHandler(scheduler *DAGScheduler) *WorkloadEventHandler {
	return &WorkloadEventHandler{
		detectionFacade: database.NewWorkloadDetectionFacade(),
		scheduler:       scheduler,
	}
}

// ScanAndTrigger queries workload_detection for rows that have a confirmed
// detection but no intent analysis started yet (intent_state is NULL or empty),
// and triggers OnWorkloadDetected for each.
func (h *WorkloadEventHandler) ScanAndTrigger(ctx context.Context) {
	detections, _, err := h.detectionFacade.ListByIntentState(ctx, "", 100, 0)
	if err != nil {
		log.Warnf("WorkloadEventHandler: failed to query detections needing intent analysis: %v", err)
		return
	}

	triggered := 0
	for _, d := range detections {
		if d.Status != "confirmed" && d.Status != "verified" {
			continue
		}

		needsAnalysis := d.IntentState == nil || *d.IntentState == "" || *d.IntentState == "pending"
		if !needsAnalysis {
			continue
		}

		h.scheduler.OnWorkloadDetected(ctx, d.WorkloadUID)
		triggered++
	}

	if triggered > 0 {
		log.Infof("WorkloadEventHandler: triggered DAG tasks for %d workloads", triggered)
	}
}
