// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkloadDetectionFacadeInterface defines the database operation interface for detection state
type WorkloadDetectionFacadeInterface interface {
	// GetDetection retrieves detection state by workload UID
	GetDetection(ctx context.Context, workloadUID string) (*model.WorkloadDetection, error)

	// CreateDetection creates a new detection state record
	CreateDetection(ctx context.Context, detection *model.WorkloadDetection) error

	// UpdateDetection updates an existing detection state record
	UpdateDetection(ctx context.Context, detection *model.WorkloadDetection) error

	// UpsertDetection creates or updates a detection state record
	UpsertDetection(ctx context.Context, detection *model.WorkloadDetection) error

	// DeleteDetection deletes a detection state record
	DeleteDetection(ctx context.Context, workloadUID string) error

	// ListDetectionsByStatus lists detections by status
	ListDetectionsByStatus(ctx context.Context, status string, limit int, offset int) ([]*model.WorkloadDetection, int64, error)

	// ListDetectionsByDetectionState lists detections by detection task state
	ListDetectionsByDetectionState(ctx context.Context, detectionState string) ([]*model.WorkloadDetection, error)

	// ListPendingDetections lists detections in pending state
	ListPendingDetections(ctx context.Context) ([]*model.WorkloadDetection, error)

	// ListDetectionsNeedingRetry lists detections that need retry (next_attempt_at <= now)
	ListDetectionsNeedingRetry(ctx context.Context, limit int) ([]*model.WorkloadDetection, error)

	// ListConfirmedDetections lists detections that are confirmed or verified
	ListConfirmedDetections(ctx context.Context, limit int, offset int) ([]*model.WorkloadDetection, int64, error)

	// ListConflictedDetections lists detections with conflicts
	ListConflictedDetections(ctx context.Context, limit int, offset int) ([]*model.WorkloadDetection, int64, error)

	// UpdateDetectionState updates the detection task state
	UpdateDetectionState(ctx context.Context, workloadUID string, detectionState string) error

	// UpdateDetectionStatus updates the detection status (unknown, suspected, confirmed, verified, conflict)
	UpdateDetectionStatus(ctx context.Context, workloadUID string, status string, confidence float64) error

	// IncrementAttemptCount increments the attempt count and updates last_attempt_at
	IncrementAttemptCount(ctx context.Context, workloadUID string) error

	// SetNextAttemptAt sets the next attempt time for a detection
	SetNextAttemptAt(ctx context.Context, workloadUID string, nextAttempt time.Time) error

	// UpdateEvidenceSummary updates the evidence count and sources
	UpdateEvidenceSummary(ctx context.Context, workloadUID string, evidenceCount int, sources []string) error

	// UpdateAggregatedResult updates the aggregated detection result
	UpdateAggregatedResult(ctx context.Context, workloadUID string, framework string, frameworks []string,
		workloadType string, confidence float64, frameworkLayer string, wrapperFramework string,
		baseFramework string, status string) error

	// MarkAsConfirmed marks a detection as confirmed with timestamp
	MarkAsConfirmed(ctx context.Context, workloadUID string, framework string, confidence float64) error

	// UpdateIntentResult updates only the intent analysis fields without touching detection fields
	UpdateIntentResult(ctx context.Context, workloadUID string, updates map[string]interface{}) error

	// UpdateIntentState updates the intent analysis lifecycle state
	UpdateIntentState(ctx context.Context, workloadUID string, intentState string) error

	// ListByIntentState lists detections by intent analysis state
	ListByIntentState(ctx context.Context, intentState string, limit int, offset int) ([]*model.WorkloadDetection, int64, error)

	// ListByCategory lists detections by intent category
	ListByCategory(ctx context.Context, category string, limit int, offset int) ([]*model.WorkloadDetection, int64, error)

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) WorkloadDetectionFacadeInterface
}

// WorkloadDetectionFacade implements WorkloadDetectionFacadeInterface
type WorkloadDetectionFacade struct {
	BaseFacade
}

// NewWorkloadDetectionFacade creates a new WorkloadDetectionFacade instance
func NewWorkloadDetectionFacade() WorkloadDetectionFacadeInterface {
	return &WorkloadDetectionFacade{}
}

