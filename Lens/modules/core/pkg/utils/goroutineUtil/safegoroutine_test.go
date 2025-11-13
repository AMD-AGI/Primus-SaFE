package goroutineUtil

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSafeGoroutine(t *testing.T) {
	t.Run("正常执行", func(t *testing.T) {
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

	t.Run("捕获panic", func(t *testing.T) {
		recovered := false
		var wg sync.WaitGroup
		wg.Add(1)

		callback := func(r interface{}) {
			recovered = true
		}

		SafeGoroutine(func() {
			defer wg.Done()
			panic("测试panic")
		}, callback)

		wg.Wait()
		assert.True(t, recovered)
	})

	t.Run("不带回调的panic捕获", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		assert.NotPanics(t, func() {
			SafeGoroutine(func() {
				defer wg.Done()
				panic("测试panic")
			})
		})

		wg.Wait()
	})
}

func TestRunGoroutineWithLog(t *testing.T) {
	t.Run("在goroutine中正常执行", func(t *testing.T) {
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

	t.Run("在goroutine中捕获panic", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		assert.NotPanics(t, func() {
			RunGoroutineWithLog(func() {
				defer wg.Done()
				panic("测试panic")
			})
		})

		wg.Wait()
	})
}

func TestSafeGoroutineWithLog(t *testing.T) {
	t.Run("正常执行", func(t *testing.T) {
		executed := false
		SafeGoroutineWithLog(func() {
			executed = true
		})
		// 给一点时间让goroutine执行
		time.Sleep(10 * time.Millisecond)
		assert.True(t, executed)
	})

	t.Run("捕获panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			SafeGoroutineWithLog(func() {
				panic("测试panic")
			})
			time.Sleep(10 * time.Millisecond)
		})
	})
}

func TestRecovery(t *testing.T) {
	t.Run("没有panic时不调用回调", func(t *testing.T) {
		callbackCalled := false
		callback := func(r interface{}) {
			callbackCalled = true
		}

		func() {
			defer Recovery(callback)
			// 正常执行
		}()

		assert.False(t, callbackCalled)
	})

	t.Run("panic时调用回调", func(t *testing.T) {
		callbackCalled := false
		var panicValue interface{}
		callback := func(r interface{}) {
			callbackCalled = true
			panicValue = r
		}

		func() {
			defer Recovery(callback)
			panic("测试panic")
		}()

		assert.True(t, callbackCalled)
		assert.Equal(t, "测试panic", panicValue)
	})

	t.Run("多个回调都被调用", func(t *testing.T) {
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
			panic("测试panic")
		}()

		assert.True(t, callback1Called)
		assert.True(t, callback2Called)
	})

	t.Run("没有回调时使用默认日志", func(t *testing.T) {
		assert.NotPanics(t, func() {
			defer Recovery()
			panic("测试panic")
		})
	})

	t.Run("nil回调被跳过", func(t *testing.T) {
		callbackCalled := false
		callback := func(r interface{}) {
			callbackCalled = true
		}

		func() {
			defer Recovery(nil, callback, nil)
			panic("测试panic")
		}()

		assert.True(t, callbackCalled)
	})
}

