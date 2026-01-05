package aitaskqueue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCleanupConfig(t *testing.T) {
	cfg := DefaultCleanupConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 7*24*time.Hour, cfg.RetentionPeriod)
	assert.Equal(t, 1*time.Hour, cfg.Interval)
	assert.Equal(t, 1000, cfg.BatchSize)
}

func TestNewCleanupJob(t *testing.T) {
	queue := &MockQueue{}

	t.Run("with nil config", func(t *testing.T) {
		job := NewCleanupJob(queue, nil)
		assert.NotNil(t, job)
		assert.NotNil(t, job.config)
		assert.Equal(t, 7*24*time.Hour, job.config.RetentionPeriod)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &CleanupConfig{
			RetentionPeriod: 24 * time.Hour,
			Interval:        30 * time.Minute,
			BatchSize:       500,
		}
		job := NewCleanupJob(queue, cfg)
		assert.Equal(t, 24*time.Hour, job.config.RetentionPeriod)
		assert.Equal(t, 30*time.Minute, job.config.Interval)
	})
}

func TestCleanupJob_RunOnce(t *testing.T) {
	cleanupCalled := false
	cleanupDuration := time.Duration(0)

	queue := &MockQueue{
		cleanupFunc: func(ctx context.Context, olderThan time.Duration) (int, error) {
			cleanupCalled = true
			cleanupDuration = olderThan
			return 10, nil
		},
	}

	job := NewCleanupJob(queue, &CleanupConfig{
		RetentionPeriod: 7 * 24 * time.Hour,
	})

	count, err := job.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.True(t, cleanupCalled)
	assert.Equal(t, 7*24*time.Hour, cleanupDuration)
}

func TestCleanupJob_StartStop(t *testing.T) {
	var cleanupCount atomic.Int32
	queue := &MockQueue{
		cleanupFunc: func(ctx context.Context, olderThan time.Duration) (int, error) {
			cleanupCount.Add(1)
			return 1, nil
		},
	}

	job := NewCleanupJob(queue, &CleanupConfig{
		RetentionPeriod: 24 * time.Hour,
		Interval:        50 * time.Millisecond,
	})

	ctx := context.Background()
	job.Start(ctx)

	// Wait for a few intervals
	time.Sleep(150 * time.Millisecond)

	job.Stop()

	// Should have run at least twice (once at start, once from ticker)
	count := cleanupCount.Load()
	assert.True(t, count >= 2, "Expected at least 2 runs, got %d", count)
}

func TestCleanupJob_ContextCancellation(t *testing.T) {
	var cleanupCount atomic.Int32
	queue := &MockQueue{
		cleanupFunc: func(ctx context.Context, olderThan time.Duration) (int, error) {
			cleanupCount.Add(1)
			return 1, nil
		},
	}

	job := NewCleanupJob(queue, &CleanupConfig{
		RetentionPeriod: 24 * time.Hour,
		Interval:        50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	job.Start(ctx)

	// Wait a bit
	time.Sleep(75 * time.Millisecond)

	// Cancel context instead of Stop
	cancel()

	// Give it time to exit
	time.Sleep(50 * time.Millisecond)

	// Should have run
	assert.True(t, cleanupCount.Load() >= 1)
}

func TestCleanupJob_CleanupError(t *testing.T) {
	queue := &MockQueue{
		cleanupFunc: func(ctx context.Context, olderThan time.Duration) (int, error) {
			return 0, assert.AnError
		},
	}

	job := NewCleanupJob(queue, &CleanupConfig{
		RetentionPeriod: 24 * time.Hour,
		Interval:        1 * time.Hour,
	})

	// Should not panic on error
	job.runCleanup(context.Background())
}

func TestDefaultTimeoutConfig(t *testing.T) {
	cfg := DefaultTimeoutConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 1*time.Minute, cfg.CheckInterval)
}

func TestNewTimeoutHandler(t *testing.T) {
	queue := &MockQueue{}

	t.Run("with nil config", func(t *testing.T) {
		handler := NewTimeoutHandler(queue, nil)
		assert.NotNil(t, handler)
		assert.NotNil(t, handler.config)
		assert.Equal(t, 1*time.Minute, handler.config.CheckInterval)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &TimeoutConfig{
			CheckInterval: 30 * time.Second,
		}
		handler := NewTimeoutHandler(queue, cfg)
		assert.Equal(t, 30*time.Second, handler.config.CheckInterval)
	})
}

func TestTimeoutHandler_RunOnce(t *testing.T) {
	handleTimeoutsCalled := false
	queue := &MockQueue{
		handleTimeoutsFunc: func(ctx context.Context) (int, error) {
			handleTimeoutsCalled = true
			return 5, nil
		},
	}

	handler := NewTimeoutHandler(queue, nil)

	count, err := handler.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 5, count)
	assert.True(t, handleTimeoutsCalled)
}

