// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package hyperparameters

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Storage handles hyperparameter storage in database
type Storage struct {
	workloadFacade database.WorkloadFacadeInterface
}

// NewStorage creates a new hyperparameter storage
func NewStorage() *Storage {
	return &Storage{
		workloadFacade: database.NewWorkloadFacade(),
	}
}

// Save saves hyperparameters to workload annotations
func (s *Storage) Save(ctx context.Context, hparams *HyperparametersMetadata) error {
	// Get workload
	workload, err := s.workloadFacade.GetGpuWorkloadByUid(ctx, hparams.WorkloadUID)
	if err != nil {
		return fmt.Errorf("failed to get workload: %w", err)
	}

	if workload == nil {
		return fmt.Errorf("workload not found: %s", hparams.WorkloadUID)
	}

	// Initialize annotations if nil
	if workload.Annotations == nil {
		workload.Annotations = make(model.ExtType)
	}

	// Store hyperparameters in annotations with key "hyperparameters"
	workload.Annotations["hyperparameters"] = hparams.ToExtType()

	// Update workload
	if err := s.workloadFacade.UpdateGpuWorkload(ctx, workload); err != nil {
		return fmt.Errorf("failed to update workload: %w", err)
	}

	log.Infof("Saved hyperparameters to workload %s annotations (version %d)",
		hparams.WorkloadUID, hparams.Version)

	return nil
}

// Get retrieves hyperparameters from workload annotations
func (s *Storage) Get(ctx context.Context, workloadUID string) (*HyperparametersMetadata, error) {
	// Get workload
	workload, err := s.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}

	if workload == nil {
		return nil, fmt.Errorf("workload not found: %s", workloadUID)
	}

	// Check if hyperparameters exist in annotations
	if workload.Annotations == nil {
		return nil, fmt.Errorf("no annotations found for workload %s", workloadUID)
	}

	hparamsData, ok := workload.Annotations["hyperparameters"]
	if !ok {
		return nil, fmt.Errorf("hyperparameters not found in workload %s annotations", workloadUID)
	}

	// Convert to ExtType for parsing
	var hparamsMap map[string]interface{}
	switch v := hparamsData.(type) {
	case map[string]interface{}:
		hparamsMap = v
	default:
		return nil, fmt.Errorf("invalid hyperparameters format in annotations")
	}

	hparams, err := FromExtType(hparamsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hyperparameters: %w", err)
	}

	return hparams, nil
}

// List lists all workloads with hyperparameters in annotations
func (s *Storage) List(ctx context.Context, opts ListOptions) ([]*HyperparametersMetadata, error) {
	// Get all workloads that are not ended
	workloads, err := s.workloadFacade.GetWorkloadNotEnd(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	var result []*HyperparametersMetadata
	for _, workload := range workloads {
		// Check if workload has hyperparameters in annotations
		if workload.Annotations == nil {
			continue
		}

		hparamsData, ok := workload.Annotations["hyperparameters"]
		if !ok {
			continue
		}

		// Convert to ExtType for parsing
		var hparamsMap map[string]interface{}
		switch v := hparamsData.(type) {
		case map[string]interface{}:
			hparamsMap = v
		default:
			log.Warnf("Invalid hyperparameters format for workload %s", workload.UID)
			continue
		}

		hparams, err := FromExtType(hparamsMap)
		if err != nil {
			log.Warnf("Failed to parse hyperparameters for workload %s: %v", workload.UID, err)
			continue
		}

		// Apply filters
		if opts.Framework != "" && hparams.Summary.Framework != opts.Framework {
			continue
		}

		result = append(result, hparams)

		// Apply limit if specified
		if opts.Limit > 0 && len(result) >= opts.Limit {
			break
		}
	}

	// Apply offset if specified
	if opts.Offset > 0 {
		if opts.Offset >= len(result) {
			return []*HyperparametersMetadata{}, nil
		}
		result = result[opts.Offset:]
	}

	return result, nil
}

// Delete deletes hyperparameters from workload annotations
func (s *Storage) Delete(ctx context.Context, workloadUID string) error {
	// Get workload
	workload, err := s.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil {
		return fmt.Errorf("failed to get workload: %w", err)
	}

	if workload == nil {
		return fmt.Errorf("workload not found: %s", workloadUID)
	}

	// Remove hyperparameters from annotations
	if workload.Annotations != nil {
		delete(workload.Annotations, "hyperparameters")

		// Update workload
		if err := s.workloadFacade.UpdateGpuWorkload(ctx, workload); err != nil {
			return fmt.Errorf("failed to update workload: %w", err)
		}
	}

	log.Infof("Deleted hyperparameters from workload %s annotations", workloadUID)
	return nil
}

// AddSource adds a new source to existing hyperparameters
func (s *Storage) AddSource(
	ctx context.Context,
	workloadUID string,
	source HyperparameterSource,
) error {
	// Get existing hyperparameters
	hparams, err := s.Get(ctx, workloadUID)
	if err != nil {
		// If not found, create new
		hparams = &HyperparametersMetadata{
			WorkloadUID: workloadUID,
		}
	}

	// Add new source
	hparams.AddSource(source)

	// Re-merge sources
	hparams.MergeSources()

	// Update summary
	hparams.UpdateSummary()

	// Save
	return s.Save(ctx, hparams)
}

// Compare compares hyperparameters between two workloads
func (s *Storage) Compare(
	ctx context.Context,
	workloadUID1, workloadUID2 string,
) (map[string]interface{}, error) {
	hparams1, err := s.Get(ctx, workloadUID1)
	if err != nil {
		return nil, fmt.Errorf("failed to get hyperparameters for %s: %w", workloadUID1, err)
	}

	hparams2, err := s.Get(ctx, workloadUID2)
	if err != nil {
		return nil, fmt.Errorf("failed to get hyperparameters for %s: %w", workloadUID2, err)
	}

	diff := hparams1.Diff(hparams2)

	return map[string]interface{}{
		"workload_uid_1": workloadUID1,
		"workload_uid_2": workloadUID2,
		"differences":    diff,
		"diff_count":     len(diff),
	}, nil
}

// ListOptions specifies options for listing hyperparameters
type ListOptions struct {
	Framework string
	Limit     int
	Offset    int
}
