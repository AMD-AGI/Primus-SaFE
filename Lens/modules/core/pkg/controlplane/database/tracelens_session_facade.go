// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// TraceLensSessionFacadeInterface defines the TraceLens Session Facade interface for Control Plane
type TraceLensSessionFacadeInterface interface {
	// GetDB returns the underlying GORM database connection
	GetDB() *gorm.DB

	// CRUD operations
	Create(ctx context.Context, session *model.TracelensSessions) error
	GetBySessionID(ctx context.Context, sessionID string) (*model.TracelensSessions, error)
	GetByID(ctx context.Context, id int32) (*model.TracelensSessions, error)
	Update(ctx context.Context, session *model.TracelensSessions) error
	Delete(ctx context.Context, sessionID string) error

	// Status management
	UpdateStatus(ctx context.Context, sessionID, status, message string) error
	UpdatePodInfo(ctx context.Context, sessionID, podName, podIP string, podPort int32) error
	UpdateLastAccessed(ctx context.Context, sessionID string) error
	MarkReady(ctx context.Context, sessionID, podIP string) error
	MarkFailed(ctx context.Context, sessionID, reason string) error

	// Query operations
	ListByCluster(ctx context.Context, clusterName string) ([]*model.TracelensSessions, error)
	ListByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.TracelensSessions, error)
	ListByStatus(ctx context.Context, status string) ([]*model.TracelensSessions, error)
	ListActive(ctx context.Context) ([]*model.TracelensSessions, error)
	ListExpired(ctx context.Context) ([]*model.TracelensSessions, error)
	ListAllClusters(ctx context.Context) ([]string, error)
	CountByStatus(ctx context.Context) (map[string]int, error)
	CountByCluster(ctx context.Context) (map[string]int, error)

	// Find existing session for reuse
	FindActiveSession(ctx context.Context, clusterName, workloadUID string, profilerFileID int32) (*model.TracelensSessions, error)
}

// TraceLensSessionFacade implements TraceLensSessionFacadeInterface
type TraceLensSessionFacade struct {
	db *gorm.DB
}

// NewTraceLensSessionFacade creates a new TraceLens Session Facade for Control Plane
func NewTraceLensSessionFacade(db *gorm.DB) TraceLensSessionFacadeInterface {
	return &TraceLensSessionFacade{db: db}
}

// GetDB returns the underlying GORM database connection
func (f *TraceLensSessionFacade) GetDB() *gorm.DB {
	return f.db
}

// Create creates a new session record
func (f *TraceLensSessionFacade) Create(ctx context.Context, session *model.TracelensSessions) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	err := f.db.WithContext(ctx).Create(session).Error
	if err != nil {
		log.Errorf("TraceLensSessionFacade Create: failed to create session: %v", err)
		return err
	}
	log.Infof("TraceLensSessionFacade Create: created session %s for cluster %s", session.SessionID, session.ClusterName)
	return nil
}

// GetBySessionID retrieves a session by its session_id
func (f *TraceLensSessionFacade) GetBySessionID(ctx context.Context, sessionID string) (*model.TracelensSessions, error) {
	var session model.TracelensSessions
	err := f.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// GetByID retrieves a session by its ID
func (f *TraceLensSessionFacade) GetByID(ctx context.Context, id int32) (*model.TracelensSessions, error) {
	var session model.TracelensSessions
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// Update updates an existing session record
func (f *TraceLensSessionFacade) Update(ctx context.Context, session *model.TracelensSessions) error {
	err := f.db.WithContext(ctx).Save(session).Error
	if err != nil {
		log.Errorf("TraceLensSessionFacade Update: failed to update session: %v", err)
		return err
	}
	return nil
}

// Delete soft-deletes a session by session_id
func (f *TraceLensSessionFacade) Delete(ctx context.Context, sessionID string) error {
	now := time.Now()
	result := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":     model.SessionStatusDeleted,
			"deleted_at": now,
		})
	if result.Error != nil {
		log.Errorf("TraceLensSessionFacade Delete: failed to delete session: %v", result.Error)
		return result.Error
	}
	return nil
}

// UpdateStatus updates the status and message of a session
func (f *TraceLensSessionFacade) UpdateStatus(ctx context.Context, sessionID, status, message string) error {
	result := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":         status,
			"status_message": message,
		})
	if result.Error != nil {
		log.Errorf("TraceLensSessionFacade UpdateStatus: failed to update status: %v", result.Error)
		return result.Error
	}
	return nil
}

// UpdatePodInfo updates the pod information of a session
func (f *TraceLensSessionFacade) UpdatePodInfo(ctx context.Context, sessionID, podName, podIP string, podPort int32) error {
	result := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"pod_name": podName,
			"pod_ip":   podIP,
			"pod_port": podPort,
		})
	if result.Error != nil {
		log.Errorf("TraceLensSessionFacade UpdatePodInfo: failed to update pod info: %v", result.Error)
		return result.Error
	}
	return nil
}

