// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitaskqueue

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
)

// ResultPoller provides convenient methods for polling task results
type ResultPoller struct {
	queue  Queue
	config *PollerConfig
}

// PollerConfig contains configuration for the result poller
type PollerConfig struct {
	// Initial poll interval
	InitialInterval time.Duration

	// Maximum poll interval
	MaxInterval time.Duration

	// Interval multiplier
	Multiplier float64

	// Default timeout for polling
	DefaultTimeout time.Duration
}

// DefaultPollerConfig returns default poller configuration
func DefaultPollerConfig() *PollerConfig {
	return &PollerConfig{
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     5 * time.Second,
		Multiplier:      1.5,
		DefaultTimeout:  5 * time.Minute,
	}
}

// NewResultPoller creates a new result poller
func NewResultPoller(queue Queue, config *PollerConfig) *ResultPoller {
	if config == nil {
		config = DefaultPollerConfig()
	}
	return &ResultPoller{
		queue:  queue,
		config: config,
	}
}

// WaitForResult polls for a task result until completion or timeout
func (p *ResultPoller) WaitForResult(ctx context.Context, taskID string) (*aitopics.Response, error) {
	return p.WaitForResultWithTimeout(ctx, taskID, p.config.DefaultTimeout)
}

// WaitForResultWithTimeout polls for a task result with specified timeout
func (p *ResultPoller) WaitForResultWithTimeout(ctx context.Context, taskID string, timeout time.Duration) (*aitopics.Response, error) {
	deadline := time.Now().Add(timeout)
	interval := p.config.InitialInterval

	for {
		// Check deadline
		if time.Now().After(deadline) {
			return nil, ErrPollTimeout
		}

		// Check context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Get task status
		task, err := p.queue.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}

		// Check if completed
		if task.IsCompleted() {
			return p.queue.GetResult(ctx, taskID)
		}

		// Wait before next poll
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		// Increase interval (with cap)
		interval = time.Duration(float64(interval) * p.config.Multiplier)
		if interval > p.config.MaxInterval {
			interval = p.config.MaxInterval
		}
	}
}

// WaitForResults polls for multiple task results
func (p *ResultPoller) WaitForResults(ctx context.Context, taskIDs []string) (map[string]*aitopics.Response, error) {
	return p.WaitForResultsWithTimeout(ctx, taskIDs, p.config.DefaultTimeout)
}

// WaitForResultsWithTimeout polls for multiple task results with specified timeout
func (p *ResultPoller) WaitForResultsWithTimeout(ctx context.Context, taskIDs []string, timeout time.Duration) (map[string]*aitopics.Response, error) {
	deadline := time.Now().Add(timeout)
	results := make(map[string]*aitopics.Response)
	pending := make(map[string]bool)

	for _, id := range taskIDs {
		pending[id] = true
	}

	interval := p.config.InitialInterval

	for len(pending) > 0 {
		// Check deadline
		if time.Now().After(deadline) {
			return results, ErrPollTimeout
		}

		// Check context
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		// Check each pending task
		for taskID := range pending {
			task, err := p.queue.GetTask(ctx, taskID)
			if err != nil {
				continue
			}

			if task.IsCompleted() {
				result, err := p.queue.GetResult(ctx, taskID)
				if err == nil {
					results[taskID] = result
				}
				delete(pending, taskID)
			}
		}

		if len(pending) == 0 {
			break
		}

		// Wait before next poll
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		case <-time.After(interval):
		}

		// Increase interval (with cap)
		interval = time.Duration(float64(interval) * p.config.Multiplier)
		if interval > p.config.MaxInterval {
			interval = p.config.MaxInterval
		}
	}

	return results, nil
}

// GetStatus gets the current status of a task
func (p *ResultPoller) GetStatus(ctx context.Context, taskID string) (TaskStatus, error) {
	task, err := p.queue.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	return task.Status, nil
}

// IsCompleted checks if a task is completed
func (p *ResultPoller) IsCompleted(ctx context.Context, taskID string) (bool, error) {
	task, err := p.queue.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	return task.IsCompleted(), nil
}

// ErrPollTimeout is returned when polling times out
var ErrPollTimeout = &PollTimeoutError{}

// PollTimeoutError represents a polling timeout error
type PollTimeoutError struct{}

func (e *PollTimeoutError) Error() string {
	return "polling timed out waiting for task completion"
}

// TaskProgress represents the progress of a task
type TaskProgress struct {
	TaskID      string
	Status      TaskStatus
	CreatedAt   time.Time
	StartedAt   *time.Time
	ElapsedTime time.Duration
	RetryCount  int
}

// GetProgress gets the progress of a task
func (p *ResultPoller) GetProgress(ctx context.Context, taskID string) (*TaskProgress, error) {
	task, err := p.queue.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	progress := &TaskProgress{
		TaskID:     task.ID,
		Status:     task.Status,
		CreatedAt:  task.CreatedAt,
		StartedAt:  task.StartedAt,
		RetryCount: task.RetryCount,
	}

	if task.StartedAt != nil {
		if task.CompletedAt != nil {
			progress.ElapsedTime = task.CompletedAt.Sub(*task.StartedAt)
		} else {
			progress.ElapsedTime = time.Since(*task.StartedAt)
		}
	}

	return progress, nil
}

