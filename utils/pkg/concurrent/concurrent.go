/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package concurrent

import (
	"sync"
)

// Exec executes the given function concurrently for the specified count.
// It returns the number of successful executions and the first error encountered, if any.
// The function uses goroutines and waits for all of them to complete before returning.
func Exec(count int, fn func() error) (int, error) {
	if count == 0 || fn == nil {
		return 0, nil
	}
	var wg sync.WaitGroup
	wg.Add(count)
	errCh := make(chan error, count)
	defer close(errCh)

	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	successes := count - len(errCh)
	if len(errCh) > 0 {
		err := <-errCh
		return successes, err
	}
	return successes, nil
}