// UpdateLastAccessed updates the last_accessed_at timestamp
func (f *TraceLensSessionFacade) UpdateLastAccessed(ctx context.Context, sessionID string) error {
	result := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Where("session_id = ?", sessionID).
		Update("last_accessed_at", time.Now())
	if result.Error != nil {
		log.Errorf("TraceLensSessionFacade UpdateLastAccessed: failed to update last accessed: %v", result.Error)
		return result.Error
	}
	return nil
}

// MarkReady marks a session as ready with pod IP
func (f *TraceLensSessionFacade) MarkReady(ctx context.Context, sessionID, podIP string) error {
	now := time.Now()
	result := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":   model.SessionStatusReady,
			"pod_ip":   podIP,
			"ready_at": now,
		})
	if result.Error != nil {
		log.Errorf("TraceLensSessionFacade MarkReady: failed to mark ready: %v", result.Error)
		return result.Error
	}
	log.Infof("TraceLensSessionFacade MarkReady: session %s is ready with IP %s", sessionID, podIP)
	return nil
}

// MarkFailed marks a session as failed with reason
func (f *TraceLensSessionFacade) MarkFailed(ctx context.Context, sessionID, reason string) error {
	result := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":         model.SessionStatusFailed,
			"status_message": reason,
		})
	if result.Error != nil {
		log.Errorf("TraceLensSessionFacade MarkFailed: failed to mark failed: %v", result.Error)
		return result.Error
	}
	log.Warnf("TraceLensSessionFacade MarkFailed: session %s failed: %s", sessionID, reason)
	return nil
}

// ListByCluster lists sessions for a specific cluster
func (f *TraceLensSessionFacade) ListByCluster(ctx context.Context, clusterName string) ([]*model.TracelensSessions, error) {
	var sessions []*model.TracelensSessions
	err := f.db.WithContext(ctx).
		Where("cluster_name = ? AND deleted_at IS NULL", clusterName).
		Order("created_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// ListByWorkloadUID lists sessions for a workload
func (f *TraceLensSessionFacade) ListByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.TracelensSessions, error) {
	var sessions []*model.TracelensSessions
	err := f.db.WithContext(ctx).
		Where("workload_uid = ? AND status != ? AND deleted_at IS NULL", workloadUID, model.SessionStatusDeleted).
		Order("created_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// ListByStatus lists sessions by status
func (f *TraceLensSessionFacade) ListByStatus(ctx context.Context, status string) ([]*model.TracelensSessions, error) {
	var sessions []*model.TracelensSessions
	err := f.db.WithContext(ctx).
		Where("status = ?", status).
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// ListActive lists all active sessions (pending, creating, initializing, ready)
func (f *TraceLensSessionFacade) ListActive(ctx context.Context) ([]*model.TracelensSessions, error) {
	var sessions []*model.TracelensSessions
	err := f.db.WithContext(ctx).
		Where("status IN ? AND deleted_at IS NULL", model.ActiveStatuses()).
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// ListExpired lists sessions that have expired but not yet cleaned up
func (f *TraceLensSessionFacade) ListExpired(ctx context.Context) ([]*model.TracelensSessions, error) {
	var sessions []*model.TracelensSessions
	now := time.Now()
	err := f.db.WithContext(ctx).
		Where("expires_at < ? AND status NOT IN ? AND deleted_at IS NULL",
			now, []string{model.SessionStatusDeleted, model.SessionStatusExpired}).
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// ListAllClusters lists all unique cluster names that have sessions
func (f *TraceLensSessionFacade) ListAllClusters(ctx context.Context) ([]string, error) {
	var clusters []string
	err := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Where("deleted_at IS NULL").
		Distinct("cluster_name").
		Pluck("cluster_name", &clusters).Error
	if err != nil {
		return nil, err
	}
	return clusters, nil
}

// CountByStatus returns a map of status to count
func (f *TraceLensSessionFacade) CountByStatus(ctx context.Context) (map[string]int, error) {
	var results []struct {
		Status string
		Count  int
	}

	err := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Select("status, count(*) as count").
		Where("deleted_at IS NULL").
		Group("status").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.Status] = r.Count
	}
	return counts, nil
}

// CountByCluster returns a map of cluster name to active session count
func (f *TraceLensSessionFacade) CountByCluster(ctx context.Context) (map[string]int, error) {
	var results []struct {
		ClusterName string
		Count       int
	}

	err := f.db.WithContext(ctx).
		Model(&model.TracelensSessions{}).
		Select("cluster_name, count(*) as count").
		Where("status IN ? AND deleted_at IS NULL", model.ActiveStatuses()).
		Group("cluster_name").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.ClusterName] = r.Count
	}
	return counts, nil
}

// FindActiveSession finds an existing active session for reuse
func (f *TraceLensSessionFacade) FindActiveSession(ctx context.Context, clusterName, workloadUID string, profilerFileID int32) (*model.TracelensSessions, error) {
	var session model.TracelensSessions
	now := time.Now()
	err := f.db.WithContext(ctx).
		Where("cluster_name = ? AND workload_uid = ? AND profiler_file_id = ? AND status = ? AND expires_at > ? AND deleted_at IS NULL",
			clusterName, workloadUID, profilerFileID, model.SessionStatusReady, now).
		First(&session).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}
