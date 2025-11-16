package gpu_workload

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNoKindMatchError(t *testing.T) {
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
			name:     "error with unknown Kind",
			err:      errors.New("no matches for kind \"CustomResource\" in version \"v1\""),
			expected: true,
		},
		{
			name:     "error with unknown kind lowercase",
			err:      errors.New("could not find the requested resource with unknown kind"),
			expected: true,
		},
		{
			name:     "error without kind match pattern",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "database error",
			err:      errors.New("database connection failed"),
			expected: false,
		},
		{
			name:     "network error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
		{
			name:     "error with 'no matches for kind' pattern",
			err:      errors.New("no matches for kind MyCustomKind in group"),
			expected: true,
		},
		{
			name:     "error with mixed case 'Unknown Kind'",
			err:      errors.New("the server could not find the requested resource with Unknown Kind"),
			expected: true,
		},
		{
			name:     "error message contains 'kind' but not the pattern",
			err:      errors.New("this is a kind of error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNoKindMatchError(tt.err)
			assert.Equal(t, tt.expected, result, "isNoKindMatchError result should match expected")
		})
	}
}

func TestIsNoKindMatchErrorCaseSensitivity(t *testing.T) {
	// Test various case combinations
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "UNKNOWN KIND uppercase",
			err:      errors.New("RESOURCE WITH UNKNOWN KIND"),
			expected: true,
		},
		{
			name:     "Unknown Kind mixed case",
			err:      errors.New("Resource with Unknown Kind found"),
			expected: true,
		},
		{
			name:     "NO MATCHES uppercase",
			err:      errors.New("NO MATCHES FOR KIND"),
			expected: true,
		},
		{
			name:     "no matches lowercase",
			err:      errors.New("no matches for kind CustomResource"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNoKindMatchError(tt.err)
			assert.Equal(t, tt.expected, result, "Should be case insensitive")
		})
	}
}

func TestIsNoKindMatchErrorWithWrappedErrors(t *testing.T) {
	// Test with wrapped errors (common in Go 1.13+)
	baseErr := errors.New("no matches for kind TestKind")
	wrappedErr := errors.New("wrapped: " + baseErr.Error())

	result := isNoKindMatchError(wrappedErr)
	assert.True(t, result, "Should detect pattern in wrapped error messages")
}

func TestIsNoKindMatchErrorEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "empty error message",
			err:      errors.New(""),
			expected: false,
		},
		{
			name:     "error with only spaces",
			err:      errors.New("   "),
			expected: false,
		},
		{
			name:     "error with kind but no match",
			err:      errors.New("kind test kind"),
			expected: false,
		},
		{
			name:     "error with matches but no kind",
			err:      errors.New("matches found"),
			expected: false,
		},
		{
			name:     "partial match 1",
			err:      errors.New("unknown"),
			expected: false,
		},
		{
			name:     "partial match 2",
			err:      errors.New("kind"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNoKindMatchError(tt.err)
			assert.Equal(t, tt.expected, result, "Edge case should be handled correctly")
		})
	}
}

func TestIsNoKindMatchErrorRealWorldExamples(t *testing.T) {
	// Real-world error messages from Kubernetes
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "CRD not installed",
			err:      errors.New("no matches for kind \"MyCustomResource\" in version \"example.com/v1\""),
			expected: true,
		},
		{
			name:     "Invalid kind in YAML",
			err:      errors.New("error from server (NotFound): the server could not find the requested resource with unknown Kind"),
			expected: true,
		},
		{
			name:     "API resource not found",
			err:      errors.New("unable to recognize resource: no matches for kind \"UnknownWorkload\" in version \"apps/v1\""),
			expected: true,
		},
		{
			name:     "Normal not found error",
			err:      errors.New("deployments.apps \"my-deployment\" not found"),
			expected: false,
		},
		{
			name:     "Permission denied",
			err:      errors.New("User \"test-user\" cannot get resource \"deployments\" in API group \"apps\""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNoKindMatchError(tt.err)
			assert.Equal(t, tt.expected, result, "Real-world example should be handled correctly")
		})
	}
}

