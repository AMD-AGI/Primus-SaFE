package aiclient

import (
	"errors"
	"fmt"
)

// Common errors for AI client
var (
	// Configuration errors
	ErrTaskQueueNotConfigured = errors.New("task queue not configured")

	// Agent errors
	ErrAgentUnavailable = errors.New("agent unavailable")
	ErrNoAgentForTopic  = errors.New("no agent registered for topic")

	// Invocation errors
	ErrTimeout          = errors.New("request timeout")
	ErrConnectionFailed = errors.New("connection to agent failed")
	ErrInvalidRequest   = errors.New("invalid request")
	ErrInvalidResponse  = errors.New("invalid response from agent")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrRateLimited      = errors.New("rate limited")

	// Circuit breaker errors
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

	// Task errors
	ErrTaskNotFound  = errors.New("task not found")
	ErrTaskCancelled = errors.New("task was cancelled")
	ErrTaskFailed    = errors.New("task failed")

	// Degradation errors
	ErrDegradationApplied = errors.New("degradation applied, AI features unavailable")
)

// APIError represents an error returned from the AI API
type APIError struct {
	Code    int
	Message string
}

// Error implements the error interface
func (e *APIError) Error() string {
	return fmt.Sprintf("AI API error (code=%d): %s", e.Code, e.Message)
}

// NewAPIError creates a new APIError
func NewAPIError(code int, message string) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
	}
}

// IsRetryableError returns true if the error is retryable
func IsRetryableError(err error) bool {
	switch {
	case errors.Is(err, ErrAgentUnavailable):
		return true
	case errors.Is(err, ErrTimeout):
		return true
	case errors.Is(err, ErrConnectionFailed):
		return true
	case errors.Is(err, ErrRateLimited):
		return true
	default:
		return false
	}
}

// IsFatalError returns true if the error should not be retried
func IsFatalError(err error) bool {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return true
	case errors.Is(err, ErrUnauthorized):
		return true
	case errors.Is(err, ErrNoAgentForTopic):
		return true
	case errors.Is(err, ErrTaskQueueNotConfigured):
		return true
	default:
		return false
	}
}

// IsCircuitBreakerError returns true if the error is due to circuit breaker
func IsCircuitBreakerError(err error) bool {
	return errors.Is(err, ErrCircuitBreakerOpen)
}

// IsDegradationError returns true if degradation was applied
func IsDegradationError(err error) bool {
	return errors.Is(err, ErrDegradationApplied)
}

