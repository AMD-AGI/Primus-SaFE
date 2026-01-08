package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AITaskFacadeInterface defines the database operation interface for AI tasks
type AITaskFacadeInterface interface {
	// Create creates a new task
	Create(ctx context.Context, task *model.AITask) error

	// Get retrieves a task by ID
	Get(ctx context.Context, id string) (*model.AITask, error)

	// ClaimTask claims a pending task for processing (atomic operation)
	ClaimTask(ctx context.Context, topics []string, agentID string) (*model.AITask, error)

	// Complete marks a task as completed with result
	Complete(ctx context.Context, id string, outputPayload json.RawMessage) error

	// Fail marks a task as failed
	Fail(ctx context.Context, id string, errorCode int, errorMsg string, retryCount int, maxRetries int) error

	// Cancel cancels a pending or processing task
	Cancel(ctx context.Context, id string) error

	// List lists tasks with optional filters
	List(ctx context.Context, filter *AITaskFilter) ([]*model.AITask, error)

	// Count counts tasks matching filter
	Count(ctx context.Context, filter *AITaskFilter) (int64, error)

	// HandleTimeouts resets timed-out processing tasks to pending or failed
	HandleTimeouts(ctx context.Context, defaultTimeout time.Duration, maxRetries int) (int, error)

	// Cleanup removes old completed/failed/cancelled tasks
	Cleanup(ctx context.Context, olderThan time.Duration) (int, error)

	// WithCluster method
	WithCluster(clusterName string) AITaskFacadeInterface
}

// AITaskFilter defines filter conditions for querying AI tasks
type AITaskFilter struct {
	Status        *string
	Topic         *string
	Topics        []string
	AgentID       *string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit         int
	Offset        int
}

// AITaskFacade implements AITaskFacadeInterface
type AITaskFacade struct {
	BaseFacade
}

// NewAITaskFacade creates a new AITaskFacade instance
func NewAITaskFacade() AITaskFacadeInterface {
	return &AITaskFacade{}
}

