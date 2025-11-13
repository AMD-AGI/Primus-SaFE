package goroutineUtil

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecoverFunc(t *testing.T) {
	t.Run("正常执行不触发恢复", func(t *testing.T) {
		hookCalled := false
		hook := func(r any) {
			hookCalled = true
		}

		func() {
			defer RecoverFunc(hook)()
			// 正常执行，不会 panic
		}()

		assert.False(t, hookCalled)
	})

	t.Run("panic时触发hook", func(t *testing.T) {
		hookCalled := false
		var panicValue any
		hook := func(r any) {
			hookCalled = true
			panicValue = r
		}

		func() {
			defer RecoverFunc(hook)()
			panic("测试panic")
		}()

		assert.True(t, hookCalled)
		assert.Equal(t, "测试panic", panicValue)
	})

	t.Run("hook为nil时不会panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			defer RecoverFunc(nil)()
			panic("测试panic")
		})
	})

	t.Run("panic error类型", func(t *testing.T) {
		var panicValue any
		hook := func(r any) {
			panicValue = r
		}

		testErr := errors.New("测试错误")
		func() {
			defer RecoverFunc(hook)()
			panic(testErr)
		}()

		assert.Equal(t, testErr, panicValue)
	})
}

func TestDefaultRecoveryFunc(t *testing.T) {
	t.Run("字符串类型的panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			DefaultRecoveryFunc("测试panic字符串")
		})
	})

	t.Run("error类型的panic", func(t *testing.T) {
		testErr := errors.New("测试错误")
		assert.NotPanics(t, func() {
			DefaultRecoveryFunc(testErr)
		})
	})

	t.Run("任意类型的panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			DefaultRecoveryFunc(123)
		})
	})

	t.Run("nil值", func(t *testing.T) {
		assert.NotPanics(t, func() {
			DefaultRecoveryFunc(nil)
		})
	})
}

