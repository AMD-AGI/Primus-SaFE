package listener

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsRetriableError(t *testing.T) {
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
			name:     "normal error - not retriable",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name: "Timeout error - retriable",
			err: apierrors.NewTimeoutError(
				"request timeout",
				1,
			),
			expected: true,
		},
		{
			name: "ServerTimeout error - retriable",
			err: apierrors.NewServerTimeout(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"get",
				1,
			),
			expected: true,
		},
		{
			name: "Conflict error - retriable",
			err: apierrors.NewConflict(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"test-deployment",
				errors.New("conflict"),
			),
			expected: true,
		},
		{
			name: "ServiceUnavailable error - retriable",
			err: apierrors.NewServiceUnavailable(
				"service unavailable",
			),
			expected: true,
		},
		{
			name: "InternalError error - retriable",
			err: apierrors.NewInternalError(
				errors.New("internal error"),
			),
			expected: true,
		},
		{
			name: "TooManyRequests error - retriable",
			err: apierrors.NewTooManyRequests(
				"too many requests",
				1,
			),
			expected: true,
		},
		{
			name: "NotFound error - not retriable",
			err: apierrors.NewNotFound(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"test-deployment",
			),
			expected: false,
		},
		{
			name: "BadRequest error - not retriable",
			err: apierrors.NewBadRequest(
				"bad request",
			),
			expected: false,
		},
		{
			name: "Unauthorized error - not retriable",
			err: apierrors.NewUnauthorized(
				"unauthorized",
			),
			expected: false,
		},
		{
			name: "Forbidden error - not retriable",
			err: apierrors.NewForbidden(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"test-deployment",
				errors.New("forbidden"),
			),
			expected: false,
		},
		{
			name: "AlreadyExists error - not retriable",
			err: apierrors.NewAlreadyExists(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"test-deployment",
			),
			expected: false,
		},
		{
			name: "Invalid error - not retriable",
			err: apierrors.NewInvalid(
				schema.GroupKind{Group: "apps", Kind: "Deployment"},
				"test-deployment",
				nil,
			),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetriableError(tt.err)
			assert.Equal(t, tt.expected, result, "Error retriability mismatch")
		})
	}
}

func TestIsRetriableError_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nested retriable error",
			err:      apierrors.NewConflict(schema.GroupResource{}, "", apierrors.NewTimeoutError("timeout", 1)),
			expected: true,
		},
		{
			name: "multiple retriable errors combination - Timeout",
			err: func() error {
				// create a timeout error
				return apierrors.NewTimeoutError("timeout", 5)
			}(),
			expected: true,
		},
		{
			name: "multiple retriable errors combination - TooManyRequests",
			err: func() error {
				// create a rate limiting error
				return apierrors.NewTooManyRequests("rate limited", 10)
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetriableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

