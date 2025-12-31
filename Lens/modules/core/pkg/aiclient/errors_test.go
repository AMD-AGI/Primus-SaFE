package aiclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"ErrAgentUnavailable", ErrAgentUnavailable, true},
		{"ErrTimeout", ErrTimeout, true},
		{"ErrConnectionFailed", ErrConnectionFailed, true},
		{"ErrRateLimited", ErrRateLimited, true},
		{"ErrInvalidRequest", ErrInvalidRequest, false},
		{"ErrUnauthorized", ErrUnauthorized, false},
		{"ErrNoAgentForTopic", ErrNoAgentForTopic, false},
		{"ErrCircuitBreakerOpen", ErrCircuitBreakerOpen, false},
		{"ErrTaskNotFound", ErrTaskNotFound, false},
		{"ErrDegradationApplied", ErrDegradationApplied, false},
		{"generic error", errors.New("some error"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryableError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsFatalError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"ErrInvalidRequest", ErrInvalidRequest, true},
		{"ErrUnauthorized", ErrUnauthorized, true},
		{"ErrNoAgentForTopic", ErrNoAgentForTopic, true},
		{"ErrTaskQueueNotConfigured", ErrTaskQueueNotConfigured, true},
		{"ErrAgentUnavailable", ErrAgentUnavailable, false},
		{"ErrTimeout", ErrTimeout, false},
		{"ErrConnectionFailed", ErrConnectionFailed, false},
		{"ErrCircuitBreakerOpen", ErrCircuitBreakerOpen, false},
		{"generic error", errors.New("some error"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsFatalError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsCircuitBreakerError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"ErrCircuitBreakerOpen", ErrCircuitBreakerOpen, true},
		{"ErrAgentUnavailable", ErrAgentUnavailable, false},
		{"ErrTimeout", ErrTimeout, false},
		{"generic error", errors.New("some error"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCircuitBreakerError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsDegradationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"ErrDegradationApplied", ErrDegradationApplied, true},
		{"ErrAgentUnavailable", ErrAgentUnavailable, false},
		{"ErrTimeout", ErrTimeout, false},
		{"generic error", errors.New("some error"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDegradationError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestErrorMessages(t *testing.T) {
	// Verify error messages are meaningful
	assert.Contains(t, ErrTaskQueueNotConfigured.Error(), "task queue")
	assert.Contains(t, ErrAgentUnavailable.Error(), "unavailable")
	assert.Contains(t, ErrNoAgentForTopic.Error(), "topic")
	assert.Contains(t, ErrTimeout.Error(), "timeout")
	assert.Contains(t, ErrConnectionFailed.Error(), "connection")
	assert.Contains(t, ErrInvalidRequest.Error(), "invalid")
	assert.Contains(t, ErrInvalidResponse.Error(), "invalid")
	assert.Contains(t, ErrUnauthorized.Error(), "unauthorized")
	assert.Contains(t, ErrRateLimited.Error(), "rate")
	assert.Contains(t, ErrCircuitBreakerOpen.Error(), "circuit breaker")
	assert.Contains(t, ErrTaskNotFound.Error(), "task")
	assert.Contains(t, ErrTaskCancelled.Error(), "cancelled")
	assert.Contains(t, ErrTaskFailed.Error(), "failed")
	assert.Contains(t, ErrDegradationApplied.Error(), "degradation")
}

func TestWrappedErrors(t *testing.T) {
	// Test that errors.Is works correctly
	wrappedErr := errors.New("wrapped: " + ErrTimeout.Error())

	// Direct comparison should not work for wrapped errors
	assert.False(t, errors.Is(wrappedErr, ErrTimeout))

	// Direct errors.Is should work
	assert.True(t, errors.Is(ErrTimeout, ErrTimeout))
	assert.True(t, errors.Is(ErrAgentUnavailable, ErrAgentUnavailable))
}
