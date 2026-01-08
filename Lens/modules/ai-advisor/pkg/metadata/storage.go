// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// DBStorage implements Storage interface using database
type DBStorage struct {
	db *gorm.DB
}

// NewDBStorage creates a new database storage
func NewDBStorage(db *gorm.DB) *DBStorage {
	return &DBStorage{db: db}
}

// Store stores workload metadata to database
func (s *DBStorage) Store(ctx context.Context, metadata *WorkloadMetadata) error {
	// Convert metadata to map for ExtType
	var metadataMap map[string]interface{}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := json.Unmarshal(metadataJSON, &metadataMap); err != nil {
		return fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	// Determine metadata type based on frameworks detected
	metadataType := "training"

	// Determine primary framework
	framework := metadata.BaseFramework
	if framework == "" && len(metadata.Frameworks) > 0 {
		framework = metadata.Frameworks[0]
	}

	// Extract image prefix from metadata if available
	imagePrefix := ""
	// This would need to come from pod spec, not available in current metadata
	// For now, leave empty or extract from other sources

	// Create or update record
	record := &model.AiWorkloadMetadata{
		WorkloadUID: metadata.WorkloadUID,
		Type:        metadataType,
		Framework:   framework,
		Metadata:    model.ExtType(metadataMap),
		ImagePrefix: imagePrefix,
		CreatedAt:   metadata.CollectedAt,
	}

	// Use UPSERT: Insert or update if exists
	result := s.db.WithContext(ctx).
		Where("workload_uid = ?", metadata.WorkloadUID).
		Assign(map[string]interface{}{
			"type":         record.Type,
			"framework":    record.Framework,
			"metadata":     record.Metadata,
			"image_prefix": record.ImagePrefix,
			"created_at":   record.CreatedAt,
		}).
		FirstOrCreate(record)

	if result.Error != nil {
		return fmt.Errorf("failed to store metadata: %w", result.Error)
	}

	log.Infof("Stored metadata for workload %s (framework: %s, type: %s)",
		metadata.WorkloadUID, framework, metadataType)

	return nil
}

// Get retrieves workload metadata from database
func (s *DBStorage) Get(ctx context.Context, workloadUID string) (*WorkloadMetadata, error) {
	var record model.AiWorkloadMetadata

	result := s.db.WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		First(&record)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get metadata: %w", result.Error)
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
func (s *DBStorage) Query(ctx context.Context, query *MetadataQuery) ([]*WorkloadMetadata, error) {
	db := s.db.WithContext(ctx)

	// Apply filters
	if query.WorkloadUID != "" {
		db = db.Where("workload_uid = ?", query.WorkloadUID)
	}

	if query.Framework != "" {
		db = db.Where("framework = ?", query.Framework)
	}

	if query.Type != "" {
		db = db.Where("type = ?", query.Type)
	}

	if query.StartTime != nil {
		db = db.Where("created_at >= ?", query.StartTime)
	}

	if query.EndTime != nil {
		db = db.Where("created_at <= ?", query.EndTime)
	}

	// Apply limit
	if query.Limit > 0 {
		db = db.Limit(query.Limit)
	} else {
		db = db.Limit(100) // Default limit
	}

	// Order by created_at desc
	db = db.Order("created_at DESC")

	// Execute query
	var records []model.AiWorkloadMetadata
	if err := db.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to query metadata: %w", err)
	}

	// Parse results
	results := make([]*WorkloadMetadata, 0, len(records))
	for _, record := range records {
		var metadata WorkloadMetadata
		metadataJSON, err := json.Marshal(record.Metadata)
		if err != nil {
			log.Warnf("Failed to marshal metadata for workload %s: %v", record.WorkloadUID, err)
			continue
		}
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			log.Warnf("Failed to unmarshal metadata for workload %s: %v", record.WorkloadUID, err)
			continue
		}
		results = append(results, &metadata)
	}

	return results, nil
}

