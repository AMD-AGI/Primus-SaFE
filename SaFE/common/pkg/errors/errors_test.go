/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package errors

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError_Error_WithoutInnerError(t *testing.T) {
	err := &Error{
		Code:    "TEST_CODE",
		Message: "test message",
		Stack:   []runtime.Frame{},
	}

	result := err.Error()

	assert.Contains(t, result, "code TEST_CODE")
	assert.Contains(t, result, "message test message")
	assert.NotContains(t, result, "error")
}

func TestError_Error_WithInnerError(t *testing.T) {
	innerErr := errors.New("inner error message")
	err := &Error{
		Code:       "TEST_CODE",
		Message:    "test message",
		InnerError: innerErr,
		Stack:      []runtime.Frame{},
	}

	result := err.Error()

	assert.Contains(t, result, "error inner error message")
	assert.Contains(t, result, "code TEST_CODE")
	assert.Contains(t, result, "message test message")
}

func TestError_GetTopStackString_EmptyStack(t *testing.T) {
	err := &Error{
		Stack: []runtime.Frame{},
	}

	result := err.GetTopStackString()

	assert.Empty(t, result)
}

func TestError_GetTopStackString_WithStack(t *testing.T) {
	// Create a stack frame by capturing current call stack
	pc, file, line, _ := runtime.Caller(0)
	fn := runtime.FuncForPC(pc)

	frame := runtime.Frame{
		PC:   pc,
		File: file,
		Line: line,
		Func: fn,
	}

	err := &Error{
		Stack: []runtime.Frame{frame},
	}

	result := err.GetTopStackString()

	assert.Contains(t, result, file)
	assert.Contains(t, result, "errors_test")
}

func TestError_GetStackString_EmptyStack(t *testing.T) {
	err := &Error{
		Stack: []runtime.Frame{},
	}

	result := err.GetStackString()

	assert.Empty(t, result)
}

func TestError_GetStackString_WithMultipleFrames(t *testing.T) {
	// Create multiple stack frames
	pc1, file1, line1, _ := runtime.Caller(0)
	fn1 := runtime.FuncForPC(pc1)
	frame1 := runtime.Frame{
		PC:   pc1,
		File: file1,
		Line: line1,
		Func: fn1,
	}

	pc2, file2, line2, _ := runtime.Caller(0)
	fn2 := runtime.FuncForPC(pc2)
	frame2 := runtime.Frame{
		PC:   pc2,
		File: file2,
		Line: line2,
		Func: fn2,
	}

	err := &Error{
		Stack: []runtime.Frame{frame1, frame2},
	}

	result := err.GetStackString()

	// Should contain both frames
	lines := strings.Split(strings.TrimSpace(result), "\n")
	assert.GreaterOrEqual(t, len(lines), 2)
}

func TestError_GetStackString_NilFunc(t *testing.T) {
	frame := runtime.Frame{
		File: "/path/to/file.go",
		Line: 42,
		Func: nil,
	}

	err := &Error{
		Stack: []runtime.Frame{frame},
	}

	result := err.GetStackString()

	assert.Contains(t, result, "/path/to/file.go")
	assert.Contains(t, result, "42")
}

func TestError_WithCode(t *testing.T) {
	err := &Error{}

	result := err.WithCode("NEW_CODE")

	assert.Same(t, err, result) // Should return same instance for chaining
	assert.Equal(t, "NEW_CODE", err.Code)
}

func TestError_WithMessage(t *testing.T) {
	err := &Error{}

	result := err.WithMessage("new message")

	assert.Same(t, err, result) // Should return same instance for chaining
	assert.Equal(t, "new message", err.Message)
}

func TestError_WithError(t *testing.T) {
	err := &Error{}
	innerErr := errors.New("inner error")

	result := err.WithError(innerErr)

	assert.Same(t, err, result) // Should return same instance for chaining
	assert.Equal(t, innerErr, err.InnerError)
}

func TestError_Chaining(t *testing.T) {
	innerErr := errors.New("inner error")
	err := &Error{}

	err.WithCode("CHAINED_CODE").
		WithMessage("chained message").
		WithError(innerErr)

	assert.Equal(t, "CHAINED_CODE", err.Code)
	assert.Equal(t, "chained message", err.Message)
	assert.Equal(t, innerErr, err.InnerError)
}

func TestError_ImplementsErrorInterface(t *testing.T) {
	var _ error = &Error{}
}

func TestError_GetTopStackString_FuncNameExtraction(t *testing.T) {
	// Test that function name is properly extracted from full path
	pc, file, line, _ := runtime.Caller(0)
	fn := runtime.FuncForPC(pc)

	frame := runtime.Frame{
		PC:   pc,
		File: file,
		Line: line,
		Func: fn,
	}

	err := &Error{
		Stack: []runtime.Frame{frame},
	}

	result := err.GetTopStackString()

	// The function name should be extracted and contain only the last part after "/"
	// It should contain the package and function name
	assert.NotEmpty(t, result)
	// Should not contain multiple "/" for the function name part
	parts := strings.Split(result, " ")
	if len(parts) > 1 {
		funcPart := parts[len(parts)-1]
		assert.False(t, strings.HasPrefix(funcPart, "/"))
	}
}

func TestError_EmptyCodeAndMessage(t *testing.T) {
	err := &Error{
		Code:    "",
		Message: "",
		Stack:   []runtime.Frame{},
	}

	result := err.Error()

	assert.Contains(t, result, "code .")
	assert.Contains(t, result, "message ")
}

