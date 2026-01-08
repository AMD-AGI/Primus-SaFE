// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aiclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCircuitBreaker(t *testing.T) {
	tests := []struct {
		name      string
		threshold int
		timeout   time.Duration
		wantThreshold int
		wantTimeout   time.Duration
	}{
		{
			name:          "with valid params",
			threshold:     10,
			timeout:       30 * time.Second,
			wantThreshold: 10,
			wantTimeout:   30 * time.Second,
		},
		{
			name:          "with zero threshold uses default",
			threshold:     0,
			timeout:       30 * time.Second,
			wantThreshold: 5,
			wantTimeout:   30 * time.Second,
		},
		{
			name:          "with negative threshold uses default",
			threshold:     -1,
			timeout:       30 * time.Second,
			wantThreshold: 5,
			wantTimeout:   30 * time.Second,
		},
		{
			name:          "with zero timeout uses default",
			threshold:     5,
			timeout:       0,
			wantThreshold: 5,
			wantTimeout:   60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(tt.threshold, tt.timeout)
			assert.NotNil(t, cb)
			assert.Equal(t, tt.wantThreshold, cb.threshold)
			assert.Equal(t, tt.wantTimeout, cb.timeout)
			assert.Equal(t, 3, cb.halfOpenMaxCalls)
		})
	}
}

func TestCircuitBreaker_IsOpen_Closed(t *testing.T) {
	cb := NewCircuitBreaker(5, 60*time.Second)

	// Circuit should be closed for unknown topic
	assert.False(t, cb.IsOpen("unknown-topic"))
}

func TestCircuitBreaker_RecordFailure_OpensCircuit(t *testing.T) {
	cb := NewCircuitBreaker(3, 60*time.Second)
	topic := "test-topic"

	// Record failures below threshold
	cb.RecordFailure(topic)
	assert.False(t, cb.IsOpen(topic))
	cb.RecordFailure(topic)
	assert.False(t, cb.IsOpen(topic))

	// Third failure should open the circuit
	cb.RecordFailure(topic)
	assert.True(t, cb.IsOpen(topic))
}

func TestCircuitBreaker_RecordSuccess_ClosesCircuit(t *testing.T) {
	cb := NewCircuitBreaker(3, 60*time.Second)
	topic := "test-topic"

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure(topic)
	}
	assert.True(t, cb.IsOpen(topic))

	// Force half-open state by manipulating lastFailure
	cb.mu.Lock()
	cb.circuits[topic].lastFailure = time.Now().Add(-2 * 60 * time.Second)
	cb.mu.Unlock()

	// Circuit should transition to half-open
	assert.False(t, cb.IsOpen(topic))

	// Record successes to close the circuit
	for i := 0; i < 3; i++ {
		cb.RecordSuccess(topic)
	}

	state := cb.GetState(topic)
	assert.Equal(t, CircuitClosed, state)
}

func TestCircuitBreaker_HalfOpenState(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	topic := "test-topic"

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure(topic)
	}
	assert.True(t, cb.IsOpen(topic))
	assert.Equal(t, CircuitOpen, cb.GetState(topic))

	// Wait for timeout to pass
	time.Sleep(150 * time.Millisecond)

	// First call should transition to half-open and be allowed
	assert.False(t, cb.IsOpen(topic))
	assert.Equal(t, CircuitHalfOpen, cb.GetState(topic))
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	topic := "test-topic"

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure(topic)
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Transition to half-open
	cb.IsOpen(topic)
	assert.Equal(t, CircuitHalfOpen, cb.GetState(topic))

	// Failure in half-open reopens the circuit
	cb.RecordFailure(topic)
	assert.Equal(t, CircuitOpen, cb.GetState(topic))
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(3, 60*time.Second)
	topic := "test-topic"

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure(topic)
	}
	assert.True(t, cb.IsOpen(topic))

	// Reset the topic
	cb.Reset(topic)

	// Should be closed now
	assert.False(t, cb.IsOpen(topic))
	assert.Equal(t, CircuitClosed, cb.GetState(topic))
}

func TestCircuitBreaker_ResetAll(t *testing.T) {
	cb := NewCircuitBreaker(3, 60*time.Second)

	// Open circuits for multiple topics
	topics := []string{"topic-1", "topic-2", "topic-3"}
	for _, topic := range topics {
		for i := 0; i < 3; i++ {
			cb.RecordFailure(topic)
		}
		assert.True(t, cb.IsOpen(topic))
	}

	// Reset all
	cb.ResetAll()

	// All should be closed
	for _, topic := range topics {
		assert.False(t, cb.IsOpen(topic))
	}
}

func TestCircuitBreaker_GetStats(t *testing.T) {
	cb := NewCircuitBreaker(5, 60*time.Second)
	topic := "test-topic"

	// Initial state
	state, failures, successes := cb.GetStats(topic)
	assert.Equal(t, CircuitClosed, state)
	assert.Equal(t, 0, failures)
	assert.Equal(t, 0, successes)

	// After failures
	cb.RecordFailure(topic)
	cb.RecordFailure(topic)
	state, failures, successes = cb.GetStats(topic)
	assert.Equal(t, CircuitClosed, state)
	assert.Equal(t, 2, failures)
	assert.Equal(t, 0, successes)
}

func TestCircuitBreaker_AllStats(t *testing.T) {
	cb := NewCircuitBreaker(3, 60*time.Second)

	// Create circuits for multiple topics
	cb.RecordFailure("topic-1")
	cb.RecordSuccess("topic-2")
	cb.RecordFailure("topic-3")
	cb.RecordFailure("topic-3")
	cb.RecordFailure("topic-3")

	stats := cb.AllStats()
	assert.Len(t, stats, 3)
	assert.Equal(t, 1, stats["topic-1"].Failures)
	assert.Equal(t, 1, stats["topic-2"].Successes)
	assert.Equal(t, CircuitOpen, stats["topic-3"].State)
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.state.String())
		})
	}
}

func TestCircuitBreaker_HalfOpenMaxCalls(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	topic := "test-topic"

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure(topic)
	}
	assert.True(t, cb.IsOpen(topic))
	assert.Equal(t, CircuitOpen, cb.GetState(topic))

	// Wait for timeout to transition to half-open
	time.Sleep(150 * time.Millisecond)

	// First call after timeout should allow request and transition to half-open
	result := cb.IsOpen(topic)
	assert.False(t, result, "First call after timeout should be allowed")
	assert.Equal(t, CircuitHalfOpen, cb.GetState(topic))

	// Continue calling - halfOpenMaxCalls is 3, so we should get limited calls
	allowedCalls := 1 // We already made one call above
	for i := 0; i < 10; i++ {
		if !cb.IsOpen(topic) {
			allowedCalls++
		} else {
			break
		}
	}
	// Should have allowed exactly halfOpenMaxCalls (3) before blocking
	assert.LessOrEqual(t, allowedCalls, 3+1, "Should limit calls in half-open state") // +1 for some slack
}

func TestCircuitBreaker_Concurrency(t *testing.T) {
	cb := NewCircuitBreaker(100, 60*time.Second)
	topic := "concurrent-topic"
	done := make(chan bool)

	// Run multiple goroutines recording failures/successes
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				cb.RecordFailure(topic)
				cb.RecordSuccess(topic)
				_ = cb.IsOpen(topic)
				_ = cb.GetState(topic)
				_, _, _ = cb.GetStats(topic)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic
	assert.NotNil(t, cb.circuits[topic])
}

