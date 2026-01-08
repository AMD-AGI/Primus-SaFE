// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package framework

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"gorm.io/gorm"
)

// MultiDimensionalDetectionManagerWithVersioning extends the manager with persistence to ai_workload_metadata
type MultiDimensionalDetectionManagerWithVersioning struct {
	*MultiDimensionalDetectionManager
	v2Storage *V2DetectionStorage
	db        *gorm.DB
}

// NewMultiDimensionalDetectionManagerWithVersioning creates manager with V2 storage
func NewMultiDimensionalDetectionManagerWithVersioning(
	db *gorm.DB,
	config *DetectionConfig,
) *MultiDimensionalDetectionManagerWithVersioning {
	if config == nil {
		config = DefaultDetectionConfig()
	}

	return &MultiDimensionalDetectionManagerWithVersioning{
		MultiDimensionalDetectionManager: NewMultiDimensionalDetectionManager(config),
		v2Storage:                        NewV2DetectionStorage(),
		db:                               db,
	}
}

// LoadDetection loads detection with automatic version handling
func (m *MultiDimensionalDetectionManagerWithVersioning) LoadDetection(
	ctx context.Context,
	workloadUID string,
) (*model.MultiDimensionalDetection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Load from database (V2 only)
	detection, err := m.v2Storage.LoadDetection(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to load detection: %w", err)
	}

	if detection == nil {
		return nil, nil
	}

	// Cache in memory
	m.storage.Save(workloadUID, detection)

	return detection, nil
}

// SaveDetection saves detection (always as v2)
func (m *MultiDimensionalDetectionManagerWithVersioning) SaveDetection(
	ctx context.Context,
	workloadUID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get from memory
	detection := m.storage.Get(workloadUID)
	if detection == nil {
		return fmt.Errorf("no detection found in memory for workload %s", workloadUID)
	}

	// Save to database as V2
	if err := m.v2Storage.SaveDetection(ctx, detection); err != nil {
		return fmt.Errorf("failed to save detection: %w", err)
	}

	return nil
}

// ReportDimensionDetectionWithPersistence reports and persists immediately
func (m *MultiDimensionalDetectionManagerWithVersioning) ReportDimensionDetectionWithPersistence(
	ctx context.Context,
	workloadUID string,
	dimension model.DetectionDimension,
	value string,
	source string,
	confidence float64,
	evidence map[string]interface{},
) error {
	// First, report to in-memory manager
	if err := m.ReportDimensionDetection(ctx, workloadUID, dimension, value, source, confidence, evidence); err != nil {
		return err
	}

	// Then, persist to database
	if err := m.SaveDetection(ctx, workloadUID); err != nil {
		return fmt.Errorf("failed to persist detection: %w", err)
	}

	return nil
}

// GetOrLoadDetection gets from cache or loads from DB
func (m *MultiDimensionalDetectionManagerWithVersioning) GetOrLoadDetection(
	ctx context.Context,
	workloadUID string,
) (*model.MultiDimensionalDetection, error) {
	// Try memory first
	detection := m.GetDetection(workloadUID)
	if detection != nil {
		return detection, nil
	}

	// Load from database
	detection, err := m.LoadDetection(ctx, workloadUID)
	if err != nil {
		return nil, err
	}

	return detection, nil
}

// EnsureSchema is a no-op since we use existing ai_workload_metadata table
func (m *MultiDimensionalDetectionManagerWithVersioning) EnsureSchema(ctx context.Context) error {
	// No new table needed - using ai_workload_metadata.metadata.framework_detection
	log.Info("Using existing ai_workload_metadata table for detection storage")
	return nil
}