func (f *WorkloadDetectionFacade) WithCluster(clusterName string) WorkloadDetectionFacadeInterface {
	return &WorkloadDetectionFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// GetDetection retrieves detection state by workload UID
func (f *WorkloadDetectionFacade) GetDetection(ctx context.Context, workloadUID string) (*model.WorkloadDetection, error) {
	// Use raw GORM query instead of DAL to avoid potential issues with generated code
	var result model.WorkloadDetection
	err := f.getDB().WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	// Double-check: if ID is 0, treat as not found
	if result.ID == 0 {
		return nil, nil
	}
	return &result, nil
}

// CreateDetection creates a new detection state record
func (f *WorkloadDetectionFacade) CreateDetection(ctx context.Context, detection *model.WorkloadDetection) error {
	now := time.Now()
	if detection.CreatedAt.IsZero() {
		detection.CreatedAt = now
	}
	detection.UpdatedAt = now
	return f.getDAL().WorkloadDetection.WithContext(ctx).Create(detection)
}

// UpdateDetection updates an existing detection state record
func (f *WorkloadDetectionFacade) UpdateDetection(ctx context.Context, detection *model.WorkloadDetection) error {
	detection.UpdatedAt = time.Now()
	return f.getDAL().WorkloadDetection.WithContext(ctx).Save(detection)
}

// UpsertDetection creates or updates a detection state record
func (f *WorkloadDetectionFacade) UpsertDetection(ctx context.Context, detection *model.WorkloadDetection) error {
	now := time.Now()
	if detection.CreatedAt.IsZero() {
		detection.CreatedAt = now
	}
	detection.UpdatedAt = now

	db := f.getDB()
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workload_uid"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status", "framework", "frameworks", "workload_type", "confidence",
				"framework_layer", "wrapper_framework", "base_framework",
				"detection_state", "attempt_count", "max_attempts",
				"last_attempt_at", "next_attempt_at", "context",
				"evidence_count", "evidence_sources", "conflicts",
				"updated_at", "confirmed_at",
			}),
		}).
		Create(detection).Error
}

// DeleteDetection deletes a detection state record
func (f *WorkloadDetectionFacade) DeleteDetection(ctx context.Context, workloadUID string) error {
	q := f.getDAL().WorkloadDetection
	_, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).Delete()
	return err
}

// ListDetectionsByStatus lists detections by status with pagination
// If status is empty string, returns all detections without status filtering
func (f *WorkloadDetectionFacade) ListDetectionsByStatus(ctx context.Context, status string, limit int, offset int) ([]*model.WorkloadDetection, int64, error) {
	q := f.getDAL().WorkloadDetection
	query := q.WithContext(ctx).Order(q.UpdatedAt.Desc())

	// Only add status filter if status is not empty
	if status != "" {
		query = query.Where(q.Status.Eq(status))
	}

	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}

	results, err := query.Limit(limit).Offset(offset).Find()
	if err != nil {
		return nil, 0, err
	}

	return results, count, nil
}

// ListDetectionsByDetectionState lists detections by detection task state
func (f *WorkloadDetectionFacade) ListDetectionsByDetectionState(ctx context.Context, detectionState string) ([]*model.WorkloadDetection, error) {
	q := f.getDAL().WorkloadDetection
	results, err := q.WithContext(ctx).
		Where(q.DetectionState.Eq(detectionState)).
		Order(q.UpdatedAt.Asc()).
		Find()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListPendingDetections lists detections in pending state
func (f *WorkloadDetectionFacade) ListPendingDetections(ctx context.Context) ([]*model.WorkloadDetection, error) {
	return f.ListDetectionsByDetectionState(ctx, "pending")
}

// ListDetectionsNeedingRetry lists detections that need retry (next_attempt_at <= now)
func (f *WorkloadDetectionFacade) ListDetectionsNeedingRetry(ctx context.Context, limit int) ([]*model.WorkloadDetection, error) {
	db := f.getDB()
	now := time.Now()

	var results []*model.WorkloadDetection
	err := db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("detection_state = ?", "in_progress").
		Where("next_attempt_at <= ?", now).
		Where("attempt_count < max_attempts").
		Order("next_attempt_at ASC").
		Limit(limit).
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListConfirmedDetections lists detections that are confirmed or verified
func (f *WorkloadDetectionFacade) ListConfirmedDetections(ctx context.Context, limit int, offset int) ([]*model.WorkloadDetection, int64, error) {
	q := f.getDAL().WorkloadDetection
	query := q.WithContext(ctx).
		Where(q.Status.In("confirmed", "verified")).
		Order(q.ConfirmedAt.Desc())

	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}

	results, err := query.Limit(limit).Offset(offset).Find()
	if err != nil {
		return nil, 0, err
	}

	return results, count, nil
}

// ListConflictedDetections lists detections with conflicts
func (f *WorkloadDetectionFacade) ListConflictedDetections(ctx context.Context, limit int, offset int) ([]*model.WorkloadDetection, int64, error) {
	q := f.getDAL().WorkloadDetection
	query := q.WithContext(ctx).
		Where(q.Status.Eq("conflict")).
		Order(q.UpdatedAt.Desc())

	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}

	results, err := query.Limit(limit).Offset(offset).Find()
	if err != nil {
		return nil, 0, err
	}

	return results, count, nil
}

