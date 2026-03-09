// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ProcessCollectionExecutor is the T4 executor that verifies process-level
// evidence has been collected for the workload.
type ProcessCollectionExecutor struct {
	evidenceFacade database.WorkloadDetectionEvidenceFacadeInterface
}

// NewProcessCollectionExecutor creates a T4 executor.
func NewProcessCollectionExecutor() *ProcessCollectionExecutor {
	return &ProcessCollectionExecutor{
		evidenceFacade: database.NewWorkloadDetectionEvidenceFacade(),
	}
}

// Execute checks that process evidence exists for the workload.
func (e *ProcessCollectionExecutor) Execute(ctx context.Context, master *MasterTask, sub *SubTask) error {
	evidenceList, err := e.evidenceFacade.ListEvidenceByWorkload(ctx, master.WorkloadUID)
	if err != nil {
		return fmt.Errorf("failed to list evidence for workload %s: %w", master.WorkloadUID, err)
	}

	if len(evidenceList) == 0 {
		return fmt.Errorf("no process evidence found for workload %s, will retry", master.WorkloadUID)
	}

	var sources []string
	for _, ev := range evidenceList {
		sources = append(sources, ev.Source)
	}

	sub.Result = map[string]interface{}{
		"evidence_count": len(evidenceList),
		"sources":        sources,
	}

	log.Debugf("ProcessCollectionExecutor: found %d evidence records for workload %s",
		len(evidenceList), master.WorkloadUID)
	return nil
}
