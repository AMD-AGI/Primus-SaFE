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
			name:     "nil错误",
			err:      nil,
			expected: false,
		},
		{
			name:     "普通错误-不可重试",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name: "Timeout错误-可重试",
			err: apierrors.NewTimeoutError(
				"request timeout",
				1,
			),
			expected: true,
		},
		{
			name: "ServerTimeout错误-可重试",
			err: apierrors.NewServerTimeout(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"get",
				1,
			),
			expected: true,
		},
		{
			name: "Conflict错误-可重试",
			err: apierrors.NewConflict(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"test-deployment",
				errors.New("conflict"),
			),
			expected: true,
		},
		{
			name: "ServiceUnavailable错误-可重试",
			err: apierrors.NewServiceUnavailable(
				"service unavailable",
			),
			expected: true,
		},
		{
			name: "InternalError错误-可重试",
			err: apierrors.NewInternalError(
				errors.New("internal error"),
			),
			expected: true,
		},
		{
			name: "TooManyRequests错误-可重试",
			err: apierrors.NewTooManyRequests(
				"too many requests",
				1,
			),
			expected: true,
		},
		{
			name: "NotFound错误-不可重试",
			err: apierrors.NewNotFound(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"test-deployment",
			),
			expected: false,
		},
		{
			name: "BadRequest错误-不可重试",
			err: apierrors.NewBadRequest(
				"bad request",
			),
			expected: false,
		},
		{
			name: "Unauthorized错误-不可重试",
			err: apierrors.NewUnauthorized(
				"unauthorized",
			),
			expected: false,
		},
		{
			name: "Forbidden错误-不可重试",
			err: apierrors.NewForbidden(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"test-deployment",
				errors.New("forbidden"),
			),
			expected: false,
		},
		{
			name: "AlreadyExists错误-不可重试",
			err: apierrors.NewAlreadyExists(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"test-deployment",
			),
			expected: false,
		},
		{
			name: "Invalid错误-不可重试",
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
			name:     "嵌套的可重试错误",
			err:      apierrors.NewConflict(schema.GroupResource{}, "", apierrors.NewTimeoutError("timeout", 1)),
			expected: true,
		},
		{
			name: "多种可重试错误组合-Timeout",
			err: func() error {
				// 创建一个 timeout 错误
				return apierrors.NewTimeoutError("timeout", 5)
			}(),
			expected: true,
		},
		{
			name: "多种可重试错误组合-TooManyRequests",
			err: func() error {
				// 创建一个限流错误
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

