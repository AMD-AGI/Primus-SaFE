// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package installer

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Executor manages the execution of installation stages with proper lifecycle.
// It handles prerequisite checking, idempotency, and wait logic.
type Executor struct {
	client     *ClusterClient
	helmClient *HelmClient
	config     *InstallConfig
}

// NewExecutor creates a new Executor
func NewExecutor(kubeconfig []byte, config *InstallConfig) (*Executor, error) {
	client, err := NewClusterClient(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster client: %w", err)
	}

	return &Executor{
		client:     client,
		helmClient: NewHelmClient(),
		config:     config,
	}, nil
}

// ExecuteStages executes a sequence of stages with proper lifecycle management.
// It returns the results for each stage and any error that stopped execution.
func (e *Executor) ExecuteStages(ctx context.Context, stages []StageV2) ([]StageResult, error) {
	var results []StageResult

	for _, stage := range stages {
		result := e.executeStage(ctx, stage)
		results = append(results, result)

		if result.Status == StageStatusFailed {
			return results, result.Error
		}
	}

	return results, nil
}

// executeStage handles the full lifecycle of a single stage
func (e *Executor) executeStage(ctx context.Context, stage StageV2) StageResult {
	startTime := time.Now()
	result := StageResult{Stage: stage.Name()}

	log.Infof("=== Starting stage: %s ===", stage.Name())

	// 1. Check prerequisites
	log.Infof("[%s] Checking prerequisites...", stage.Name())
	missing, err := stage.CheckPrerequisites(ctx, e.client, e.config)
	if err != nil {
		result.Status = StageStatusFailed
		result.Error = fmt.Errorf("prerequisites check failed: %w", err)
		result.Duration = time.Since(startTime)
		log.Errorf("[%s] Prerequisites check error: %v", stage.Name(), err)
		return result
	}
	if len(missing) > 0 {
		result.Status = StageStatusFailed
		result.Error = fmt.Errorf("missing prerequisites: %v", missing)
		result.Duration = time.Since(startTime)
		log.Errorf("[%s] Missing prerequisites: %v", stage.Name(), missing)
		return result
	}
	log.Infof("[%s] Prerequisites check passed", stage.Name())

	// 2. Check if should run (idempotency)
	log.Infof("[%s] Checking if stage should run...", stage.Name())
	shouldRun, reason, err := stage.ShouldRun(ctx, e.client, e.config)
	if err != nil {
		result.Status = StageStatusFailed
		result.Error = fmt.Errorf("should run check failed: %w", err)
		result.Duration = time.Since(startTime)
		log.Errorf("[%s] ShouldRun check error: %v", stage.Name(), err)
		return result
	}
	if !shouldRun {
		result.Status = StageStatusSkipped
		result.Reason = reason
		result.Duration = time.Since(startTime)
		log.Infof("[%s] Skipping: %s", stage.Name(), reason)
		return result
	}
	log.Infof("[%s] Will execute: %s", stage.Name(), reason)

	// 3. Execute
	log.Infof("[%s] Executing...", stage.Name())
	if err := stage.Execute(ctx, e.client, e.config); err != nil {
		result.Status = StageStatusFailed
		result.Error = fmt.Errorf("execution failed: %w", err)
		result.Duration = time.Since(startTime)
		log.Errorf("[%s] Execution failed: %v", stage.Name(), err)

		// Attempt rollback for non-required stages
		if !stage.IsRequired() {
			log.Infof("[%s] Attempting rollback...", stage.Name())
			if rbErr := stage.Rollback(ctx, e.client, e.config); rbErr != nil {
				log.Warnf("[%s] Rollback failed: %v", stage.Name(), rbErr)
			}
		}

		return result
	}
	log.Infof("[%s] Execution completed", stage.Name())

	// 4. Wait for ready
	timeout := GetStageTimeout(stage.Name())
	log.Infof("[%s] Waiting for ready (timeout: %v)...", stage.Name(), timeout)
	if err := stage.WaitForReady(ctx, e.client, e.config, timeout); err != nil {
		result.Status = StageStatusFailed
		result.Error = fmt.Errorf("wait for ready failed: %w", err)
		result.Duration = time.Since(startTime)
		log.Errorf("[%s] Wait for ready failed: %v", stage.Name(), err)

		// Attempt rollback for non-required stages
		if !stage.IsRequired() {
			log.Infof("[%s] Attempting rollback...", stage.Name())
			if rbErr := stage.Rollback(ctx, e.client, e.config); rbErr != nil {
				log.Warnf("[%s] Rollback failed: %v", stage.Name(), rbErr)
			}
		}

		return result
	}

	result.Status = StageStatusCompleted
	result.Duration = time.Since(startTime)
	log.Infof("[%s] Stage completed in %v", stage.Name(), result.Duration)
	return result
}

// ExecuteWithResume executes stages, starting from the specified stage.
// This is used for resuming from a failed installation.
func (e *Executor) ExecuteWithResume(ctx context.Context, stages []StageV2, startStage string) ([]StageResult, error) {
	var results []StageResult

	// Find the starting index
	startIndex := 0
	if startStage != "" {
		for i, stage := range stages {
			if stage.Name() == startStage {
				startIndex = i
				break
			}
		}
	}

	log.Infof("Starting from stage '%s' (index %d), total stages: %d", stages[startIndex].Name(), startIndex, len(stages))

	// Execute from the starting point
	for i := startIndex; i < len(stages); i++ {
		result := e.executeStage(ctx, stages[i])
		results = append(results, result)

		if result.Status == StageStatusFailed {
			return results, result.Error
		}
	}

	return results, nil
}

// GetClient returns the cluster client
func (e *Executor) GetClient() *ClusterClient {
	return e.client
}

// GetHelmClient returns the Helm client
func (e *Executor) GetHelmClient() *HelmClient {
	return e.helmClient
}

// GetConfig returns the install config
func (e *Executor) GetConfig() *InstallConfig {
	return e.config
}
