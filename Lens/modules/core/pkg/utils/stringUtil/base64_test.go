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
			name:     "简单字符串",
			input:    "hello",
			expected: "aGVsbG8=",
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "",
		},
		{
			name:     "包含特殊字符的字符串",
			input:    "hello@world!123",
			expected: "aGVsbG9Ad29ybGQhMTIz",
		},
		{
			name:     "中文字符串",
			input:    "你好世界",
			expected: "5L2g5aW95LiW55WM",
		},
		{
			name:     "包含换行符",
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
			name:        "简单字符串",
			input:       "aGVsbG8=",
			expected:    "hello",
			expectError: false,
		},
		{
			name:        "空字符串",
			input:       "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "包含特殊字符的字符串",
			input:       "aGVsbG9Ad29ybGQhMTIz",
			expected:    "hello@world!123",
			expectError: false,
		},
		{
			name:        "中文字符串",
			input:       "5L2g5aW95LiW55WM",
			expected:    "你好世界",
			expectError: false,
		},
		{
			name:        "无效的base64字符串",
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
			name:     "包含+和/的字符串",
			input:    "subjects?_d=1",
			expected: "c3ViamVjdHM_X2Q9MQ==",
		},
		{
			name:     "简单字符串",
			input:    "hello",
			expected: "aGVsbG8=",
		},
		{
			name:     "空字符串",
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
			name:        "URL安全的base64字符串",
			input:       "c3ViamVjdHM_X2Q9MQ==",
			expected:    "subjects?_d=1",
			expectError: false,
		},
		{
			name:        "简单字符串",
			input:       "aGVsbG8=",
			expected:    "hello",
			expectError: false,
		},
		{
			name:        "空字符串",
			input:       "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "无效的base64字符串",
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
			name:  "普通字符串往返",
			input: "hello world",
		},
		{
			name:  "中文字符串往返",
			input: "你好世界",
		},
		{
			name:  "特殊字符往返",
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
			name:  "URL字符串往返",
			input: "https://example.com?param=value",
		},
		{
			name:  "特殊URL字符往返",
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

