// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/pipeline"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/registry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// AssembleSubmitExecutor is the T5 executor that builds a WorkloadJSON payload
// from all collected evidence and writes it to the workload_detection table
// with intent_state='pending'.
type AssembleSubmitExecutor struct {
	specCollector       *pipeline.SpecCollector
	processCollector    *pipeline.ProcessEvidenceCollector
	imageRegCollector   *pipeline.ImageRegistryCollector
	detectionFacade     database.WorkloadDetectionFacadeInterface
	imageAnalyzer       *registry.InlineImageAnalyzer
}

// NewAssembleSubmitExecutor creates a T5 executor.
func NewAssembleSubmitExecutor() *AssembleSubmitExecutor {
	return &AssembleSubmitExecutor{
		specCollector:     pipeline.NewSpecCollector(),
		processCollector:  pipeline.NewProcessEvidenceCollector(),
		imageRegCollector: pipeline.NewImageRegistryCollector(),
		detectionFacade:   database.NewWorkloadDetectionFacade(),
		imageAnalyzer:     registry.NewInlineImageAnalyzer(),
	}
}

// Execute assembles all evidence into a WorkloadJSON and submits it for
// intent classification.
func (e *AssembleSubmitExecutor) Execute(ctx context.Context, master *MasterTask, sub *SubTask) error {
	evidence, err := e.specCollector.Collect(ctx, master.WorkloadUID)
	if err != nil {
		return fmt.Errorf("spec collection failed: %w", err)
	}

	e.processCollector.Enrich(ctx, master.WorkloadUID, evidence)
	e.imageRegCollector.Enrich(ctx, evidence)

	workloadJSON, err := json.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("failed to marshal workload JSON: %w", err)
	}

	updates := map[string]interface{}{
		"intent_workload_json": string(workloadJSON),
		"intent_state":         "pending",
	}

	if err := e.detectionFacade.UpdateIntentResult(ctx, master.WorkloadUID, updates); err != nil {
		return fmt.Errorf("failed to update intent result for %s: %w", master.WorkloadUID, err)
	}

	sub.Result = map[string]interface{}{
		"workload_json_size": len(workloadJSON),
		"submitted":          true,
	}

	log.Infof("AssembleSubmitExecutor: submitted workload JSON (%d bytes) for workload %s",
		len(workloadJSON), master.WorkloadUID)
	return nil
}
