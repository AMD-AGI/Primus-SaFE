// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package goroutineUtil

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecoverFunc(t *testing.T) {
	t.Run("normal execution does not trigger recovery", func(t *testing.T) {
		hookCalled := false
		hook := func(r any) {
			hookCalled = true
		}

		func() {
			defer RecoverFunc(hook)()
			// normal execution, no panic
		}()

		assert.False(t, hookCalled)
	})

	t.Run("hook triggered on panic", func(t *testing.T) {
		hookCalled := false
		var panicValue any
		hook := func(r any) {
			hookCalled = true
			panicValue = r
		}

		func() {
			defer RecoverFunc(hook)()
			panic("test panic")
		}()

		assert.True(t, hookCalled)
		assert.Equal(t, "test panic", panicValue)
	})

	t.Run("no panic when hook is nil", func(t *testing.T) {
		assert.NotPanics(t, func() {
			defer RecoverFunc(nil)()
			panic("test panic")
		})
	})

	t.Run("panic error type", func(t *testing.T) {
		var panicValue any
		hook := func(r any) {
			panicValue = r
		}

		testErr := errors.New("test error")
		func() {
			defer RecoverFunc(hook)()
			panic(testErr)
		}()

		assert.Equal(t, testErr, panicValue)
	})
}

func TestDefaultRecoveryFunc(t *testing.T) {
	t.Run("string type panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			DefaultRecoveryFunc("test panic string")
		})
	})

	t.Run("error type panic", func(t *testing.T) {
		testErr := errors.New("test error")
		assert.NotPanics(t, func() {
			DefaultRecoveryFunc(testErr)
		})
	})

	t.Run("any type panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			DefaultRecoveryFunc(123)
		})
	})

	t.Run("nil value", func(t *testing.T) {
		assert.NotPanics(t, func() {
			DefaultRecoveryFunc(nil)
		})
	})
}