// UpdateDetectionState updates the detection task state
func (f *WorkloadDetectionFacade) UpdateDetectionState(ctx context.Context, workloadUID string, detectionState string) error {
	q := f.getDAL().WorkloadDetection
	_, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		UpdateSimple(q.DetectionState.Value(detectionState), q.UpdatedAt.Value(time.Now()))
	return err
}

// UpdateDetectionStatus updates the detection status
func (f *WorkloadDetectionFacade) UpdateDetectionStatus(ctx context.Context, workloadUID string, status string, confidence float64) error {
	db := f.getDB()
	now := time.Now()

	updates := map[string]interface{}{
		"status":     status,
		"confidence": confidence,
		"updated_at": now,
	}

	// If status is confirmed or verified, set confirmed_at
	if status == "confirmed" || status == "verified" {
		updates["confirmed_at"] = now
	}

	return db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("workload_uid = ?", workloadUID).
		Updates(updates).Error
}

// IncrementAttemptCount increments the attempt count and updates last_attempt_at
func (f *WorkloadDetectionFacade) IncrementAttemptCount(ctx context.Context, workloadUID string) error {
	db := f.getDB()
	now := time.Now()

	return db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("workload_uid = ?", workloadUID).
		Updates(map[string]interface{}{
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"last_attempt_at": now,
			"updated_at":      now,
		}).Error
}

// SetNextAttemptAt sets the next attempt time for a detection
func (f *WorkloadDetectionFacade) SetNextAttemptAt(ctx context.Context, workloadUID string, nextAttempt time.Time) error {
	q := f.getDAL().WorkloadDetection
	_, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		UpdateSimple(q.NextAttemptAt.Value(nextAttempt), q.UpdatedAt.Value(time.Now()))
	return err
}

// UpdateEvidenceSummary updates the evidence count and sources
func (f *WorkloadDetectionFacade) UpdateEvidenceSummary(ctx context.Context, workloadUID string, evidenceCount int, sources []string) error {
	db := f.getDB()

	return db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("workload_uid = ?", workloadUID).
		Updates(map[string]interface{}{
			"evidence_count":   evidenceCount,
			"evidence_sources": sources,
			"updated_at":       time.Now(),
		}).Error
}

// UpdateAggregatedResult updates the aggregated detection result
func (f *WorkloadDetectionFacade) UpdateAggregatedResult(ctx context.Context, workloadUID string,
	framework string, frameworks []string, workloadType string, confidence float64,
	frameworkLayer string, wrapperFramework string, baseFramework string, status string) error {

	db := f.getDB()
	now := time.Now()

	updates := map[string]interface{}{
		"framework":         framework,
		"frameworks":        frameworks,
		"workload_type":     workloadType,
		"confidence":        confidence,
		"framework_layer":   frameworkLayer,
		"wrapper_framework": wrapperFramework,
		"base_framework":    baseFramework,
		"status":            status,
		"updated_at":        now,
	}

	if status == "confirmed" || status == "verified" {
		updates["confirmed_at"] = now
		updates["detection_state"] = "completed"
	}

	return db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("workload_uid = ?", workloadUID).
		Updates(updates).Error
}

