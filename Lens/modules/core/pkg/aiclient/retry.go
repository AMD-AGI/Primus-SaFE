package aiclient

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// RetryConfig contains retry configuration
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including initial)
	MaxAttempts int

	// InitialDelay is the delay before the first retry
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration

	// Multiplier is the factor by which delay increases
	Multiplier float64

	// Jitter adds randomness to delays (0-1, fraction of delay)
	Jitter float64
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
	}
}

// Retrier handles retry logic
type Retrier struct {
	config *RetryConfig
}

// NewRetrier creates a new retrier
func NewRetrier(config *RetryConfig) *Retrier {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &Retrier{config: config}
}

// RetryableFunc is a function that can be retried
type RetryableFunc func(ctx context.Context, attempt int) error

// Do executes the function with retry logic
func (r *Retrier) Do(ctx context.Context, fn RetryableFunc) error {
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		// Wait before retry (not on first attempt)
		if attempt > 0 {
			delay := r.calculateDelay(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := fn(ctx, attempt)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryableError(err) {
			return err
		}

		// Check if we should continue
		if attempt >= r.config.MaxAttempts-1 {
			break
		}
	}

	return lastErr
}

// calculateDelay calculates the delay for a given attempt
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt-1))

	// Apply max delay cap
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// Apply jitter
	if r.config.Jitter > 0 {
		jitter := delay * r.config.Jitter * (rand.Float64()*2 - 1)
		delay += jitter
	}

	return time.Duration(delay)
}

// DoWithResult executes a function that returns a result with retry logic
func DoWithResult[T any](ctx context.Context, r *Retrier, fn func(ctx context.Context, attempt int) (T, error)) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		// Wait before retry (not on first attempt)
		if attempt > 0 {
			delay := r.calculateDelay(attempt)
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(delay):
			}
		}

		var err error
		result, err = fn(ctx, attempt)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryableError(err) {
			return result, err
		}
	}

	return result, lastErr
}

// RetryPolicy defines when to retry
type RetryPolicy interface {
	ShouldRetry(err error, attempt int) bool
	GetDelay(attempt int) time.Duration
}

// ExponentialBackoff implements RetryPolicy with exponential backoff
type ExponentialBackoff struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	MaxAttempts  int
}

// ShouldRetry returns true if the operation should be retried
func (e *ExponentialBackoff) ShouldRetry(err error, attempt int) bool {
	if attempt >= e.MaxAttempts {
		return false
	}
	return IsRetryableError(err)
}

// GetDelay returns the delay before the next retry
func (e *ExponentialBackoff) GetDelay(attempt int) time.Duration {
	delay := float64(e.InitialDelay) * math.Pow(e.Multiplier, float64(attempt))
	if delay > float64(e.MaxDelay) {
		delay = float64(e.MaxDelay)
	}
	return time.Duration(delay)
}
