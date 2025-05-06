/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package concurrent

import (
	"math/rand"
	"testing"

	"github.com/pkg/errors"
	"gotest.tools/assert"
)

func TestExec(t *testing.T) {
	biggerErr := errors.New("bigger than 5")
	fn := func(min, max int) func() error {
		return func() error {
			num := rand.Intn(max-min) + min
			if num >= 5 {
				return biggerErr
			}
			return nil
		}
	}

	tests := []struct {
		name          string
		count         int
		fn            func() error
		expectSuccess int
		expectErr     error
	}{
		{"zero", 0, fn(1, 10), 0, nil},
		{"null function", 10, nil, 0, nil},
		{"no err", 10, fn(1, 4), 10, nil},
		{"has err", 10, fn(6, 10), 0, biggerErr},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			success, err := Exec(test.count, test.fn)
			assert.Equal(t, success, test.expectSuccess)
			if test.expectErr == nil {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, test.expectErr, biggerErr.Error())
			}
		})
	}
}
