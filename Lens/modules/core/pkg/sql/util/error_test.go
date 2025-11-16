package dal

import (
	"errors"
	"testing"

	errors2 "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestCheckErr_NilError tests CheckErr with nil error
func TestCheckErr_NilError(t *testing.T) {
	tests := []struct {
		name          string
		allowNotExist bool
	}{
		{"allow not exist", true},
		{"not allow not exist", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckErr(nil, tt.allowNotExist)
			assert.NoError(t, err)
		})
	}
}

// TestCheckErr_RecordNotFound_Allowed tests CheckErr with ErrRecordNotFound when allowed
func TestCheckErr_RecordNotFound_Allowed(t *testing.T) {
	err := CheckErr(gorm.ErrRecordNotFound, true)
	assert.NoError(t, err, "Should return nil when ErrRecordNotFound is allowed")
}

// TestCheckErr_RecordNotFound_NotAllowed tests CheckErr with ErrRecordNotFound when not allowed
func TestCheckErr_RecordNotFound_NotAllowed(t *testing.T) {
	err := CheckErr(gorm.ErrRecordNotFound, false)
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err, "Should return ErrRecordNotFound when not allowed")
}

// TestCheckErr_OtherError tests CheckErr with other errors
func TestCheckErr_OtherError(t *testing.T) {
	originalErr := errors.New("database connection failed")

	tests := []struct {
		name          string
		allowNotExist bool
	}{
		{"allow not exist", true},
		{"not allow not exist", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckErr(originalErr, tt.allowNotExist)
			require.Error(t, err)

			// Should wrap the error with CodeDatabaseError
			customErr, ok := err.(*errors2.Error)
			require.True(t, ok, "Should return a custom Error type")
			assert.Equal(t, errors2.CodeDatabaseError, customErr.Code)
			assert.Equal(t, originalErr, customErr.InnerError)
		})
	}
}

// TestCheckErr_WrappedRecordNotFound tests CheckErr with wrapped ErrRecordNotFound
func TestCheckErr_WrappedRecordNotFound(t *testing.T) {
	wrappedErr := errors.New("query failed: " + gorm.ErrRecordNotFound.Error())

	// Test with allowNotExist = true
	err := CheckErr(wrappedErr, true)
	require.Error(t, err) // Wrapped error won't match errors.Is

	// Should return wrapped error with CodeDatabaseError
	customErr, ok := err.(*errors2.Error)
	require.True(t, ok)
	assert.Equal(t, errors2.CodeDatabaseError, customErr.Code)
}

// TestCheckErr_MultipleErrors tests CheckErr with various error types
func TestCheckErr_MultipleErrors(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		allowNotExist bool
		expectNil     bool
		expectCode    int
	}{
		{
			name:          "nil error, allow not exist",
			err:           nil,
			allowNotExist: true,
			expectNil:     true,
		},
		{
			name:          "nil error, not allow not exist",
			err:           nil,
			allowNotExist: false,
			expectNil:     true,
		},
		{
			name:          "record not found, allow",
			err:           gorm.ErrRecordNotFound,
			allowNotExist: true,
			expectNil:     true,
		},
		{
			name:          "record not found, not allow",
			err:           gorm.ErrRecordNotFound,
			allowNotExist: false,
			expectNil:     false,
		},
		{
			name:          "connection error, allow not exist",
			err:           errors.New("connection error"),
			allowNotExist: true,
			expectNil:     false,
			expectCode:    errors2.CodeDatabaseError,
		},
		{
			name:          "connection error, not allow not exist",
			err:           errors.New("connection error"),
			allowNotExist: false,
			expectNil:     false,
			expectCode:    errors2.CodeDatabaseError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckErr(tt.err, tt.allowNotExist)

			if tt.expectNil {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				if tt.err == gorm.ErrRecordNotFound {
					// If not allowed, should return ErrRecordNotFound directly
					assert.Equal(t, gorm.ErrRecordNotFound, err)
				} else {
					// Other errors should be wrapped
					customErr, ok := err.(*errors2.Error)
					require.True(t, ok)
					assert.Equal(t, tt.expectCode, customErr.Code)
				}
			}
		})
	}
}

// TestCheckErr_ErrorIsCheck tests that errors.Is works correctly
func TestCheckErr_ErrorIsCheck(t *testing.T) {
	// Create a wrapped ErrRecordNotFound
	wrappedErr := gorm.ErrRecordNotFound

	// Test with allowNotExist = true
	err := CheckErr(wrappedErr, true)
	assert.NoError(t, err)

	// Test with allowNotExist = false
	err = CheckErr(wrappedErr, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

// TestCheckErr_DifferentGormErrors tests various GORM error types
func TestCheckErr_DifferentGormErrors(t *testing.T) {
	gormErrors := []error{
		gorm.ErrRecordNotFound,
		gorm.ErrInvalidTransaction,
		gorm.ErrNotImplemented,
		gorm.ErrMissingWhereClause,
		gorm.ErrUnsupportedRelation,
		gorm.ErrPrimaryKeyRequired,
		gorm.ErrModelValueRequired,
		gorm.ErrInvalidData,
		gorm.ErrUnsupportedDriver,
		gorm.ErrRegistered,
		gorm.ErrInvalidField,
		gorm.ErrEmptySlice,
		gorm.ErrDryRunModeUnsupported,
	}

	for _, gormErr := range gormErrors {
		t.Run(gormErr.Error(), func(t *testing.T) {
			if errors.Is(gormErr, gorm.ErrRecordNotFound) {
				// Test with allowNotExist = true
				err := CheckErr(gormErr, true)
				assert.NoError(t, err)

				// Test with allowNotExist = false
				err = CheckErr(gormErr, false)
				assert.Error(t, err)
				assert.Equal(t, gormErr, err)
			} else {
				// Other GORM errors should be wrapped
				err := CheckErr(gormErr, false)
				require.Error(t, err)
				customErr, ok := err.(*errors2.Error)
				require.True(t, ok)
				assert.Equal(t, errors2.CodeDatabaseError, customErr.Code)
			}
		})
	}
}

// TestCheckErr_CustomError tests CheckErr with custom error types
func TestCheckErr_CustomError(t *testing.T) {
	type customError struct {
		message string
	}

	customErr := &customError{message: "custom error"}

	// Custom errors should be wrapped
	err := CheckErr(errors.New(customErr.message), false)
	require.Error(t, err)
	wrappedErr, ok := err.(*errors2.Error)
	require.True(t, ok)
	assert.Equal(t, errors2.CodeDatabaseError, wrappedErr.Code)
}

// BenchmarkCheckErr_Nil benchmarks CheckErr with nil error
func BenchmarkCheckErr_Nil(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CheckErr(nil, false)
	}
}

// BenchmarkCheckErr_RecordNotFound_Allowed benchmarks CheckErr with allowed ErrRecordNotFound
func BenchmarkCheckErr_RecordNotFound_Allowed(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CheckErr(gorm.ErrRecordNotFound, true)
	}
}

// BenchmarkCheckErr_RecordNotFound_NotAllowed benchmarks CheckErr with not allowed ErrRecordNotFound
func BenchmarkCheckErr_RecordNotFound_NotAllowed(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CheckErr(gorm.ErrRecordNotFound, false)
	}
}

// BenchmarkCheckErr_OtherError benchmarks CheckErr with other errors
func BenchmarkCheckErr_OtherError(b *testing.B) {
	testErr := errors.New("test error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CheckErr(testErr, false)
	}
}

