package aitaskqueue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/google/uuid"
)

// PGStore implements Queue using database facade
type PGStore struct {
	facade      database.AITaskFacadeInterface
	config      *QueueConfig
	clusterName string
}

// NewPGStore creates a new PostgreSQL-backed queue
func NewPGStore(clusterName string, config *QueueConfig) *PGStore {
	if config == nil {
		config = DefaultQueueConfig()
	}

	facade := database.NewAITaskFacade()
	if clusterName != "" {
		facade = facade.WithCluster(clusterName)
	}

	return &PGStore{
		facade:      facade,
		config:      config,
		clusterName: clusterName,
	}
}

// NewPGStoreWithFacade creates a new PostgreSQL-backed queue with a custom facade
func NewPGStoreWithFacade(facade database.AITaskFacadeInterface, config *QueueConfig) *PGStore {
	if config == nil {
		config = DefaultQueueConfig()
	}
	return &PGStore{
		facade: facade,
		config: config,
	}
}

// Publish adds a new task to the queue
func (s *PGStore) Publish(ctx context.Context, topic string, payload json.RawMessage, reqCtx aitopics.RequestContext) (string, error) {
	return s.PublishWithOptions(ctx, &PublishOptions{
		Topic:      topic,
		Payload:    payload,
		Context:    reqCtx,
		Priority:   0,
		MaxRetries: s.config.DefaultMaxRetries,
		Timeout:    s.config.DefaultTimeout,
	})
}

// PublishWithOptions adds a new task with options
func (s *PGStore) PublishWithOptions(ctx context.Context, opts *PublishOptions) (string, error) {
	taskID := uuid.New().String()
	now := time.Now()

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = s.config.DefaultTimeout
	}

	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = s.config.DefaultMaxRetries
	}

	// Serialize context
	contextJSON := "{}"
	if opts.Context.ClusterID != "" || opts.Context.TenantID != "" {
		contextBytes, err := json.Marshal(opts.Context)
		if err != nil {
			return "", err
		}
		contextJSON = string(contextBytes)
	}

	dbTask := &model.AITask{
		ID:           taskID,
		Topic:        opts.Topic,
		Status:       string(TaskStatusPending),
		Priority:     opts.Priority,
		InputPayload: opts.Payload,
		MaxRetries:   maxRetries,
		ContextJSON:  contextJSON,
		CreatedAt:    now,
		TimeoutAt:    now.Add(timeout),
	}

	err := s.facade.Create(ctx, dbTask)
	if err != nil {
		return "", err
	}

	return taskID, nil
}

// GetTask retrieves a task by ID
func (s *PGStore) GetTask(ctx context.Context, taskID string) (*Task, error) {
	dbTask, err := s.facade.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if dbTask == nil {
		return nil, ErrTaskNotFound
	}
	return s.fromDBModel(dbTask), nil
}

// GetResult retrieves the result of a completed task
func (s *PGStore) GetResult(ctx context.Context, taskID string) (*aitopics.Response, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	switch task.Status {
	case TaskStatusCompleted:
		var completedAt time.Time
		if task.CompletedAt != nil {
			completedAt = *task.CompletedAt
		}
		return &aitopics.Response{
			RequestID: taskID,
			Status:    aitopics.StatusSuccess,
			Code:      aitopics.CodeSuccess,
			Message:   "success",
			Timestamp: completedAt,
			Payload:   task.OutputPayload,
		}, nil
	case TaskStatusFailed:
		return &aitopics.Response{
			RequestID: taskID,
			Status:    aitopics.StatusError,
			Code:      task.ErrorCode,
			Message:   task.ErrorMessage,
		}, nil
	case TaskStatusCancelled:
		return &aitopics.Response{
			RequestID: taskID,
			Status:    aitopics.StatusError,
			Code:      aitopics.CodeInternalError,
			Message:   "task was cancelled",
		}, nil
	default:
		return nil, ErrTaskNotCompleted
	}
}

// ClaimTask claims a pending task for processing
func (s *PGStore) ClaimTask(ctx context.Context, topics []string, agentID string) (*Task, error) {
	dbTask, err := s.facade.ClaimTask(ctx, topics, agentID)
	if err != nil {
		return nil, err
	}
	if dbTask == nil {
		return nil, nil // No pending tasks
	}
	return s.fromDBModel(dbTask), nil
}

