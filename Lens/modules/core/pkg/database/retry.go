package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// RetryConfig defines the retry configuration
type RetryConfig struct {
	MaxRetries    int           // Maximum number of retry attempts
	InitialDelay  time.Duration // Initial delay before first retry
	MaxDelay      time.Duration // Maximum delay between retries
	DelayMultiple float64       // Delay multiplier for exponential backoff
}

// DefaultRetryConfig provides default retry configuration
var DefaultRetryConfig = RetryConfig{
	MaxRetries:    3,
	InitialDelay:  500 * time.Millisecond,
	MaxDelay:      5 * time.Second,
	DelayMultiple: 2.0,
}

// isRetriableError determines if an error is retriable
func isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()

	// Read-only transaction errors
	readOnlyErrors := []string{
		"cannot execute INSERT in a read-only transaction",
		"cannot execute UPDATE in a read-only transaction",
		"cannot execute DELETE in a read-only transaction",
		"SQLSTATE 25006",
	}

	for _, pattern := range readOnlyErrors {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	// Connection errors
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"i/o timeout",
	}

	for _, pattern := range connectionErrors {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// WithRetry adds automatic retry functionality to database operations
// Example usage:
//
//	err := database.WithRetry(ctx, func() error {
//	    return facade.GetNode().UpdateNode(ctx, node)
//	})
func WithRetry(ctx context.Context, fn func() error) error {
	return WithRetryConfig(ctx, DefaultRetryConfig, fn)
}

// WithRetryConfig performs retry with custom configuration
func WithRetryConfig(ctx context.Context, config RetryConfig, fn func() error) error {
	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		// Execute operation
		err := fn()
		if err == nil {
			if attempt > 0 {
				log.Infof("Operation succeeded after %d retries", attempt)
			}
			return nil
		}

		lastErr = err

		// Check if error is retriable
		if !isRetriableError(err) {
			return err
		}

		// Return error if max retries reached
		if attempt >= config.MaxRetries {
			return fmt.Errorf("max retries (%d) exceeded, last error: %w", config.MaxRetries, lastErr)
		}

		// Log retry attempt
		log.Warnf("Retriable error encountered (attempt %d/%d): %v, retrying in %v...",
			attempt+1, config.MaxRetries, err, delay)

		// Wait before retry
		select {
		case <-time.After(delay):
			// Calculate next delay (exponential backoff)
			delay = time.Duration(float64(delay) * config.DelayMultiple)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry wait: %w", ctx.Err())
		}
	}

	return lastErr
}

// RetryableOperation wraps a function to make it retryable
// Example usage:
//
//	facade := database.GetFacade().GetNode()
//	retryableUpdate := database.RetryableOperation(facade.UpdateNode)
//	err := retryableUpdate(ctx, node)
func RetryableOperation[T any](fn func(context.Context, T) error) func(context.Context, T) error {
	return func(ctx context.Context, arg T) error {
		return WithRetry(ctx, func() error {
			return fn(ctx, arg)
		})
	}
}

// WithRetryAsync executes an operation with retry asynchronously, returns a result channel
// Example usage:
//
//	resultCh := database.WithRetryAsync(ctx, func() error {
//	    return facade.GetNode().UpdateNode(ctx, node)
//	})
//	err := <-resultCh
func WithRetryAsync(ctx context.Context, fn func() error) <-chan error {
	resultCh := make(chan error, 1)
	go func() {
		defer close(resultCh)
		resultCh <- WithRetry(ctx, fn)
	}()
	return resultCh
}
