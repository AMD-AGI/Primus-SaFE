// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// For Pod/Node/Facade mocks use core: database.NewMockFacade(), database.NewMockPodFacade(), database.NewMockNodeFacade().

// MockAiWorkloadMetadataFacade is a mock implementation of AiWorkloadMetadataFacadeInterface
type MockAiWorkloadMetadataFacade struct {
	Metadata  map[string]*model.AiWorkloadMetadata
	CreateErr error
	GetErr    error
	UpdateErr error
	DeleteErr error

	CreateCalls int
	GetCalls    int
	UpdateCalls int
	DeleteCalls int
}

// NewMockAiWorkloadMetadataFacade creates a new mock AI workload metadata facade
func NewMockAiWorkloadMetadataFacade() *MockAiWorkloadMetadataFacade {
	return &MockAiWorkloadMetadataFacade{
		Metadata: make(map[string]*model.AiWorkloadMetadata),
	}
}

// CreateAiWorkloadMetadata creates AI workload metadata
func (m *MockAiWorkloadMetadataFacade) CreateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	m.CreateCalls++

	if m.CreateErr != nil {
		return m.CreateErr
	}

	if _, exists := m.Metadata[metadata.WorkloadUID]; exists {
		return fmt.Errorf("metadata already exists for workload %s", metadata.WorkloadUID)
	}

	m.Metadata[metadata.WorkloadUID] = metadata
	return nil
}

// GetAiWorkloadMetadata retrieves AI workload metadata
func (m *MockAiWorkloadMetadataFacade) GetAiWorkloadMetadata(ctx context.Context, workloadUID string) (*model.AiWorkloadMetadata, error) {
	m.GetCalls++

	if m.GetErr != nil {
		return nil, m.GetErr
	}

	metadata, ok := m.Metadata[workloadUID]
	if !ok {
		return nil, nil
	}

	return metadata, nil
}

// UpdateAiWorkloadMetadata updates AI workload metadata
func (m *MockAiWorkloadMetadataFacade) UpdateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	m.UpdateCalls++

	if m.UpdateErr != nil {
		return m.UpdateErr
	}

	if _, exists := m.Metadata[metadata.WorkloadUID]; !exists {
		return fmt.Errorf("metadata not found for workload %s", metadata.WorkloadUID)
	}

	m.Metadata[metadata.WorkloadUID] = metadata
	return nil
}

// DeleteAiWorkloadMetadata deletes AI workload metadata
func (m *MockAiWorkloadMetadataFacade) DeleteAiWorkloadMetadata(ctx context.Context, workloadUID string) error {
	m.DeleteCalls++

	if m.DeleteErr != nil {
		return m.DeleteErr
	}

	delete(m.Metadata, workloadUID)
	return nil
}

// FindCandidateWorkloads finds candidate workloads for reuse
func (m *MockAiWorkloadMetadataFacade) FindCandidateWorkloads(ctx context.Context, imagePrefix string, timeWindow time.Time, minConfidence float64, limit int) ([]*model.AiWorkloadMetadata, error) {
	var candidates []*model.AiWorkloadMetadata

	for _, metadata := range m.Metadata {
		if metadata.ImagePrefix == imagePrefix {
			candidates = append(candidates, metadata)
		}
	}

	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return candidates, nil
}

// ListAiWorkloadMetadataByUIDs retrieves multiple metadata records by workload UIDs
func (m *MockAiWorkloadMetadataFacade) ListAiWorkloadMetadataByUIDs(ctx context.Context, workloadUIDs []string) ([]*model.AiWorkloadMetadata, error) {
	var results []*model.AiWorkloadMetadata

	for _, uid := range workloadUIDs {
		if metadata, ok := m.Metadata[uid]; ok {
			results = append(results, metadata)
		}
	}

	return results, nil
}

// WithCluster returns the facade itself (for testing)
func (m *MockAiWorkloadMetadataFacade) WithCluster(clusterName string) database.AiWorkloadMetadataFacadeInterface {
	return m
}
