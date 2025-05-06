/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package concurrent

import (
	"sync"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

func Exec(count int, fn func() error) (int, error) {
	if count == 0 || fn == nil {
		return 0, nil
	}
	var wg sync.WaitGroup
	wg.Add(count)
	errCh := make(chan error, count)
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
		var errList []error
		for l := len(errCh); l > 0; l-- {
			errList = append(errList, <-errCh)
		}
		return successes, utilerrors.NewAggregate(errList)
	}
	return successes, nil
}
