package errors

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewError tests the NewError constructor
func TestNewError(t *testing.T) {
	err := NewError()
	require.NotNil(t, err)
	assert.Equal(t, 0, err.Code)
	assert.Equal(t, "", err.Message)
	assert.Nil(t, err.InnerError)
	assert.NotEmpty(t, err.Stack, "Stack should be captured")
}

// TestError_WithCode tests the WithCode method
func TestError_WithCode(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"Client error", RequestParameterInvalid},
		{"Internal error", InternalError},
		{"Custom code", 9999},
		{"Zero code", 0},
		{"Negative code", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError().WithCode(tt.code)
			assert.Equal(t, tt.code, err.Code)
		})
	}
}

// TestError_WithMessage tests the WithMessage method
func TestError_WithMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"Simple message", "test error"},
		{"Empty message", ""},
		{"Long message", strings.Repeat("a", 1000)},
		{"Unicode message", "error message 🚨"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError().WithMessage(tt.message)
			assert.Equal(t, tt.message, err.Message)
		})
	}
}

// TestError_WithMessagef tests the WithMessagef method
func TestError_WithMessagef(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "Simple format",
			format:   "error: %s",
			args:     []interface{}{"test"},
			expected: "error: test",
		},
		{
			name:     "Multiple args",
			format:   "code: %d, message: %s",
			args:     []interface{}{500, "internal error"},
			expected: "code: 500, message: internal error",
		},
		{
			name:     "No args",
			format:   "no arguments",
			args:     []interface{}{},
			expected: "no arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError().WithMessagef(tt.format, tt.args...)
			assert.Equal(t, tt.expected, err.Message)
		})
	}
}

// TestError_WithError tests the WithError method
func TestError_WithError(t *testing.T) {
	innerErr := errors.New("inner error")
	err := NewError().WithError(innerErr)
	assert.Equal(t, innerErr, err.InnerError)
}

// TestError_ChainedMethods tests chaining multiple With* methods
func TestError_ChainedMethods(t *testing.T) {
	innerErr := errors.New("database connection failed")
	err := NewError().
		WithCode(CodeDatabaseError).
		WithMessage("failed to query database").
		WithError(innerErr)

	assert.Equal(t, CodeDatabaseError, err.Code)
	assert.Equal(t, "failed to query database", err.Message)
	assert.Equal(t, innerErr, err.InnerError)
}

// TestError_Error tests the Error() method without inner error
func TestError_Error_WithoutInnerError(t *testing.T) {
	err := NewError().
		WithCode(RequestParameterInvalid).
		WithMessage("invalid parameter")

	result := err.Error()
	assert.Contains(t, result, "code 4001")
	assert.Contains(t, result, "message invalid parameter")
	assert.Contains(t, result, "stack")
	assert.NotContains(t, result, "error ")
}

// TestError_Error tests the Error() method with inner error
func TestError_Error_WithInnerError(t *testing.T) {
	innerErr := errors.New("connection refused")
	err := NewError().
		WithCode(ClientError).
		WithMessage("failed to connect").
		WithError(innerErr)

	result := err.Error()
	assert.Contains(t, result, "error connection refused")
	assert.Contains(t, result, "code 6001")
	assert.Contains(t, result, "message failed to connect")
	assert.Contains(t, result, "stack")
}

// TestError_GetStackString tests the GetStackString method
func TestError_GetStackString(t *testing.T) {
	err := NewError()
	stackString := err.GetStackString()

	assert.NotEmpty(t, stackString)
	// Stack should contain file names and line numbers
	assert.Contains(t, stackString, "error_test.go")
	assert.Contains(t, stackString, ":")
	// Stack should contain function names
	assert.True(t, strings.Contains(stackString, "TestError_GetStackString") ||
		strings.Contains(stackString, "errors."))
}

// TestError_GetStackString_EmptyStack tests GetStackString with empty stack
func TestError_GetStackString_EmptyStack(t *testing.T) {
	err := &Error{Stack: []runtime.Frame{}}
	stackString := err.GetStackString()
	assert.Equal(t, "", stackString)
}