// Delete deletes workload metadata from database
func (s *DBStorage) Delete(ctx context.Context, workloadUID string) error {
	result := s.db.WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		Delete(&model.AiWorkloadMetadata{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete metadata: %w", result.Error)
	}

	log.Infof("Deleted metadata for workload %s", workloadUID)
	return nil
}

// StoreBatch stores multiple metadata records in a transaction
func (s *DBStorage) StoreBatch(ctx context.Context, metadataList []*WorkloadMetadata) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, metadata := range metadataList {
			var metadataMap map[string]interface{}
			metadataJSON, err := json.Marshal(metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal metadata for %s: %w", metadata.WorkloadUID, err)
			}
			if err := json.Unmarshal(metadataJSON, &metadataMap); err != nil {
				return fmt.Errorf("failed to unmarshal to map for %s: %w", metadata.WorkloadUID, err)
			}

			framework := metadata.BaseFramework
			if framework == "" && len(metadata.Frameworks) > 0 {
				framework = metadata.Frameworks[0]
			}

			record := &model.AiWorkloadMetadata{
				WorkloadUID: metadata.WorkloadUID,
				Type:        "training",
				Framework:   framework,
				Metadata:    model.ExtType(metadataMap),
				CreatedAt:   metadata.CollectedAt,
			}

			result := tx.Where("workload_uid = ?", metadata.WorkloadUID).
				Assign(map[string]interface{}{
					"type":       record.Type,
					"framework":  record.Framework,
					"metadata":   record.Metadata,
					"created_at": record.CreatedAt,
				}).
				FirstOrCreate(record)

			if result.Error != nil {
				return fmt.Errorf("failed to store metadata for %s: %w", metadata.WorkloadUID, result.Error)
			}
		}
		return nil
	})
}

// GetStatistics retrieves metadata statistics
func (s *DBStorage) GetStatistics(ctx context.Context, startTime, endTime *time.Time) (*MetadataStatistics, error) {
	db := s.db.WithContext(ctx)

	if startTime != nil {
		db = db.Where("created_at >= ?", startTime)
	}
	if endTime != nil {
		db = db.Where("created_at <= ?", endTime)
	}

	stats := &MetadataStatistics{
		ByFramework: make(map[string]int),
		ByType:      make(map[string]int),
	}

	// Total count
	var totalCount int64
	if err := db.Model(&model.AiWorkloadMetadata{}).Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count total: %w", err)
	}
	stats.TotalWorkloads = int(totalCount)

	// By framework
	var frameworkCounts []struct {
		Framework string
		Count     int64
	}
	if err := db.Model(&model.AiWorkloadMetadata{}).
		Select("framework, COUNT(*) as count").
		Group("framework").
		Scan(&frameworkCounts).Error; err != nil {
		return nil, fmt.Errorf("failed to count by framework: %w", err)
	}
	for _, fc := range frameworkCounts {
		stats.ByFramework[fc.Framework] = int(fc.Count)
	}

	// By type
	var typeCounts []struct {
		Type  string
		Count int64
	}
	if err := db.Model(&model.AiWorkloadMetadata{}).
		Select("type, COUNT(*) as count").
		Group("type").
		Scan(&typeCounts).Error; err != nil {
		return nil, fmt.Errorf("failed to count by type: %w", err)
	}
	for _, tc := range typeCounts {
		stats.ByType[tc.Type] = int(tc.Count)
	}

	return stats, nil
}

// MetadataStatistics represents aggregated statistics
type MetadataStatistics struct {
	TotalWorkloads int            `json:"total_workloads"`
	ByFramework    map[string]int `json:"by_framework"`
	ByType         map[string]int `json:"by_type"`
}

// ListRecent lists recent metadata records
func (s *DBStorage) ListRecent(ctx context.Context, limit int) ([]*WorkloadMetadata, error) {
	if limit <= 0 {
		limit = 20
	}

	var records []model.AiWorkloadMetadata
	if err := s.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list recent metadata: %w", err)
	}

	results := make([]*WorkloadMetadata, 0, len(records))
	for _, record := range records {
		var metadata WorkloadMetadata
		metadataJSON, err := json.Marshal(record.Metadata)
		if err != nil {
			log.Warnf("Failed to marshal metadata for workload %s: %v", record.WorkloadUID, err)
			continue
		}
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			log.Warnf("Failed to unmarshal metadata for workload %s: %v", record.WorkloadUID, err)
			continue
		}
		results = append(results, &metadata)
	}

	return results, nil
}

// UpdateFramework updates the framework for a workload
func (s *DBStorage) UpdateFramework(ctx context.Context, workloadUID, framework string) error {
	result := s.db.WithContext(ctx).
		Model(&model.AiWorkloadMetadata{}).
		Where("workload_uid = ?", workloadUID).
		Update("framework", framework)

	if result.Error != nil {
		return fmt.Errorf("failed to update framework: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("workload not found: %s", workloadUID)
	}

	return nil
}
