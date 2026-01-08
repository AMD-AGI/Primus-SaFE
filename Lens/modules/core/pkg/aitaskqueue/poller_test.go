package aitaskqueue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPollerConfig(t *testing.T) {
	cfg := DefaultPollerConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 100*time.Millisecond, cfg.InitialInterval)
	assert.Equal(t, 5*time.Second, cfg.MaxInterval)
	assert.Equal(t, 1.5, cfg.Multiplier)
	assert.Equal(t, 5*time.Minute, cfg.DefaultTimeout)
}

func TestNewResultPoller(t *testing.T) {
	queue := &MockQueue{}

	t.Run("with nil config", func(t *testing.T) {
		poller := NewResultPoller(queue, nil)
		assert.NotNil(t, poller)
		assert.NotNil(t, poller.config)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &PollerConfig{
			InitialInterval: 50 * time.Millisecond,
			MaxInterval:     2 * time.Second,
			Multiplier:      2.0,
			DefaultTimeout:  1 * time.Minute,
		}
		poller := NewResultPoller(queue, cfg)
		assert.Equal(t, 50*time.Millisecond, poller.config.InitialInterval)
	})
}

func TestResultPoller_WaitForResult_ImmediateComplete(t *testing.T) {
	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{
				ID:     taskID,
				Status: TaskStatusCompleted,
			}, nil
		},
		getResultFunc: func(ctx context.Context, taskID string) (*aitopics.Response, error) {
			return &aitopics.Response{
				RequestID: taskID,
				Status:    aitopics.StatusSuccess,
				Message:   "completed",
			}, nil
		},
	}

	poller := NewResultPoller(queue, &PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     100 * time.Millisecond,
		Multiplier:      1.5,
		DefaultTimeout:  1 * time.Second,
	})

	resp, err := poller.WaitForResult(context.Background(), "task-123")
	require.NoError(t, err)
	assert.Equal(t, "task-123", resp.RequestID)
	assert.Equal(t, aitopics.StatusSuccess, resp.Status)
}

func TestResultPoller_WaitForResult_DelayedComplete(t *testing.T) {
	callCount := 0
	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			callCount++
			if callCount < 3 {
				return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
			}
			return &Task{ID: taskID, Status: TaskStatusCompleted}, nil
		},
		getResultFunc: func(ctx context.Context, taskID string) (*aitopics.Response, error) {
			return &aitopics.Response{RequestID: taskID, Status: aitopics.StatusSuccess}, nil
		},
	}

	poller := NewResultPoller(queue, &PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     100 * time.Millisecond,
		Multiplier:      1.5,
		DefaultTimeout:  1 * time.Second,
	})

	resp, err := poller.WaitForResult(context.Background(), "task-123")
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, callCount >= 3)
}

func TestResultPoller_WaitForResultWithTimeout(t *testing.T) {
	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
		},
	}

	poller := NewResultPoller(queue, &PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      1.5,
		DefaultTimeout:  1 * time.Second,
	})

	_, err := poller.WaitForResultWithTimeout(context.Background(), "task-123", 50*time.Millisecond)
	assert.Equal(t, ErrPollTimeout, err)
}

func TestResultPoller_WaitForResult_ContextCancelled(t *testing.T) {
	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
		},
	}

	poller := NewResultPoller(queue, &PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     100 * time.Millisecond,
		Multiplier:      1.5,
		DefaultTimeout:  1 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	_, err := poller.WaitForResult(ctx, "task-123")
	assert.Equal(t, context.Canceled, err)
}

func TestResultPoller_WaitForResults(t *testing.T) {
	taskStates := map[string]int{
		"task-1": 0,
		"task-2": 0,
		"task-3": 0,
	}

	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			taskStates[taskID]++
			if taskStates[taskID] < 2 {
				return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
			}
			return &Task{ID: taskID, Status: TaskStatusCompleted}, nil
		},
		getResultFunc: func(ctx context.Context, taskID string) (*aitopics.Response, error) {
			return &aitopics.Response{
				RequestID: taskID,
				Status:    aitopics.StatusSuccess,
				Payload:   json.RawMessage(`{"result":"` + taskID + `"}`),
			}, nil
		},
	}

	poller := NewResultPoller(queue, &PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     100 * time.Millisecond,
		Multiplier:      1.5,
		DefaultTimeout:  1 * time.Second,
	})

	results, err := poller.WaitForResults(context.Background(), []string{"task-1", "task-2", "task-3"})
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.NotNil(t, results["task-1"])
	assert.NotNil(t, results["task-2"])
	assert.NotNil(t, results["task-3"])
}