func (f *AITaskFacade) WithCluster(clusterName string) AITaskFacadeInterface {
	return &AITaskFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new task
func (f *AITaskFacade) Create(ctx context.Context, task *model.AITask) error {
	db := f.getDB().WithContext(ctx)

	// Serialize context if needed
	if task.ContextJSON == "" && task.Context != nil {
		contextBytes, err := json.Marshal(task.Context)
		if err != nil {
			return err
		}
		task.ContextJSON = string(contextBytes)
	}

	return db.Create(task).Error
}

// Get retrieves a task by ID
func (f *AITaskFacade) Get(ctx context.Context, id string) (*model.AITask, error) {
	db := f.getDB().WithContext(ctx)
	var task model.AITask
	err := db.Where("id = ?", id).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	deserializeTask(&task)
	return &task, nil
}

// ClaimTask claims a pending task for processing using SELECT FOR UPDATE SKIP LOCKED
func (f *AITaskFacade) ClaimTask(ctx context.Context, topics []string, agentID string) (*model.AITask, error) {
	db := f.getDB().WithContext(ctx)
	var task model.AITask

	err := db.Transaction(func(tx *gorm.DB) error {
		// Find and lock a pending task
		query := tx.Clauses(clause.Locking{
			Strength: "UPDATE",
			Options:  "SKIP LOCKED",
		}).Where("status = ?", "pending")

		if len(topics) > 0 {
			query = query.Where("topic IN ?", topics)
		}

		result := query.Order("priority DESC, created_at ASC").First(&task)
		if result.Error != nil {
			return result.Error
		}

		// Update task status
		now := time.Now()
		task.Status = "processing"
		task.AgentID = agentID
		task.StartedAt = &now

		return tx.Save(&task).Error
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No pending tasks
		}
		return nil, err
	}

	deserializeTask(&task)
	return &task, nil
}

// Complete marks a task as completed with result
func (f *AITaskFacade) Complete(ctx context.Context, id string, outputPayload json.RawMessage) error {
	db := f.getDB().WithContext(ctx)
	now := time.Now()

	result := db.Model(&model.AITask{}).
		Where("id = ? AND status = ?", id, "processing").
		Updates(map[string]interface{}{
			"status":         "completed",
			"output_payload": outputPayload,
			"completed_at":   now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// Fail marks a task as failed
func (f *AITaskFacade) Fail(ctx context.Context, id string, errorCode int, errorMsg string, retryCount int, maxRetries int) error {
	db := f.getDB().WithContext(ctx)
	now := time.Now()

	// Determine if we should retry or fail permanently
	if retryCount < maxRetries {
		// Reset to pending for retry
		result := db.Model(&model.AITask{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"status":        "pending",
				"retry_count":   retryCount,
				"error_message": errorMsg,
				"error_code":    errorCode,
				"agent_id":      "",
				"started_at":    nil,
			})
		return result.Error
	}

	// Permanent failure
	result := db.Model(&model.AITask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        "failed",
			"retry_count":   retryCount,
			"error_message": errorMsg,
			"error_code":    errorCode,
			"completed_at":  now,
		})

	return result.Error
}

// Cancel cancels a pending or processing task
func (f *AITaskFacade) Cancel(ctx context.Context, id string) error {
	db := f.getDB().WithContext(ctx)
	now := time.Now()

	result := db.Model(&model.AITask{}).
		Where("id = ? AND status IN ?", id, []string{"pending", "processing"}).
		Updates(map[string]interface{}{
			"status":       "cancelled",
			"completed_at": now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// List lists tasks with optional filters
func (f *AITaskFacade) List(ctx context.Context, filter *AITaskFilter) ([]*model.AITask, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AITask{})

	if filter != nil {
		if filter.Status != nil {
			query = query.Where("status = ?", *filter.Status)
		}
		if filter.Topic != nil {
			query = query.Where("topic = ?", *filter.Topic)
		}
		if len(filter.Topics) > 0 {
			query = query.Where("topic IN ?", filter.Topics)
		}
		if filter.AgentID != nil {
			query = query.Where("agent_id = ?", *filter.AgentID)
		}
		if filter.CreatedAfter != nil {
			query = query.Where("created_at > ?", *filter.CreatedAfter)
		}
		if filter.CreatedBefore != nil {
			query = query.Where("created_at < ?", *filter.CreatedBefore)
		}
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit).Offset(filter.Offset)
		}
	}

	var tasks []model.AITask
	if err := query.Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, err
	}

	result := make([]*model.AITask, len(tasks))
	for i := range tasks {
		deserializeTask(&tasks[i])
		result[i] = &tasks[i]
	}

	return result, nil
}

// Count counts tasks matching filter
func (f *AITaskFacade) Count(ctx context.Context, filter *AITaskFilter) (int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.AITask{})

	if filter != nil {
		if filter.Status != nil {
			query = query.Where("status = ?", *filter.Status)
		}
		if filter.Topic != nil {
			query = query.Where("topic = ?", *filter.Topic)
		}
		if len(filter.Topics) > 0 {
			query = query.Where("topic IN ?", filter.Topics)
		}
	}

	var count int64
	err := query.Count(&count).Error
	return count, err
}

// HandleTimeouts resets timed-out processing tasks to pending or failed
func (f *AITaskFacade) HandleTimeouts(ctx context.Context, defaultTimeout time.Duration, maxRetries int) (int, error) {
	db := f.getDB().WithContext(ctx)
	now := time.Now()

	// Find timed-out processing tasks
	var tasks []model.AITask
	err := db.Where("status = ? AND timeout_at < ?", "processing", now).Find(&tasks).Error
	if err != nil {
		return 0, err
	}

	count := 0
	for _, task := range tasks {
		if task.RetryCount < maxRetries {
			// Reset to pending
			err := db.Model(&model.AITask{}).
				Where("id = ?", task.ID).
				Updates(map[string]interface{}{
					"status":      "pending",
					"retry_count": task.RetryCount + 1,
					"agent_id":    "",
					"started_at":  nil,
					"timeout_at":  now.Add(defaultTimeout),
				}).Error
			if err == nil {
				count++
			}
		} else {
			// Mark as failed
			err := db.Model(&model.AITask{}).
				Where("id = ?", task.ID).
				Updates(map[string]interface{}{
					"status":        "failed",
					"error_message": "task timed out after max retries",
					"error_code":    2004, // Timeout error code
					"completed_at":  now,
				}).Error
			if err == nil {
				count++
			}
		}
	}

	return count, nil
}

// Cleanup removes old completed/failed/cancelled tasks
func (f *AITaskFacade) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	db := f.getDB().WithContext(ctx)
	cutoff := time.Now().Add(-olderThan)

	result := db.Where("status IN ? AND completed_at < ?",
		[]string{"completed", "failed", "cancelled"},
		cutoff).
		Delete(&model.AITask{})

	return int(result.RowsAffected), result.Error
}

// deserializeTask deserializes JSON fields
func deserializeTask(task *model.AITask) {
	if task.ContextJSON != "" && task.Context == nil {
		task.Context = make(map[string]interface{})
		json.Unmarshal([]byte(task.ContextJSON), &task.Context)
	}
}

// ErrTaskNotFound is returned when a task is not found
var ErrTaskNotFound = errors.New("task not found")

