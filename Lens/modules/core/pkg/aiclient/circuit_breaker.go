// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aiclient

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Failing, reject requests
	CircuitHalfOpen                     // Testing if recovered
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu               sync.RWMutex
	threshold        int           // Failures before opening
	timeout          time.Duration // Time to wait before half-open
	circuits         map[string]*circuit
	halfOpenMaxCalls int // Max calls in half-open state
}

// circuit tracks state for a single topic
type circuit struct {
	state         CircuitState
	failures      int
	successes     int
	lastFailure   time.Time
	halfOpenCalls int
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &CircuitBreaker{
		threshold:        threshold,
		timeout:          timeout,
		circuits:         make(map[string]*circuit),
		halfOpenMaxCalls: 3,
	}
}

// IsOpen checks if the circuit is open for a topic
func (cb *CircuitBreaker) IsOpen(topic string) bool {
	cb.mu.RLock()
	c, exists := cb.circuits[topic]
	cb.mu.RUnlock()

	if !exists {
		return false
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch c.state {
	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(c.lastFailure) > cb.timeout {
			c.state = CircuitHalfOpen
			c.halfOpenCalls = 0
			return false
		}
		return true
	case CircuitHalfOpen:
		// Allow limited calls in half-open state
		if c.halfOpenCalls >= cb.halfOpenMaxCalls {
			return true
		}
		c.halfOpenCalls++
		return false
	default:
		return false
	}
}

// RecordSuccess records a successful call
func (cb *CircuitBreaker) RecordSuccess(topic string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	c := cb.getOrCreate(topic)
	c.successes++
	c.failures = 0

	// If in half-open state and successful, close the circuit
	if c.state == CircuitHalfOpen {
		if c.successes >= cb.halfOpenMaxCalls {
			c.state = CircuitClosed
			c.halfOpenCalls = 0
		}
	}
}

// RecordFailure records a failed call
func (cb *CircuitBreaker) RecordFailure(topic string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	c := cb.getOrCreate(topic)
	c.failures++
	c.successes = 0
	c.lastFailure = time.Now()

	// Open circuit if threshold reached
	if c.failures >= cb.threshold {
		c.state = CircuitOpen
	}

	// If in half-open state and failed, reopen
	if c.state == CircuitHalfOpen {
		c.state = CircuitOpen
	}
}

// Reset resets the circuit for a topic
func (cb *CircuitBreaker) Reset(topic string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	delete(cb.circuits, topic)
}

// ResetAll resets all circuits
func (cb *CircuitBreaker) ResetAll() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.circuits = make(map[string]*circuit)
}

// GetState returns the current state for a topic
func (cb *CircuitBreaker) GetState(topic string) CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if c, exists := cb.circuits[topic]; exists {
		return c.state
	}
	return CircuitClosed
}

// GetStats returns statistics for a topic
func (cb *CircuitBreaker) GetStats(topic string) (state CircuitState, failures, successes int) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if c, exists := cb.circuits[topic]; exists {
		return c.state, c.failures, c.successes
	}
	return CircuitClosed, 0, 0
}

// getOrCreate gets or creates a circuit for a topic
func (cb *CircuitBreaker) getOrCreate(topic string) *circuit {
	c, exists := cb.circuits[topic]
	if !exists {
		c = &circuit{
			state: CircuitClosed,
		}
		cb.circuits[topic] = c
	}
	return c
}

// AllStats returns statistics for all topics
func (cb *CircuitBreaker) AllStats() map[string]struct {
	State     CircuitState
	Failures  int
	Successes int
} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	stats := make(map[string]struct {
		State     CircuitState
		Failures  int
		Successes int
	})

	for topic, c := range cb.circuits {
		stats[topic] = struct {
			State     CircuitState
			Failures  int
			Successes int
		}{
			State:     c.state,
			Failures:  c.failures,
			Successes: c.successes,
		}
	}

	return stats
}

// StateString returns a string representation of the circuit state
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