// TestError_GetStackString_Format tests the format of stack string
func TestError_GetStackString_Format(t *testing.T) {
	err := NewError()
	stackString := err.GetStackString()

	lines := strings.Split(strings.TrimSpace(stackString), "\n")
	for _, line := range lines {
		if line != "" {
			// Each line should have format: "file:line functionName"
			assert.True(t, strings.Contains(line, ":"), "Line should contain colon: %s", line)
		}
	}
}

// TestWrapError tests the WrapError function
func TestWrapError(t *testing.T) {
	innerErr := errors.New("original error")
	err := WrapError(innerErr, "wrapped message", InternalError)

	assert.Equal(t, InternalError, err.Code)
	assert.Equal(t, "wrapped message", err.Message)
	assert.Equal(t, innerErr, err.InnerError)
	assert.NotEmpty(t, err.Stack)
}

// TestWrapMessage tests the WrapMessage function
func TestWrapMessage(t *testing.T) {
	err := WrapMessage("error occurred", RequestDataNotExisted)

	assert.Equal(t, RequestDataNotExisted, err.Code)
	assert.Equal(t, "error occurred", err.Message)
	assert.Nil(t, err.InnerError)
	assert.NotEmpty(t, err.Stack)
}

// TestErrorCodes tests that error codes are defined correctly
func TestErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"RequestParameterInvalid", RequestParameterInvalid},
		{"RequestDataExists", RequestDataExists},
		{"AuthFailed", AuthFailed},
		{"RequestDataNotExisted", RequestDataNotExisted},
		{"PermissionDeny", PermissionDeny},
		{"InvalidOperation", InvalidOperation},
		{"InvalidArgument", InvalidArgument},
		{"InternalError", InternalError},
		{"InvalidDataError", InvalidDataError},
		{"CodeDatabaseError", CodeDatabaseError},
		{"ClientError", ClientError},
		{"K8SOperationError", K8SOperationError},
		{"OpensearchError", OpensearchError},
		{"CodeInitializeError", CodeInitializeError},
		{"CodeLackOfConfig", CodeLackOfConfig},
		{"CodeRemoteServiceError", CodeRemoteServiceError},
		{"CodeInvalidArgument", CodeInvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEqual(t, 0, tt.code, "Error code should not be zero")
		})
	}
}

// TestErrorCodes_Categories tests that error codes are in correct ranges
func TestErrorCodes_Categories(t *testing.T) {
	// 4xxx: Client errors
	assert.True(t, RequestParameterInvalid >= 4000 && RequestParameterInvalid < 5000)
	assert.True(t, RequestDataExists >= 4000 && RequestDataExists < 5000)
	assert.True(t, AuthFailed >= 4000 && AuthFailed < 5000)
	assert.True(t, RequestDataNotExisted >= 4000 && RequestDataNotExisted < 5000)
	assert.True(t, PermissionDeny >= 4000 && PermissionDeny < 5000)
	assert.True(t, InvalidOperation >= 4000 && InvalidOperation < 5000)
	assert.True(t, InvalidArgument >= 4000 && InvalidArgument < 5000)

	// 5xxx: Internal errors
	assert.True(t, InternalError >= 5000 && InternalError < 6000)
	assert.True(t, InvalidDataError >= 5000 && InvalidDataError < 6000)
	assert.True(t, CodeDatabaseError >= 5000 && CodeDatabaseError < 6000)

	// 6xxx: External service errors
	assert.True(t, ClientError >= 6000 && ClientError < 7000)
	assert.True(t, K8SOperationError >= 6000 && K8SOperationError < 7000)
	assert.True(t, OpensearchError >= 6000 && OpensearchError < 7000)

	// 7xxx: Initialization errors
	assert.True(t, CodeInitializeError >= 7000 && CodeInitializeError < 8000)
	assert.True(t, CodeLackOfConfig >= 7000 && CodeLackOfConfig < 8000)

	// 8xxx: Remote service errors
	assert.True(t, CodeRemoteServiceError >= 8000 && CodeRemoteServiceError < 9000)
	assert.True(t, CodeInvalidArgument >= 8000 && CodeInvalidArgument < 9000)
}

