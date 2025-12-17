package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/dal"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
	"gorm.io/gorm"
)

// TraceLensSessionFacadeInterface defines the TraceLens Session Facade interface
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
	ListByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.TracelensSessions, error)
	ListByUserID(ctx context.Context, userID string) ([]*model.TracelensSessions, error)
	ListByStatus(ctx context.Context, status string) ([]*model.TracelensSessions, error)
	ListActive(ctx context.Context) ([]*model.TracelensSessions, error)
	ListExpired(ctx context.Context) ([]*model.TracelensSessions, error)
	CountByStatus(ctx context.Context) (map[string]int, error)

	// Find existing session for reuse
	FindActiveSession(ctx context.Context, workloadUID string, profilerFileID int32) (*model.TracelensSessions, error)

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) TraceLensSessionFacadeInterface
}

// TraceLensSessionFacade implements TraceLensSessionFacadeInterface
type TraceLensSessionFacade struct {
	BaseFacade
}

// NewTraceLensSessionFacade creates a new TraceLens Session Facade
func NewTraceLensSessionFacade() *TraceLensSessionFacade {
	return &TraceLensSessionFacade{}
}

// GetDB returns the underlying GORM database connection
func (f *TraceLensSessionFacade) GetDB() *gorm.DB {
	return f.getDB()
}

// Create creates a new session record
func (f *TraceLensSessionFacade) Create(ctx context.Context, session *model.TracelensSessions) error {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions
	return q.WithContext(ctx).Create(session)
}

// GetBySessionID retrieves a session by its session_id
func (f *TraceLensSessionFacade) GetBySessionID(ctx context.Context, sessionID string) (*model.TracelensSessions, error) {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	record, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).First()
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return record, nil
}

// GetByID retrieves a session by its ID
func (f *TraceLensSessionFacade) GetByID(ctx context.Context, id int32) (*model.TracelensSessions, error) {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	record, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return record, nil
}

// Update updates an existing session record
func (f *TraceLensSessionFacade) Update(ctx context.Context, session *model.TracelensSessions) error {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	_, err := q.WithContext(ctx).Where(q.ID.Eq(session.ID)).Updates(session)
	return err
}

// Delete soft-deletes a session by session_id (marks as deleted)
func (f *TraceLensSessionFacade) Delete(ctx context.Context, sessionID string) error {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	now := time.Now()
	_, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
		"status":     tracelens.StatusDeleted,
		"deleted_at": now,
	})
	return err
}

// UpdateStatus updates the status and message of a session
func (f *TraceLensSessionFacade) UpdateStatus(ctx context.Context, sessionID, status, message string) error {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	_, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
		"status":         status,
		"status_message": message,
	})
	return err
}

// UpdatePodInfo updates the pod information of a session
func (f *TraceLensSessionFacade) UpdatePodInfo(ctx context.Context, sessionID, podName, podIP string, podPort int32) error {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	_, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
		"pod_name": podName,
		"pod_ip":   podIP,
		"pod_port": podPort,
	})
	return err
}

// UpdateLastAccessed updates the last_accessed_at timestamp
func (f *TraceLensSessionFacade) UpdateLastAccessed(ctx context.Context, sessionID string) error {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	_, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Update(
		q.LastAccessedAt, time.Now(),
	)
	return err
}

// MarkReady marks a session as ready with pod IP
func (f *TraceLensSessionFacade) MarkReady(ctx context.Context, sessionID, podIP string) error {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	now := time.Now()
	_, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
		"status":   tracelens.StatusReady,
		"pod_ip":   podIP,
		"ready_at": now,
	})
	return err
}

// MarkFailed marks a session as failed with reason
func (f *TraceLensSessionFacade) MarkFailed(ctx context.Context, sessionID, reason string) error {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	_, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
		"status":         tracelens.StatusFailed,
		"status_message": reason,
	})
	return err
}

// ListByWorkloadUID lists sessions for a workload
func (f *TraceLensSessionFacade) ListByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.TracelensSessions, error) {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	return q.WithContext(ctx).Where(
		q.WorkloadUID.Eq(workloadUID),
		q.Status.NotIn(tracelens.StatusDeleted),
	).Order(q.CreatedAt.Desc()).Find()
}

// ListByUserID lists sessions for a user
func (f *TraceLensSessionFacade) ListByUserID(ctx context.Context, userID string) ([]*model.TracelensSessions, error) {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	return q.WithContext(ctx).Where(
		q.UserID.Eq(userID),
		q.Status.NotIn(tracelens.StatusDeleted),
	).Order(q.CreatedAt.Desc()).Find()
}

// ListByStatus lists sessions by status
func (f *TraceLensSessionFacade) ListByStatus(ctx context.Context, status string) ([]*model.TracelensSessions, error) {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	return q.WithContext(ctx).Where(q.Status.Eq(status)).Find()
}

// ListActive lists all active sessions (pending, creating, initializing, ready)
func (f *TraceLensSessionFacade) ListActive(ctx context.Context) ([]*model.TracelensSessions, error) {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	return q.WithContext(ctx).Where(
		q.Status.In(tracelens.ActiveStatuses()...),
	).Find()
}

// ListExpired lists sessions that have expired but not yet cleaned up
func (f *TraceLensSessionFacade) ListExpired(ctx context.Context) ([]*model.TracelensSessions, error) {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	now := time.Now()
	return q.WithContext(ctx).Where(
		q.ExpiresAt.Lt(now),
		q.Status.NotIn(tracelens.StatusDeleted, tracelens.StatusExpired),
	).Find()
}

// CountByStatus returns a map of status to count
func (f *TraceLensSessionFacade) CountByStatus(ctx context.Context) (map[string]int, error) {
	db := f.getDB()

	var results []struct {
		Status string
		Count  int
	}

	err := db.WithContext(ctx).
		Table("tracelens_sessions").
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

// FindActiveSession finds an existing active session for reuse
func (f *TraceLensSessionFacade) FindActiveSession(ctx context.Context, workloadUID string, profilerFileID int32) (*model.TracelensSessions, error) {
	db := f.getDB()
	q := dal.Use(db).TracelensSessions

	now := time.Now()
	record, err := q.WithContext(ctx).Where(
		q.WorkloadUID.Eq(workloadUID),
		q.ProfilerFileID.Eq(profilerFileID),
		q.Status.Eq(tracelens.StatusReady),
		q.ExpiresAt.Gt(now), // Not expired
	).First()

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return record, nil
}

// WithCluster returns a new facade instance for the specified cluster
func (f *TraceLensSessionFacade) WithCluster(clusterName string) TraceLensSessionFacadeInterface {
	return &TraceLensSessionFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

