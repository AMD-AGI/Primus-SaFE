/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package backoff

import (
	"time"

	"github.com/cenkalti/backoff/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func Retry(f backoff.Operation, maxElapsedTime, maxInterval time.Duration) error {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = maxElapsedTime
	b.MaxInterval = maxInterval
	if err := backoff.Retry(f, b); err != nil {
		return err
	}
	return nil
}

func ConflictRetry(f backoff.Operation, count int, interval time.Duration) error {
	for i := 0; i < count; i++ {
		err := f()
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
