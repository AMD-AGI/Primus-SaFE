// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// FacadeStorage implements Storage interface using database facade
type FacadeStorage struct {
	facade database.AiWorkloadMetadataFacadeInterface
}

// NewFacadeStorage creates a new facade storage
func NewFacadeStorage(facade database.AiWorkloadMetadataFacadeInterface) *FacadeStorage {
	return &FacadeStorage{facade: facade}
}

// Store stores workload metadata to database
func (s *FacadeStorage) Store(ctx context.Context, metadata *WorkloadMetadata) error {
	// Convert metadata to map for ExtType
	var metadataMap map[string]interface{}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := json.Unmarshal(metadataJSON, &metadataMap); err != nil {
		return fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	// Determine primary framework
	framework := metadata.BaseFramework
	if framework == "" && len(metadata.Frameworks) > 0 {
		framework = metadata.Frameworks[0]
	}

	// Create record
	record := &model.AiWorkloadMetadata{
		WorkloadUID: metadata.WorkloadUID,
		Type:        "training",
		Framework:   framework,
		Metadata:    model.ExtType(metadataMap),
		ImagePrefix: "",
		CreatedAt:   metadata.CollectedAt,
	}

	// Check if exists
	existing, err := s.facade.GetAiWorkloadMetadata(ctx, metadata.WorkloadUID)
	if err != nil {
		return fmt.Errorf("failed to check existing metadata: %w", err)
	}

	if existing != nil {
		// Update
		record.ID = existing.ID
		if err := s.facade.UpdateAiWorkloadMetadata(ctx, record); err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}
	} else {
		// Create
		if err := s.facade.CreateAiWorkloadMetadata(ctx, record); err != nil {
			return fmt.Errorf("failed to create metadata: %w", err)
		}
	}

	log.Infof("Stored metadata for workload %s (framework: %s)",
		metadata.WorkloadUID, framework)

	return nil
}

// Get retrieves workload metadata from database
func (s *FacadeStorage) Get(ctx context.Context, workloadUID string) (*WorkloadMetadata, error) {
	record, err := s.facade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	if record == nil {
		return nil, nil
	}

	// Parse metadata JSON
	var metadata WorkloadMetadata
	metadataJSON, err := json.Marshal(record.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

// Query queries workload metadata with filters
func (s *FacadeStorage) Query(ctx context.Context, query *MetadataQuery) ([]*WorkloadMetadata, error) {
	// For now, implement simple query using List
	// A more efficient implementation would require adding query methods to the facade

	if query.WorkloadUID != "" {
		// Single workload query
		metadata, err := s.Get(ctx, query.WorkloadUID)
		if err != nil {
			return nil, err
		}
		if metadata == nil {
			return []*WorkloadMetadata{}, nil
		}
		return []*WorkloadMetadata{metadata}, nil
	}

	// For other queries, we'd need to extend the facade interface
	// For now, return empty result
	log.Warn("Query with filters not fully implemented in facade storage")
	return []*WorkloadMetadata{}, nil
}

// Delete deletes workload metadata from database
func (s *FacadeStorage) Delete(ctx context.Context, workloadUID string) error {
	if err := s.facade.DeleteAiWorkloadMetadata(ctx, workloadUID); err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	log.Infof("Deleted metadata for workload %s", workloadUID)
	return nil
}
