/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package backoff

import (
	"errors"
	"testing"
	"time"

	"gotest.tools/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestRetry verifies Retry runs the operation and returns the final result.
func TestRetry(t *testing.T) {
	// operation succeeds immediately
	count := 0
	err := Retry(func() error {
		count++
		return nil
	}, time.Second, 100*time.Millisecond)
	assert.NilError(t, err)
	assert.Equal(t, count, 1)

	// operation keeps failing until max elapsed time is exceeded
	err = Retry(func() error {
		return errors.New("boom")
	}, 50*time.Millisecond, 10*time.Millisecond)
	assert.Assert(t, err != nil)
}

// TestConflictRetry verifies the conflict-specific retry behavior.
func TestConflictRetry(t *testing.T) {
	// operation succeeds on the first try
	err := ConflictRetry(func() error { return nil }, 3, time.Millisecond)
	assert.NilError(t, err)

	// conflict error is retried until the attempt count is exhausted
	conflictAttempts := 0
	err = ConflictRetry(func() error {
		conflictAttempts++
		return apierrors.NewConflict(schema.GroupResource{Resource: "pods"}, "p", errors.New("conflict"))
	}, 3, time.Millisecond)
	assert.Assert(t, err != nil)
	assert.Equal(t, conflictAttempts, 3)

	// non-conflict error returns immediately without retry
	nonConflictAttempts := 0
	err = ConflictRetry(func() error {
		nonConflictAttempts++
		return errors.New("boom")
	}, 3, time.Millisecond)
	assert.Assert(t, err != nil)
	assert.Equal(t, nonConflictAttempts, 1)
}
