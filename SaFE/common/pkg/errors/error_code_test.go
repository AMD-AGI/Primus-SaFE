/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsPrimus(t *testing.T) {
	err := NewBadRequest("test")
	assert.Equal(t, IsPrimus(err), true)
	assert.Equal(t, GetErrorCode(err), BadRequest)

	err2 := fmt.Errorf("test")
	assert.Equal(t, IsPrimus(err2), false)
	assert.Equal(t, GetErrorCode(err2), "")
}

func TestIsAlreadyExist(t *testing.T) {
	err := NewAlreadyExist("test")
	assert.Equal(t, IsAlreadyExist(err), true)
	err2 := fmt.Errorf("test")
	assert.Equal(t, IsAlreadyExist(err2), false)

	err3 := apierrors.NewAlreadyExists(schema.GroupResource{}, "test")
	assert.Equal(t, IsAlreadyExist(err3), false)
}

// TestIsNonRetryableError tests the identification of non-retryable errors
func TestIsNonRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "bad request error",
			err:      NewBadRequest("invalid input"),
			expected: true,
		},
		{
			name:     "internal error",
			err:      NewInternalError("internal server error"),
			expected: true,
		},
		{
			name:     "not found error",
			err:      NewNotFound("resource", "test-resource"),
			expected: true,
		},
		{
			name:     "k8s forbidden error",
			err:      apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "test", fmt.Errorf("forbidden")),
			expected: true,
		},
		{
			name:     "k8s not found error",
			err:      apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "test"),
			expected: true,
		},
		{
			name:     "retryable error - timeout",
			err:      apierrors.NewTimeoutError("timeout", 30),
			expected: false,
		},
		{
			name:     "retryable error - service unavailable",
			err:      apierrors.NewServiceUnavailable("service unavailable"),
			expected: false,
		},
		{
			name:     "retryable error - too many requests",
			err:      apierrors.NewTooManyRequests("too many requests", 10),
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some generic error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNonRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
