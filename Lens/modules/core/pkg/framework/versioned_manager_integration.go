package framework

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"gorm.io/gorm"
)

// MultiDimensionalDetectionManagerWithVersioning extends the manager with versioned storage
type MultiDimensionalDetectionManagerWithVersioning struct {
	*MultiDimensionalDetectionManager
	versionedStorage *VersionedDetectionStorage
	db               *gorm.DB
}

// NewMultiDimensionalDetectionManagerWithVersioning creates manager with version support
func NewMultiDimensionalDetectionManagerWithVersioning(
	db *gorm.DB,
	config *DetectionConfig,
) *MultiDimensionalDetectionManagerWithVersioning {
	if config == nil {
		config = DefaultDetectionConfig()
	}
	
	return &MultiDimensionalDetectionManagerWithVersioning{
		MultiDimensionalDetectionManager: NewMultiDimensionalDetectionManager(config),
		versionedStorage:                  NewVersionedDetectionStorage(db),
		db:                                db,
	}
}

// LoadDetection loads detection with automatic version handling
func (m *MultiDimensionalDetectionManagerWithVersioning) LoadDetection(
	ctx context.Context,
	workloadUID string,
) (*model.MultiDimensionalDetection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Load from database with version awareness
	detection, err := m.versionedStorage.LoadDetection(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to load versioned detection: %w", err)
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
	
	// Save to database as v2
	if err := m.versionedStorage.SaveDetection(ctx, detection); err != nil {
		return fmt.Errorf("failed to save versioned detection: %w", err)
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

// MigrateWorkload migrates a specific workload to v2
func (m *MultiDimensionalDetectionManagerWithVersioning) MigrateWorkload(
	ctx context.Context,
	workloadUID string,
) error {
	// Load (will auto-convert if v1)
	detection, err := m.LoadDetection(ctx, workloadUID)
	if err != nil {
		return fmt.Errorf("failed to load workload for migration: %w", err)
	}
	
	if detection == nil {
		return fmt.Errorf("workload not found: %s", workloadUID)
	}
	
	// Save as v2
	if err := m.versionedStorage.SaveDetection(ctx, detection); err != nil {
		return fmt.Errorf("failed to save migrated workload: %w", err)
	}
	
	log.Infof("Successfully migrated workload %s to v2", workloadUID)
	return nil
}

// MigrateAll migrates all v1 workloads to v2
func (m *MultiDimensionalDetectionManagerWithVersioning) MigrateAll(ctx context.Context) (int, error) {
	return m.versionedStorage.MigrateAllV1ToV2(ctx)
}

// GetVersionStats returns version distribution
func (m *MultiDimensionalDetectionManagerWithVersioning) GetVersionStats(ctx context.Context) (map[string]int, error) {
	return m.versionedStorage.GetVersionStats(ctx)
}

// EnsureSchema ensures the database schema exists
func (m *MultiDimensionalDetectionManagerWithVersioning) EnsureSchema(ctx context.Context) error {
	// Auto-migrate the table
	if err := m.db.WithContext(ctx).AutoMigrate(&DetectionRecord{}); err != nil {
		return fmt.Errorf("failed to migrate schema: %w", err)
	}
	
	log.Info("Detection schema ensured")
	return nil
}