func TestResultPoller_WaitForResultsWithTimeout(t *testing.T) {
	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			if taskID == "task-1" {
				return &Task{ID: taskID, Status: TaskStatusCompleted}, nil
			}
			return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
		},
		getResultFunc: func(ctx context.Context, taskID string) (*aitopics.Response, error) {
			return &aitopics.Response{RequestID: taskID, Status: aitopics.StatusSuccess}, nil
		},
	}

	poller := NewResultPoller(queue, &PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      1.5,
		DefaultTimeout:  1 * time.Second,
	})

	results, err := poller.WaitForResultsWithTimeout(context.Background(), []string{"task-1", "task-2"}, 100*time.Millisecond)
	assert.Equal(t, ErrPollTimeout, err)
	// task-1 should be completed
	assert.Len(t, results, 1)
	assert.NotNil(t, results["task-1"])
}

func TestResultPoller_GetStatus(t *testing.T) {
	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
		},
	}

	poller := NewResultPoller(queue, nil)

	status, err := poller.GetStatus(context.Background(), "task-123")
	require.NoError(t, err)
	assert.Equal(t, TaskStatusProcessing, status)
}

func TestResultPoller_GetStatus_Error(t *testing.T) {
	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return nil, ErrTaskNotFound
		},
	}

	poller := NewResultPoller(queue, nil)

	_, err := poller.GetStatus(context.Background(), "task-123")
	assert.Equal(t, ErrTaskNotFound, err)
}

func TestResultPoller_IsCompleted(t *testing.T) {
	tests := []struct {
		status TaskStatus
		want   bool
	}{
		{TaskStatusPending, false},
		{TaskStatusProcessing, false},
		{TaskStatusCompleted, true},
		{TaskStatusFailed, true},
		{TaskStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			queue := &MockQueue{
				getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
					return &Task{ID: taskID, Status: tt.status}, nil
				},
			}

			poller := NewResultPoller(queue, nil)
			completed, err := poller.IsCompleted(context.Background(), "task-123")
			require.NoError(t, err)
			assert.Equal(t, tt.want, completed)
		})
	}
}

func TestResultPoller_GetProgress(t *testing.T) {
	now := time.Now()
	startedAt := now.Add(-1 * time.Minute)

	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{
				ID:         taskID,
				Status:     TaskStatusProcessing,
				CreatedAt:  now.Add(-2 * time.Minute),
				StartedAt:  &startedAt,
				RetryCount: 1,
			}, nil
		},
	}

	poller := NewResultPoller(queue, nil)

	progress, err := poller.GetProgress(context.Background(), "task-123")
	require.NoError(t, err)
	assert.Equal(t, "task-123", progress.TaskID)
	assert.Equal(t, TaskStatusProcessing, progress.Status)
	assert.NotNil(t, progress.StartedAt)
	assert.True(t, progress.ElapsedTime > 0)
	assert.Equal(t, 1, progress.RetryCount)
}

func TestResultPoller_GetProgress_Completed(t *testing.T) {
	now := time.Now()
	startedAt := now.Add(-2 * time.Minute)
	completedAt := now.Add(-1 * time.Minute)

	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{
				ID:          taskID,
				Status:      TaskStatusCompleted,
				CreatedAt:   now.Add(-3 * time.Minute),
				StartedAt:   &startedAt,
				CompletedAt: &completedAt,
			}, nil
		},
	}

	poller := NewResultPoller(queue, nil)

	progress, err := poller.GetProgress(context.Background(), "task-123")
	require.NoError(t, err)
	assert.Equal(t, 1*time.Minute, progress.ElapsedTime)
}

func TestResultPoller_GetProgress_NotStarted(t *testing.T) {
	now := time.Now()

	queue := &MockQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{
				ID:        taskID,
				Status:    TaskStatusPending,
				CreatedAt: now,
			}, nil
		},
	}

	poller := NewResultPoller(queue, nil)

	progress, err := poller.GetProgress(context.Background(), "task-123")
	require.NoError(t, err)
	assert.Nil(t, progress.StartedAt)
	assert.Equal(t, time.Duration(0), progress.ElapsedTime)
}

func TestPollTimeoutError(t *testing.T) {
	err := &PollTimeoutError{}
	assert.Contains(t, err.Error(), "timed out")
	assert.Equal(t, ErrPollTimeout, err)
}

func TestTaskProgress(t *testing.T) {
	now := time.Now()
	startedAt := now.Add(-1 * time.Minute)

	progress := &TaskProgress{
		TaskID:      "task-123",
		Status:      TaskStatusProcessing,
		CreatedAt:   now.Add(-2 * time.Minute),
		StartedAt:   &startedAt,
		ElapsedTime: 1 * time.Minute,
		RetryCount:  2,
	}

	assert.Equal(t, "task-123", progress.TaskID)
	assert.Equal(t, TaskStatusProcessing, progress.Status)
	assert.NotNil(t, progress.StartedAt)
	assert.Equal(t, 1*time.Minute, progress.ElapsedTime)
	assert.Equal(t, 2, progress.RetryCount)
}

