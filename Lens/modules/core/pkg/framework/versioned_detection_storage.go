// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"gorm.io/gorm"
)

const (
	// DetectionVersion constants
	DetectionVersionV1      = "1.0" // Legacy single-dimension detection
	DetectionVersionV2      = "2.0" // Multi-dimensional detection
	DetectionVersionCurrent = DetectionVersionV2
)

// VersionedDetectionStorage handles version-aware storage and loading
type VersionedDetectionStorage struct {
	db *gorm.DB
}

// NewVersionedDetectionStorage creates a new versioned storage
func NewVersionedDetectionStorage(db *gorm.DB) *VersionedDetectionStorage {
	return &VersionedDetectionStorage{
		db: db,
	}
}

// DetectionRecord represents the database record (supports both v1 and v2)
type DetectionRecord struct {
	WorkloadUID string    `gorm:"primaryKey;column:workload_uid"`
	Version     string    `gorm:"column:version;default:'1.0'"` // Default to v1 for old records
	Data        []byte    `gorm:"column:data;type:jsonb"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName specifies the table name
func (DetectionRecord) TableName() string {
	return "framework_detection_versioned"
}

// LoadDetection loads detection with automatic version handling
func (s *VersionedDetectionStorage) LoadDetection(
	ctx context.Context,
	workloadUID string,
) (*model.MultiDimensionalDetection, error) {
	var record DetectionRecord
	
	err := s.db.WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		First(&record).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to load detection record: %w", err)
	}
	
	// Version-aware deserialization
	switch record.Version {
	case DetectionVersionV2, "": // Empty version treated as v2 if data structure matches
		return s.loadV2Detection(&record)
	
	case DetectionVersionV1:
		return s.loadAndConvertV1Detection(&record)
	
	default:
		log.Warnf("Unknown detection version %s for workload %s, attempting v2 parse", 
			record.Version, workloadUID)
		return s.loadV2Detection(&record)
	}
}

// SaveDetection saves detection (always uses v2 format)
func (s *VersionedDetectionStorage) SaveDetection(
	ctx context.Context,
	detection *model.MultiDimensionalDetection,
) error {
	// Always save as v2
	detection.Version = DetectionVersionV2
	detection.UpdatedAt = time.Now()
	
	// Serialize to JSON
	data, err := json.Marshal(detection)
	if err != nil {
		return fmt.Errorf("failed to marshal detection: %w", err)
	}
	
	record := &DetectionRecord{
		WorkloadUID: detection.WorkloadUID,
		Version:     DetectionVersionV2,
		Data:        data,
		UpdatedAt:   time.Now(),
	}
	
	// Upsert
	err = s.db.WithContext(ctx).
		Save(record).Error
	
	if err != nil {
		return fmt.Errorf("failed to save detection: %w", err)
	}
	
	log.Debugf("Saved detection as v2 for workload %s", detection.WorkloadUID)
	return nil
}

// loadV2Detection loads v2 format directly
func (s *VersionedDetectionStorage) loadV2Detection(record *DetectionRecord) (*model.MultiDimensionalDetection, error) {
	var detection model.MultiDimensionalDetection
	
	if err := json.Unmarshal(record.Data, &detection); err != nil {
		return nil, fmt.Errorf("failed to unmarshal v2 detection: %w", err)
	}
	
	log.Debugf("Loaded v2 detection for workload %s", detection.WorkloadUID)
	return &detection, nil
}

// loadAndConvertV1Detection loads v1 format and converts to v2
func (s *VersionedDetectionStorage) loadAndConvertV1Detection(
	record *DetectionRecord,
) (*model.MultiDimensionalDetection, error) {
	// Parse v1 format
	var v1Detection model.FrameworkDetection
	
	if err := json.Unmarshal(record.Data, &v1Detection); err != nil {
		return nil, fmt.Errorf("failed to unmarshal v1 detection: %w", err)
	}
	
	log.Infof("Converting v1 detection to v2 for workload %s", record.WorkloadUID)
	
	// Convert v1 to v2
	v2Detection := ConvertV1ToV2(&v1Detection)
	
	return v2Detection, nil
}

// ConvertV1ToV2 converts old single-dimension detection to multi-dimensional
func ConvertV1ToV2(v1 *model.FrameworkDetection) *model.MultiDimensionalDetection {
	v2 := &model.MultiDimensionalDetection{
		WorkloadUID: "", // Will be set from record
		Version:     DetectionVersionV2,
		Dimensions:  make(map[model.DetectionDimension][]model.DimensionValue),
		Confidence:  v1.Confidence,
		Status:      v1.Status,
		Sources:     v1.Sources,
		Conflicts:   make(map[model.DetectionDimension][]model.DetectionConflict),
		UpdatedAt:   time.Now(),
	}
	
	// Convert wrapper framework
	if v1.WrapperFramework != "" {
		v2.Dimensions[model.DimensionWrapperFramework] = []model.DimensionValue{
			{
				Value:      v1.WrapperFramework,
				Confidence: v1.Confidence,
				Source:     "v1_migration",
				DetectedAt: time.Now(),
				Evidence: map[string]interface{}{
					"migrated_from": "v1",
					"original_data": v1.WrapperFramework,
				},
			},
		}
	}
	
	// Convert base framework
	if v1.BaseFramework != "" {
		v2.Dimensions[model.DimensionBaseFramework] = []model.DimensionValue{
			{
				Value:      v1.BaseFramework,
				Confidence: v1.Confidence,
				Source:     "v1_migration",
				DetectedAt: time.Now(),
				Evidence: map[string]interface{}{
					"migrated_from": "v1",
					"original_data": v1.BaseFramework,
				},
			},
		}
		
		// Infer runtime from base framework
		runtime := inferRuntimeFromFramework(v1.BaseFramework)
		if runtime != "" {
			v2.Dimensions[model.DimensionRuntime] = []model.DimensionValue{
				{
					Value:      runtime,
					Confidence: v1.Confidence * 0.9, // Slightly lower confidence for inferred
					Source:     "v1_migration_inferred",
					DetectedAt: time.Now(),
					Evidence: map[string]interface{}{
						"migrated_from":   "v1",
						"inferred_from":   v1.BaseFramework,
						"inference_basis": "base_framework",
					},
				},
			}
		}
	}
	
	// Convert behavior/task type
	if v1.Type != "" {
		v2.Dimensions[model.DimensionBehavior] = []model.DimensionValue{
			{
				Value:      v1.Type,
				Confidence: v1.Confidence,
				Source:     "v1_migration",
				DetectedAt: time.Now(),
				Evidence: map[string]interface{}{
					"migrated_from": "v1",
					"original_type": v1.Type,
				},
			},
		}
	}
	
	// Try to infer language from sources
	language := inferLanguageFromSources(v1.Sources)
	if language != "" {
		v2.Dimensions[model.DimensionLanguage] = []model.DimensionValue{
			{
				Value:      language,
				Confidence: 0.7, // Lower confidence for inferred
				Source:     "v1_migration_inferred",
				DetectedAt: time.Now(),
				Evidence: map[string]interface{}{
					"migrated_from":   "v1",
					"inferred_from":   "sources",
					"inference_basis": "detection_sources",
				},
			},
		}
	}
	
	// Convert conflicts (map to appropriate dimension)
	if len(v1.Conflicts) > 0 {
		// Try to determine which dimension the conflict belongs to
		// For simplicity, put all v1 conflicts in base_framework dimension
		v2.Conflicts[model.DimensionBaseFramework] = v1.Conflicts
	}
	
	log.Infof("Converted v1 detection: frameworks=%v -> dimensions with %d entries",
		v1.Frameworks, len(v2.Dimensions))
	
	return v2
}

// inferRuntimeFromFramework infers runtime from framework name
func inferRuntimeFromFramework(framework string) string {
	// PyTorch-based frameworks
	pytorchFrameworks := map[string]bool{
		"megatron": true, "deepspeed": true, "fairscale": true,
		"lightning": true, "pytorch_lightning": true,
		"transformers": true, "horovod": true,
	}
	
	if pytorchFrameworks[framework] {
		return "pytorch"
	}
	
	// TensorFlow-based
	if framework == "keras" || framework == "tensorflow" {
		return "tensorflow"
	}
	
	// JAX-based
	if framework == "jax" || framework == "flax" {
		return "jax"
	}
	
	return ""
}

// inferLanguageFromSources infers language from detection sources
func inferLanguageFromSources(sources []model.DetectionSource) string {
	// Check evidence for language hints
	for _, source := range sources {
		if source.Evidence == nil {
			continue
		}
		
		// Check for python indicators
		if system, ok := source.Evidence["system"].(map[string]interface{}); ok {
			if pythonVersion, ok := system["python_version"].(string); ok && pythonVersion != "" {
				return "python"
			}
		}
		
		// Check pytorch (implies python)
		if pytorch, ok := source.Evidence["pytorch"]; ok && pytorch != nil {
			return "python"
		}
		
		// Check wandb (typically python)
		if _, ok := source.Evidence["wandb"]; ok {
			return "python"
		}
	}
	
	// Default assumption for ML workloads
	return "python"
}

// BatchMigrate migrates multiple workloads from v1 to v2
func (s *VersionedDetectionStorage) BatchMigrate(
	ctx context.Context,
	workloadUIDs []string,
) (int, error) {
	successCount := 0
	
	for _, uid := range workloadUIDs {
		// Load (will auto-convert if v1)
		detection, err := s.LoadDetection(ctx, uid)
		if err != nil {
			log.Errorf("Failed to load detection for migration %s: %v", uid, err)
			continue
		}
		
		if detection == nil {
			continue
		}
		
		// Save (will write as v2)
		if err := s.SaveDetection(ctx, detection); err != nil {
			log.Errorf("Failed to save migrated detection %s: %v", uid, err)
			continue
		}
		
		successCount++
	}
	
	log.Infof("Batch migration completed: %d/%d workloads migrated", successCount, len(workloadUIDs))
	return successCount, nil
}

// GetVersionStats returns version distribution statistics
func (s *VersionedDetectionStorage) GetVersionStats(ctx context.Context) (map[string]int, error) {
	type VersionCount struct {
		Version string
		Count   int64
	}
	
	var results []VersionCount
	
	err := s.db.WithContext(ctx).
		Model(&DetectionRecord{}).
		Select("COALESCE(version, '1.0') as version, COUNT(*) as count").
		Group("version").
		Scan(&results).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get version stats: %w", err)
	}
	
	stats := make(map[string]int)
	for _, r := range results {
		stats[r.Version] = int(r.Count)
	}
	
	return stats, nil
}

// MigrateAllV1ToV2 migrates all v1 records to v2
func (s *VersionedDetectionStorage) MigrateAllV1ToV2(ctx context.Context) (int, error) {
	// Find all v1 records
	var records []DetectionRecord
	
	err := s.db.WithContext(ctx).
		Model(&DetectionRecord{}).
		Where("version = ? OR version IS NULL", DetectionVersionV1).
		Find(&records).Error
	
	if err != nil {
		return 0, fmt.Errorf("failed to query v1 records: %w", err)
	}
	
	log.Infof("Found %d v1 records to migrate", len(records))
	
	successCount := 0
	for _, record := range records {
		// Load and convert
		detection, err := s.loadAndConvertV1Detection(&record)
		if err != nil {
			log.Errorf("Failed to convert v1 record %s: %v", record.WorkloadUID, err)
			continue
		}
		
		detection.WorkloadUID = record.WorkloadUID
		
		// Save as v2
		if err := s.SaveDetection(ctx, detection); err != nil {
			log.Errorf("Failed to save migrated record %s: %v", record.WorkloadUID, err)
			continue
		}
		
		successCount++
		
		if successCount%100 == 0 {
			log.Infof("Migration progress: %d/%d records", successCount, len(records))
		}
	}
	
	log.Infof("Migration completed: %d/%d records successfully migrated", successCount, len(records))
	return successCount, nil
}