// CompleteTask marks a task as completed with result
func (s *PGStore) CompleteTask(ctx context.Context, taskID string, result *aitopics.Response) error {
	err := s.facade.Complete(ctx, taskID, result.Payload)
	if err == database.ErrTaskNotFound {
		return ErrTaskNotFound
	}
	return err
}

// FailTask marks a task as failed
func (s *PGStore) FailTask(ctx context.Context, taskID string, errorCode int, errorMsg string) error {
	// Get current task to check retry count
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	newRetryCount := task.RetryCount + 1
	return s.facade.Fail(ctx, taskID, errorCode, errorMsg, newRetryCount, task.MaxRetries)
}

// CancelTask cancels a pending task
func (s *PGStore) CancelTask(ctx context.Context, taskID string) error {
	err := s.facade.Cancel(ctx, taskID)
	if err == database.ErrTaskNotFound {
		return ErrTaskNotFound
	}
	return err
}

// ListTasks lists tasks with optional filters
func (s *PGStore) ListTasks(ctx context.Context, filter *TaskFilter) ([]*Task, error) {
	dbFilter := s.toDBFilter(filter)
	dbTasks, err := s.facade.List(ctx, dbFilter)
	if err != nil {
		return nil, err
	}

	result := make([]*Task, len(dbTasks))
	for i, dbTask := range dbTasks {
		result[i] = s.fromDBModel(dbTask)
	}
	return result, nil
}

// CountTasks counts tasks by status
func (s *PGStore) CountTasks(ctx context.Context, filter *TaskFilter) (int64, error) {
	dbFilter := s.toDBFilter(filter)
	return s.facade.Count(ctx, dbFilter)
}

// HandleTimeouts moves timed-out tasks back to pending
func (s *PGStore) HandleTimeouts(ctx context.Context) (int, error) {
	return s.facade.HandleTimeouts(ctx, s.config.DefaultTimeout, s.config.DefaultMaxRetries)
}

// Cleanup removes old completed tasks
func (s *PGStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	return s.facade.Cleanup(ctx, olderThan)
}

// fromDBModel converts database model to Task
func (s *PGStore) fromDBModel(dbTask *model.AITask) *Task {
	var context aitopics.RequestContext
	if dbTask.Context != nil {
		if clusterID, ok := dbTask.Context["cluster_id"].(string); ok {
			context.ClusterID = clusterID
		}
		if tenantID, ok := dbTask.Context["tenant_id"].(string); ok {
			context.TenantID = tenantID
		}
		if traceID, ok := dbTask.Context["trace_id"].(string); ok {
			context.TraceID = traceID
		}
	}

	return &Task{
		ID:            dbTask.ID,
		Topic:         dbTask.Topic,
		Status:        TaskStatus(dbTask.Status),
		Priority:      dbTask.Priority,
		InputPayload:  dbTask.InputPayload,
		OutputPayload: dbTask.OutputPayload,
		ErrorMessage:  dbTask.ErrorMessage,
		ErrorCode:     dbTask.ErrorCode,
		RetryCount:    dbTask.RetryCount,
		MaxRetries:    dbTask.MaxRetries,
		AgentID:       dbTask.AgentID,
		Context:       context,
		CreatedAt:     dbTask.CreatedAt,
		StartedAt:     dbTask.StartedAt,
		CompletedAt:   dbTask.CompletedAt,
		TimeoutAt:     dbTask.TimeoutAt,
	}
}

// toDBFilter converts TaskFilter to database filter
func (s *PGStore) toDBFilter(filter *TaskFilter) *database.AITaskFilter {
	if filter == nil {
		return nil
	}

	dbFilter := &database.AITaskFilter{
		Topics:        filter.Topics,
		Limit:         filter.Limit,
		Offset:        filter.Offset,
		CreatedAfter:  filter.CreatedAfter,
		CreatedBefore: filter.CreatedBefore,
	}

	if filter.Status != nil {
		statusStr := string(*filter.Status)
		dbFilter.Status = &statusStr
	}
	if filter.Topic != "" {
		dbFilter.Topic = &filter.Topic
	}
	if filter.AgentID != "" {
		dbFilter.AgentID = &filter.AgentID
	}

	return dbFilter
}
