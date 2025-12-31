package aitaskqueue

import (
	"context"
	"time"
)

// CleanupJob handles periodic cleanup of old tasks
type CleanupJob struct {
	queue  Queue
	config *CleanupConfig
	stopCh chan struct{}
	doneCh chan struct{}
}

// CleanupConfig contains configuration for the cleanup job
type CleanupConfig struct {
	// How long to keep completed tasks
	RetentionPeriod time.Duration

	// How often to run cleanup
	Interval time.Duration

	// Batch size for cleanup
	BatchSize int
}

// DefaultCleanupConfig returns default cleanup configuration
func DefaultCleanupConfig() *CleanupConfig {
	return &CleanupConfig{
		RetentionPeriod: 7 * 24 * time.Hour, // 7 days
		Interval:        1 * time.Hour,
		BatchSize:       1000,
	}
}

// NewCleanupJob creates a new cleanup job
func NewCleanupJob(queue Queue, config *CleanupConfig) *CleanupJob {
	if config == nil {
		config = DefaultCleanupConfig()
	}
	return &CleanupJob{
		queue:  queue,
		config: config,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start starts the cleanup job in a goroutine
func (j *CleanupJob) Start(ctx context.Context) {
	go j.run(ctx)
}

// Stop stops the cleanup job
func (j *CleanupJob) Stop() {
	close(j.stopCh)
	<-j.doneCh
}

// run is the main loop for the cleanup job
func (j *CleanupJob) run(ctx context.Context) {
	defer close(j.doneCh)

	ticker := time.NewTicker(j.config.Interval)
	defer ticker.Stop()

	// Run once at start
	j.runCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-j.stopCh:
			return
		case <-ticker.C:
			j.runCleanup(ctx)
		}
	}
}

// runCleanup performs the cleanup
func (j *CleanupJob) runCleanup(ctx context.Context) {
	count, err := j.queue.Cleanup(ctx, j.config.RetentionPeriod)
	if err != nil {
		// Log error but continue
		return
	}
	_ = count // Could log this
}

// RunOnce runs the cleanup once
func (j *CleanupJob) RunOnce(ctx context.Context) (int, error) {
	return j.queue.Cleanup(ctx, j.config.RetentionPeriod)
}

// TimeoutHandler handles task timeout processing
type TimeoutHandler struct {
	queue  Queue
	config *TimeoutConfig
	stopCh chan struct{}
	doneCh chan struct{}
}

// TimeoutConfig contains configuration for the timeout handler
type TimeoutConfig struct {
	// How often to check for timeouts
	CheckInterval time.Duration
}

// DefaultTimeoutConfig returns default timeout configuration
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		CheckInterval: 1 * time.Minute,
	}
}

// NewTimeoutHandler creates a new timeout handler
func NewTimeoutHandler(queue Queue, config *TimeoutConfig) *TimeoutHandler {
	if config == nil {
		config = DefaultTimeoutConfig()
	}
	return &TimeoutHandler{
		queue:  queue,
		config: config,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start starts the timeout handler in a goroutine
func (h *TimeoutHandler) Start(ctx context.Context) {
	go h.run(ctx)
}

// Stop stops the timeout handler
func (h *TimeoutHandler) Stop() {
	close(h.stopCh)
	<-h.doneCh
}

// run is the main loop for the timeout handler
func (h *TimeoutHandler) run(ctx context.Context) {
	defer close(h.doneCh)

	ticker := time.NewTicker(h.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.handleTimeouts(ctx)
		}
	}
}

// handleTimeouts processes timed-out tasks
func (h *TimeoutHandler) handleTimeouts(ctx context.Context) {
	count, err := h.queue.HandleTimeouts(ctx)
	if err != nil {
		// Log error but continue
		return
	}
	_ = count // Could log this
}

// RunOnce runs the timeout handling once
func (h *TimeoutHandler) RunOnce(ctx context.Context) (int, error) {
	return h.queue.HandleTimeouts(ctx)
}

// QueueStats contains statistics about the queue
type QueueStats struct {
	PendingCount    int64
	ProcessingCount int64
	CompletedCount  int64
	FailedCount     int64
	CancelledCount  int64
	TotalCount      int64
}

// GetStats retrieves queue statistics
func GetStats(ctx context.Context, queue Queue) (*QueueStats, error) {
	stats := &QueueStats{}

	pending := TaskStatusPending
	pendingCount, err := queue.CountTasks(ctx, &TaskFilter{Status: &pending})
	if err != nil {
		return nil, err
	}
	stats.PendingCount = pendingCount

	processing := TaskStatusProcessing
	processingCount, err := queue.CountTasks(ctx, &TaskFilter{Status: &processing})
	if err != nil {
		return nil, err
	}
	stats.ProcessingCount = processingCount

	completed := TaskStatusCompleted
	completedCount, err := queue.CountTasks(ctx, &TaskFilter{Status: &completed})
	if err != nil {
		return nil, err
	}
	stats.CompletedCount = completedCount

	failed := TaskStatusFailed
	failedCount, err := queue.CountTasks(ctx, &TaskFilter{Status: &failed})
	if err != nil {
		return nil, err
	}
	stats.FailedCount = failedCount

	cancelled := TaskStatusCancelled
	cancelledCount, err := queue.CountTasks(ctx, &TaskFilter{Status: &cancelled})
	if err != nil {
		return nil, err
	}
	stats.CancelledCount = cancelledCount

	stats.TotalCount = pendingCount + processingCount + completedCount + failedCount + cancelledCount

	return stats, nil
}
