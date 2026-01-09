// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workflow

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/config"
)

// BaseWorkflow provides common functionality for workflows
type BaseWorkflow struct {
	name   string
	config *config.Config
	stages []Stage
}

// NewBaseWorkflow creates a new base workflow
func NewBaseWorkflow(name string, cfg *config.Config) *BaseWorkflow {
	return &BaseWorkflow{
		name:   name,
		config: cfg,
		stages: make([]Stage, 0),
	}
}

// Name returns the workflow name
func (w *BaseWorkflow) Name() string {
	return w.name
}

// Config returns the workflow configuration
func (w *BaseWorkflow) Config() *config.Config {
	return w.config
}

// AddStage adds a stage to the workflow
func (w *BaseWorkflow) AddStage(stage Stage) {
	w.stages = append(w.stages, stage)
}

// Stages returns all stages
func (w *BaseWorkflow) Stages() []Stage {
	return w.stages
}

// Install runs all stages in order
func (w *BaseWorkflow) Install(ctx context.Context, opts RunOptions) error {
	totalStages := len(w.stages)

	for i, stage := range w.stages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Printf("[%d/%d] Running stage: %s\n", i+1, totalStages, stage.Name())

		// Run the stage
		if err := stage.Run(ctx, opts); err != nil {
			return fmt.Errorf("stage %s failed: %w", stage.Name(), err)
		}

		// Verify the stage completed
		status, err := stage.Verify(ctx, opts)
		if err != nil {
			return fmt.Errorf("stage %s verification failed: %w", stage.Name(), err)
		}

		if status.State == StateFailed {
			return fmt.Errorf("stage %s failed: %s", stage.Name(), status.Message)
		}

		fmt.Printf("[%d/%d] Stage %s completed ✓\n\n", i+1, totalStages, stage.Name())
	}

	return nil
}

// Uninstall runs all stages in reverse order
func (w *BaseWorkflow) Uninstall(ctx context.Context, opts RunOptions, uninstallOpts UninstallOptions) error {
	totalStages := len(w.stages)

	// Run stages in reverse order
	for i := totalStages - 1; i >= 0; i-- {
		stage := w.stages[i]

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Printf("[%d/%d] Rolling back stage: %s\n", totalStages-i, totalStages, stage.Name())

		if err := stage.Rollback(ctx, opts); err != nil {
			if uninstallOpts.Force {
				fmt.Printf("Warning: stage %s rollback failed (continuing due to --force): %v\n", stage.Name(), err)
				continue
			}
			return fmt.Errorf("stage %s rollback failed: %w", stage.Name(), err)
		}

		fmt.Printf("[%d/%d] Stage %s rolled back ✓\n\n", totalStages-i, totalStages, stage.Name())
	}

	return nil
}

// Status returns the status of all stages
func (w *BaseWorkflow) Status(ctx context.Context, opts RunOptions) (*Status, error) {
	status := &Status{
		WorkflowName: w.name,
		OverallState: StateReady,
		Stages:       make([]StageStatus, 0, len(w.stages)),
	}

	for _, stage := range w.stages {
		stageStatus, err := stage.Verify(ctx, opts)
		if err != nil {
			stageStatus = &StageStatus{
				Name:    stage.Name(),
				State:   StateUnknown,
				Message: err.Error(),
			}
		}

		status.Stages = append(status.Stages, *stageStatus)

		// Update overall state
		if stageStatus.State == StateFailed {
			status.OverallState = StateFailed
		} else if stageStatus.State == StateInProgress && status.OverallState != StateFailed {
			status.OverallState = StateInProgress
		} else if stageStatus.State == StatePending && status.OverallState == StateReady {
			status.OverallState = StatePending
		}
	}

	return status, nil
}
