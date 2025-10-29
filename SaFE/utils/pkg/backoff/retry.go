/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package backoff

import (
	"time"

	"github.com/cenkalti/backoff/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Retry executes an operation with exponential backoff retry logic.
// It uses the backoff library to retry the operation with exponential backoff intervals
// until the operation succeeds or the maximum elapsed time is reached.
//
// Parameters:
//   - op: The operation function to execute, which should return an error
//   - maxElapsedTime: Maximum total time to spend retrying before giving up
//   - maxInterval: Maximum interval between retry attempts
//
// Returns:
//   - error: The last error returned by the operation, or nil if operation succeeded
func Retry(op backoff.Operation, maxElapsedTime, maxInterval time.Duration) error {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = maxElapsedTime
	b.MaxInterval = maxInterval
	if err := backoff.Retry(op, b); err != nil {
		return err
	}
	return nil
}

// ConflictRetry executes an operation with fixed-interval retry logic specifically for conflict errors.
// It retries the operation a fixed number of times with a fixed interval between attempts,
// but only continues retrying if the error is a conflict error (apierrors.IsConflict).
// Non-conflict errors or reaching the maximum retry count will stop the retry loop.
//
// Parameters:
//   - op: The operation function to execute, which should return an error
//   - count: Maximum number of retry attempts
//   - interval: Fixed time interval to wait between retry attempts
//
// Returns:
//   - error: The last error returned by the operation, or nil if operation succeeded
func ConflictRetry(op backoff.Operation, count int, interval time.Duration) error {
	for i := 0; i < count; i++ {
		err := op()
		if err == nil {
			break
		}
		if i == count-1 || !apierrors.IsConflict(err) {
			return err
		}
		time.Sleep(interval)
	}
	return nil
}
