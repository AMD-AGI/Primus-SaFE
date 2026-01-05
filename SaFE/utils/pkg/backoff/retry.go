/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package backoff

import (
	"time"

	"github.com/cenkalti/backoff/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Retry executes an operation with exponential backoff retry logic.
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