// MarkAsConfirmed marks a detection as confirmed with timestamp
func (f *WorkloadDetectionFacade) MarkAsConfirmed(ctx context.Context, workloadUID string, framework string, confidence float64) error {
	db := f.getDB()
	now := time.Now()

	return db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("workload_uid = ?", workloadUID).
		Updates(map[string]interface{}{
			"status":          "confirmed",
			"framework":       framework,
			"confidence":      confidence,
			"detection_state": "completed",
			"confirmed_at":    now,
			"updated_at":      now,
		}).Error
}

// UpdateIntentResult updates only the intent analysis fields without touching detection fields.
// The updates map should use column names as keys (e.g., "category", "model_family", "intent_detail").
// Allowed intent fields: category, expected_behavior, model_path, model_family, model_scale,
// model_variant, runtime_framework, intent_detail, intent_confidence, intent_source,
// intent_reasoning, intent_field_sources, intent_analysis_mode, intent_matched_rules,
// intent_state, intent_analyzed_at.
func (f *WorkloadDetectionFacade) UpdateIntentResult(ctx context.Context, workloadUID string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	// Whitelist of allowed intent fields to prevent accidental detection field overwrites
	allowedFields := map[string]bool{
		"category": true, "expected_behavior": true,
		"model_path": true, "model_family": true, "model_scale": true, "model_variant": true,
		"runtime_framework": true, "intent_detail": true,
		"intent_confidence": true, "intent_source": true, "intent_reasoning": true,
		"intent_field_sources": true, "intent_analysis_mode": true, "intent_matched_rules": true,
		"intent_state": true, "intent_analyzed_at": true,
	}

	filtered := make(map[string]interface{})
	for k, v := range updates {
		if allowedFields[k] {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	filtered["updated_at"] = time.Now()

	db := f.getDB()

	// Ensure the workload_detection row exists before updating. The
	// detection_coordinator may not have created one if it couldn't find
	// pods (e.g. Workload/PyTorchJob UID mismatch).
	existing, _ := f.GetDetection(ctx, workloadUID)
	if existing == nil {
		now := time.Now()
		seed := &model.WorkloadDetection{
			WorkloadUID: workloadUID,
			Status:      "unknown",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := f.getDAL().WorkloadDetection.WithContext(ctx).Create(seed); err != nil {
			return fmt.Errorf("failed to create detection record for intent result: %w", err)
		}
	}

	return db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("workload_uid = ?", workloadUID).
		Updates(filtered).Error
}

// UpdateIntentState updates the intent analysis lifecycle state.
// Creates the workload_detection row if it does not exist yet.
func (f *WorkloadDetectionFacade) UpdateIntentState(ctx context.Context, workloadUID string, intentState string) error {
	db := f.getDB()
	now := time.Now()

	existing, _ := f.GetDetection(ctx, workloadUID)
	if existing == nil {
		seed := &model.WorkloadDetection{
			WorkloadUID: workloadUID,
			Status:      "unknown",
			IntentState: &intentState,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		return f.getDAL().WorkloadDetection.WithContext(ctx).Create(seed)
	}

	updates := map[string]interface{}{
		"intent_state": intentState,
		"updated_at":   now,
	}
	if intentState == "completed" {
		updates["intent_analyzed_at"] = now
	}

	return db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("workload_uid = ?", workloadUID).
		Updates(updates).Error
}

// ListByIntentState lists detections by intent analysis state
func (f *WorkloadDetectionFacade) ListByIntentState(ctx context.Context, intentState string, limit int, offset int) ([]*model.WorkloadDetection, int64, error) {
	db := f.getDB()
	var results []*model.WorkloadDetection
	var total int64

	query := db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("intent_state = ?", intentState)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("updated_at DESC").Limit(limit).Offset(offset).Find(&results).Error; err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// ListByCategory lists detections by intent category
func (f *WorkloadDetectionFacade) ListByCategory(ctx context.Context, category string, limit int, offset int) ([]*model.WorkloadDetection, int64, error) {
	db := f.getDB()
	var results []*model.WorkloadDetection
	var total int64

	query := db.WithContext(ctx).
		Table(model.TableNameWorkloadDetection).
		Where("category = ?", category)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("updated_at DESC").Limit(limit).Offset(offset).Find(&results).Error; err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

