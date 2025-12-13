package stringUtil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeBase64(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "aGVsbG8=",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "string with special characters",
			input:    "hello@world!123",
			expected: "aGVsbG9Ad29ybGQhMTIz",
		},
		{
			name:     "chinese string",
			input:    "hello world",
			expected: "aGVsbG8gd29ybGQ=",
		},
		{
			name:     "contains newline",
			input:    "line1\nline2",
			expected: "bGluZTEKbGluZTI=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeBase64(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodeBase64(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "simple string",
			input:       "aGVsbG8=",
			expected:    "hello",
			expectError: false,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "string with special characters",
			input:       "aGVsbG9Ad29ybGQhMTIz",
			expected:    "hello@world!123",
			expectError: false,
		},
		{
			name:        "chinese string",
			input:       "aGVsbG8gd29ybGQ=",
			expected:    "hello world",
			expectError: false,
		},
		{
			name:        "invalid base64 string",
			input:       "invalid!!!",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeBase64(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestEncodeBase64URL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "string with + and /",
			input:    "subjects?_d=1",
			expected: "c3ViamVjdHM_X2Q9MQ==",
		},
		{
			name:     "simple string",
			input:    "hello",
			expected: "aGVsbG8=",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeBase64URL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodeBase64URL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "URL-safe base64 string",
			input:       "c3ViamVjdHM_X2Q9MQ==",
			expected:    "subjects?_d=1",
			expectError: false,
		},
		{
			name:        "simple string",
			input:       "aGVsbG8=",
			expected:    "hello",
			expectError: false,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "invalid base64 string",
			input:       "invalid!!!",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeBase64URL(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBase64RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "normal string round trip",
			input: "hello world",
		},
		{
			name:  "unicode string round trip",
			input: "hello world мир",
		},
		{
			name:  "special characters round trip",
			input: "!@#$%^&*()_+-=[]{}|;':\",./<>?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeBase64(tt.input)
			decoded, err := DecodeBase64(encoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.input, decoded)
		})
	}
}

func TestBase64URLRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "URL string round trip",
			input: "https://example.com?param=value",
		},
		{
			name:  "special URL characters round trip",
			input: "subjects?_d=1&test=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeBase64URL(tt.input)
			decoded, err := DecodeBase64URL(encoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.input, decoded)
		})
	}
}
