package aiclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 3, cfg.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, cfg.InitialDelay)
	assert.Equal(t, 10*time.Second, cfg.MaxDelay)
	assert.Equal(t, 2.0, cfg.Multiplier)
	assert.Equal(t, 0.1, cfg.Jitter)
}

func TestNewRetrier(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		r := NewRetrier(nil)
		assert.NotNil(t, r)
		assert.Equal(t, 3, r.config.MaxAttempts)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &RetryConfig{
			MaxAttempts:  5,
			InitialDelay: 200 * time.Millisecond,
			MaxDelay:     5 * time.Second,
			Multiplier:   1.5,
			Jitter:       0.2,
		}
		r := NewRetrier(cfg)
		assert.Equal(t, 5, r.config.MaxAttempts)
		assert.Equal(t, 200*time.Millisecond, r.config.InitialDelay)
	})
}

func TestRetrier_Do_Success(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, attempts)
}

func TestRetrier_Do_RetryOnRetryableError(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		if attempts < 3 {
			return ErrTimeout // Retryable error
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestRetrier_Do_NoRetryOnFatalError(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return ErrInvalidRequest // Non-retryable error
	})

	assert.Equal(t, ErrInvalidRequest, err)
	assert.Equal(t, 1, attempts)
}

func TestRetrier_Do_MaxAttemptsReached(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return ErrAgentUnavailable // Retryable error
	})

	assert.Equal(t, ErrAgentUnavailable, err)
	assert.Equal(t, 3, attempts)
}

func TestRetrier_Do_ContextCancelled(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	})

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := r.Do(ctx, func(ctx context.Context, attempt int) error {
		attempts++
		return ErrTimeout // Retryable error
	})

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestRetrier_calculateDelay(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       0, // No jitter for predictable tests
	})

	// Test exponential backoff
	delay1 := r.calculateDelay(1)
	delay2 := r.calculateDelay(2)
	delay3 := r.calculateDelay(3)

	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 200*time.Millisecond, delay2)
	assert.Equal(t, 400*time.Millisecond, delay3)
}

func TestRetrier_calculateDelay_MaxDelayCap(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	})

	// After several attempts, should be capped at MaxDelay
	delay := r.calculateDelay(10)
	assert.Equal(t, 500*time.Millisecond, delay)
}

func TestRetrier_calculateDelay_WithJitter(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1, // 10% jitter
	})

	// Get multiple delays and verify they have some variation
	delays := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		delays[i] = r.calculateDelay(2)
	}

	// With jitter, not all delays should be exactly the same
	baseDelay := 200 * time.Millisecond
	minExpected := time.Duration(float64(baseDelay) * 0.9)
	maxExpected := time.Duration(float64(baseDelay) * 1.1)

	for _, d := range delays {
		assert.True(t, d >= minExpected && d <= maxExpected,
			"delay %v not in expected range [%v, %v]", d, minExpected, maxExpected)
	}
}

func TestDoWithResult_Success(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	result, err := DoWithResult[string](context.Background(), r, func(ctx context.Context, attempt int) (string, error) {
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestDoWithResult_RetryAndSuccess(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	attempts := 0
	result, err := DoWithResult[int](context.Background(), r, func(ctx context.Context, attempt int) (int, error) {
		attempts++
		if attempts < 3 {
			return 0, ErrConnectionFailed
		}
		return 42, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 42, result)
	assert.Equal(t, 3, attempts)
}

func TestDoWithResult_MaxAttemptsReached(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	result, err := DoWithResult[string](context.Background(), r, func(ctx context.Context, attempt int) (string, error) {
		return "", ErrAgentUnavailable
	})

	assert.Equal(t, ErrAgentUnavailable, err)
	assert.Equal(t, "", result)
}

func TestDoWithResult_ContextCancelled(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := DoWithResult[string](ctx, r, func(ctx context.Context, attempt int) (string, error) {
		return "", ErrTimeout
	})

	assert.Error(t, err)
	assert.Equal(t, "", result)
}

func TestExponentialBackoff_ShouldRetry(t *testing.T) {
	eb := &ExponentialBackoff{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		MaxAttempts:  3,
	}

	// Retryable errors within max attempts
	assert.True(t, eb.ShouldRetry(ErrAgentUnavailable, 0))
	assert.True(t, eb.ShouldRetry(ErrTimeout, 1))
	assert.True(t, eb.ShouldRetry(ErrConnectionFailed, 2))

	// Non-retryable error
	assert.False(t, eb.ShouldRetry(ErrInvalidRequest, 0))

	// Max attempts reached
	assert.False(t, eb.ShouldRetry(ErrAgentUnavailable, 3))
	assert.False(t, eb.ShouldRetry(ErrTimeout, 5))
}

func TestExponentialBackoff_GetDelay(t *testing.T) {
	eb := &ExponentialBackoff{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2.0,
		MaxAttempts:  5,
	}

	assert.Equal(t, 100*time.Millisecond, eb.GetDelay(0))
	assert.Equal(t, 200*time.Millisecond, eb.GetDelay(1))
	assert.Equal(t, 400*time.Millisecond, eb.GetDelay(2))
	assert.Equal(t, 500*time.Millisecond, eb.GetDelay(3)) // Capped at MaxDelay
	assert.Equal(t, 500*time.Millisecond, eb.GetDelay(10)) // Still capped
}

func TestRetryPolicy_Interface(t *testing.T) {
	// Ensure ExponentialBackoff implements RetryPolicy
	var _ RetryPolicy = &ExponentialBackoff{}
}

func TestRetrier_AttemptParameter(t *testing.T) {
	r := NewRetrier(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	observedAttempts := []int{}
	err := r.Do(context.Background(), func(ctx context.Context, attempt int) error {
		observedAttempts = append(observedAttempts, attempt)
		if attempt < 2 {
			return ErrTimeout
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2}, observedAttempts)
}