// TestError_StackCapture tests that stack is captured at the correct depth
func TestError_StackCapture(t *testing.T) {
	// Create error in a nested function to test stack depth
	err := createNestedError()

	stackString := err.GetStackString()
	assert.Contains(t, stackString, "createNestedError", "Stack should contain the nested function")
	assert.Contains(t, stackString, "TestError_StackCapture", "Stack should contain the test function")
}

func createNestedError() *Error {
	return NewError().WithMessage("nested error")
}

// TestError_NilInnerError tests behavior with nil inner error
func TestError_NilInnerError(t *testing.T) {
	err := NewError().
		WithCode(InternalError).
		WithMessage("test error").
		WithError(nil)

	result := err.Error()
	assert.Nil(t, err.InnerError)
	assert.NotContains(t, result, "error <nil>")
}

// TestError_ComplexScenario tests a realistic error handling scenario
func TestError_ComplexScenario(t *testing.T) {
	// Simulate a database error
	dbErr := errors.New("connection timeout")
	err := WrapError(dbErr, "failed to execute query", CodeDatabaseError)

	// Verify all components
	assert.Equal(t, CodeDatabaseError, err.Code)
	assert.Equal(t, "failed to execute query", err.Message)
	assert.Equal(t, dbErr, err.InnerError)
	assert.NotEmpty(t, err.Stack)

	// Verify error string formatting
	errorString := err.Error()
	assert.Contains(t, errorString, "connection timeout")
	assert.Contains(t, errorString, "5002")
	assert.Contains(t, errorString, "failed to execute query")
}

// TestError_FunctionNameParsing tests that function names are parsed correctly
func TestError_FunctionNameParsing(t *testing.T) {
	err := NewError()
	stackString := err.GetStackString()

	// Function names should be shortened (package name removed)
	lines := strings.Split(strings.TrimSpace(stackString), "\n")
	for _, line := range lines {
		if line != "" {
			parts := strings.Split(line, " ")
			if len(parts) > 1 {
				funcName := parts[len(parts)-1]
				// Should not contain full package path
				slashCount := strings.Count(funcName, "/")
				assert.Equal(t, 0, slashCount, "Function name should not contain slashes: %s", funcName)
			}
		}
	}
}

// BenchmarkNewError benchmarks error creation
func BenchmarkNewError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewError()
	}
}

// BenchmarkWrapError benchmarks error wrapping
func BenchmarkWrapError(b *testing.B) {
	innerErr := errors.New("inner error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WrapError(innerErr, "wrapped", InternalError)
	}
}

// BenchmarkError_Error benchmarks error string generation
func BenchmarkError_Error(b *testing.B) {
	err := NewError().
		WithCode(InternalError).
		WithMessage("test error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

// BenchmarkError_GetStackString benchmarks stack string generation
func BenchmarkError_GetStackString(b *testing.B) {
	err := NewError()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.GetStackString()
	}
}

// BenchmarkError_ChainedMethods benchmarks chained builder methods
func BenchmarkError_ChainedMethods(b *testing.B) {
	innerErr := errors.New("inner")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewError().
			WithCode(InternalError).
			WithMessage("test").
			WithError(innerErr)
	}
}

// Example tests
func ExampleNewError() {
	err := NewError().
		WithCode(InternalError).
		WithMessage("something went wrong")
	fmt.Printf("Code: %d, Message: %s\n", err.Code, err.Message)
	// Output will vary due to stack trace
}

func ExampleWrapError() {
	originalErr := errors.New("database connection failed")
	err := WrapError(originalErr, "failed to save user", CodeDatabaseError)
	_ = err.Error()
	// Error string will contain both the original error and the wrap message
}

func ExampleWrapMessage() {
	err := WrapMessage("invalid user input", RequestParameterInvalid)
	fmt.Printf("Error code: %d\n", err.Code)
	// Output: Error code: 4001
}

