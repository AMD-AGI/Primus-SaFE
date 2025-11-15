package goroutineUtil

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSafeGoroutine(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		executed := false
		var wg sync.WaitGroup
		wg.Add(1)

		SafeGoroutine(func() {
			defer wg.Done()
			executed = true
		})

		wg.Wait()
		assert.True(t, executed)
	})

	t.Run("capture panic", func(t *testing.T) {
		recovered := false
		var wg sync.WaitGroup
		wg.Add(1)

		callback := func(r interface{}) {
			recovered = true
		}

		SafeGoroutine(func() {
			defer wg.Done()
			panic("test panic")
		}, callback)

		wg.Wait()
		assert.True(t, recovered)
	})

	t.Run("panic capture without callback", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		assert.NotPanics(t, func() {
			SafeGoroutine(func() {
				defer wg.Done()
				panic("test panic")
			})
		})

		wg.Wait()
	})
}

func TestRunGoroutineWithLog(t *testing.T) {
	t.Run("normal execution in goroutine", func(t *testing.T) {
		executed := false
		var wg sync.WaitGroup
		wg.Add(1)

		RunGoroutineWithLog(func() {
			defer wg.Done()
			executed = true
		})

		wg.Wait()
		assert.True(t, executed)
	})

	t.Run("capture panic in goroutine", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		assert.NotPanics(t, func() {
			RunGoroutineWithLog(func() {
				defer wg.Done()
				panic("test panic")
			})
		})

		wg.Wait()
	})
}

func TestSafeGoroutineWithLog(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		executed := false
		SafeGoroutineWithLog(func() {
			executed = true
		})
		// give some time for goroutine to execute
		time.Sleep(10 * time.Millisecond)
		assert.True(t, executed)
	})

	t.Run("capture panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			SafeGoroutineWithLog(func() {
				panic("test panic")
			})
			time.Sleep(10 * time.Millisecond)
		})
	})
}

func TestRecovery(t *testing.T) {
	t.Run("callback not called when no panic", func(t *testing.T) {
		callbackCalled := false
		callback := func(r interface{}) {
			callbackCalled = true
		}

		func() {
			defer Recovery(callback)
			// normal execution
		}()

		assert.False(t, callbackCalled)
	})

	t.Run("callback called on panic", func(t *testing.T) {
		callbackCalled := false
		var panicValue interface{}
		callback := func(r interface{}) {
			callbackCalled = true
			panicValue = r
		}

		func() {
			defer Recovery(callback)
			panic("test panic")
		}()

		assert.True(t, callbackCalled)
		assert.Equal(t, "test panic", panicValue)
	})

	t.Run("all callbacks are called", func(t *testing.T) {
		callback1Called := false
		callback2Called := false

		callback1 := func(r interface{}) {
			callback1Called = true
		}
		callback2 := func(r interface{}) {
			callback2Called = true
		}

		func() {
			defer Recovery(callback1, callback2)
			panic("test panic")
		}()

		assert.True(t, callback1Called)
		assert.True(t, callback2Called)
	})

	t.Run("use default logging when no callback", func(t *testing.T) {
		assert.NotPanics(t, func() {
			defer Recovery()
			panic("test panic")
		})
	})

	t.Run("nil callbacks are skipped", func(t *testing.T) {
		callbackCalled := false
		callback := func(r interface{}) {
			callbackCalled = true
		}

		func() {
			defer Recovery(nil, callback, nil)
			panic("test panic")
		}()

		assert.True(t, callbackCalled)
	})
}