func TestTimeoutHandler_StartStop(t *testing.T) {
	var timeoutCount atomic.Int32
	queue := &MockQueue{
		handleTimeoutsFunc: func(ctx context.Context) (int, error) {
			timeoutCount.Add(1)
			return 1, nil
		},
	}

	handler := NewTimeoutHandler(queue, &TimeoutConfig{
		CheckInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	handler.Start(ctx)

	// Wait for a few intervals
	time.Sleep(150 * time.Millisecond)

	handler.Stop()

	// Should have run at least once from ticker
	count := timeoutCount.Load()
	assert.True(t, count >= 1, "Expected at least 1 run, got %d", count)
}

func TestTimeoutHandler_HandleTimeoutsError(t *testing.T) {
	queue := &MockQueue{
		handleTimeoutsFunc: func(ctx context.Context) (int, error) {
			return 0, assert.AnError
		},
	}

	handler := NewTimeoutHandler(queue, nil)

	// Should not panic on error
	handler.handleTimeouts(context.Background())
}

func TestGetStats(t *testing.T) {
	queue := &MockQueue{
		countTasksFunc: func(ctx context.Context, filter *TaskFilter) (int64, error) {
			switch *filter.Status {
			case TaskStatusPending:
				return 10, nil
			case TaskStatusProcessing:
				return 5, nil
			case TaskStatusCompleted:
				return 100, nil
			case TaskStatusFailed:
				return 3, nil
			case TaskStatusCancelled:
				return 2, nil
			}
			return 0, nil
		},
	}

	stats, err := GetStats(context.Background(), queue)
	require.NoError(t, err)
	assert.Equal(t, int64(10), stats.PendingCount)
	assert.Equal(t, int64(5), stats.ProcessingCount)
	assert.Equal(t, int64(100), stats.CompletedCount)
	assert.Equal(t, int64(3), stats.FailedCount)
	assert.Equal(t, int64(2), stats.CancelledCount)
	assert.Equal(t, int64(120), stats.TotalCount)
}

func TestGetStats_Error(t *testing.T) {
	queue := &MockQueue{
		countTasksFunc: func(ctx context.Context, filter *TaskFilter) (int64, error) {
			return 0, assert.AnError
		},
	}

	_, err := GetStats(context.Background(), queue)
	assert.Error(t, err)
}

func TestQueueStats(t *testing.T) {
	stats := &QueueStats{
		PendingCount:    10,
		ProcessingCount: 5,
		CompletedCount:  100,
		FailedCount:     3,
		CancelledCount:  2,
		TotalCount:      120,
	}

	assert.Equal(t, int64(10), stats.PendingCount)
	assert.Equal(t, int64(5), stats.ProcessingCount)
	assert.Equal(t, int64(100), stats.CompletedCount)
	assert.Equal(t, int64(3), stats.FailedCount)
	assert.Equal(t, int64(2), stats.CancelledCount)
	assert.Equal(t, int64(120), stats.TotalCount)
}

func TestCleanupConfig(t *testing.T) {
	cfg := &CleanupConfig{
		RetentionPeriod: 14 * 24 * time.Hour,
		Interval:        2 * time.Hour,
		BatchSize:       2000,
	}

	assert.Equal(t, 14*24*time.Hour, cfg.RetentionPeriod)
	assert.Equal(t, 2*time.Hour, cfg.Interval)
	assert.Equal(t, 2000, cfg.BatchSize)
}

func TestTimeoutConfig(t *testing.T) {
	cfg := &TimeoutConfig{
		CheckInterval: 5 * time.Minute,
	}

	assert.Equal(t, 5*time.Minute, cfg.CheckInterval)
}
