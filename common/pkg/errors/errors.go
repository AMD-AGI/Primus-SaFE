/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// Error represents a custom error type that includes stack trace information,
// inner error, error code, and error message.
type Error struct {
	Stack      []runtime.Frame
	InnerError error
	Code       string
	Message    string
}

// Error implements the error interface and returns a formatted error string.
// If InnerError exists, it includes the inner error details along with code, message and stack trace.
// Otherwise, it returns code, message and full stack trace information.
//
// Returns:
//   - string: Formatted error string containing all error details
func (e *Error) Error() string {
	if e.InnerError == nil {
		return fmt.Sprintf(" code %s.message %s \nstack %s", e.Code, e.Message, e.GetStackString())
	}
	return fmt.Sprintf("error %s code %s message %s \nstack %s", e.InnerError.Error(), e.Code, e.Message, e.GetStackString())
}

// GetTopStackString returns the top frame of the stack trace as a formatted string.
// It extracts file name, line number, and function name from the first stack frame.
// The function name is simplified by removing package path prefix.
//
// Returns:
//   - string: Formatted string in format "filename:line functionName
func (e *Error) GetTopStackString() string {
	if len(e.Stack) == 0 {
		return ""
	}
	frame := e.Stack[0]
	funcName := ""
	if frame.Func != nil {
		funcName = frame.Func.Name()
	}
	funcNames := strings.Split(funcName, "/")
	if len(funcNames) > 0 {
		funcName = funcNames[len(funcNames)-1]
	}
	return fmt.Sprintf("%s:%d %s", frame.File, frame.Line, funcName)
}

// GetStackString returns the complete stack trace as a formatted string.
// It iterates through all stack frames and formats each with file name, line number and function name.
// Function names are simplified by removing package path prefixes.
//
// Returns:
//   - string: Formatted string containing all stack frames, one per lin
func (e *Error) GetStackString() string {
	result := ""
	for _, frame := range e.Stack {
		funcName := ""
		if frame.Func != nil {
			funcName = frame.Func.Name()
		}
		funcNames := strings.Split(funcName, "/")
		if len(funcNames) > 0 {
			funcName = funcNames[len(funcNames)-1]
		}
		result = fmt.Sprintf("%s%s:%d %s\n", result, frame.File, frame.Line, funcName)
	}
	return result
}

// WithCode sets the error code and returns the Error instance for chaining.
// Enables fluent interface pattern for setting error properties.
//
// Parameters:
//   - code: Error code string for categorizing the error
//
// Returns:
//   - *Error: Pointer to the current Error instance
func (e *Error) WithCode(code string) *Error {
	e.Code = code
	return e
}

// WithMessage sets the error message and returns the Error instance for chaining.
// Enables fluent interface pattern for setting error properties.
//
// Parameters:
//   - message: Human-readable error message
//
// Returns:
//   - *Error: Pointer to the current Error instance
func (e *Error) WithMessage(message string) *Error {
	e.Message = message
	return e
}

// WithError sets the inner error and returns the Error instance for chaining.
// Enables fluent interface pattern for wrapping underlying errors.
//
// Parameters:
//   - err: The underlying error to wrap
//
// Returns:
//   - *Error: Pointer to the current Error instance
func (e *Error) WithError(err error) *Error {
	e.InnerError = err
	return e
}
