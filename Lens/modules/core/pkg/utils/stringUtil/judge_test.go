package stringUtil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "positive integer",
			input:    "123",
			expected: true,
		},
		{
			name:     "zero",
			input:    "0",
			expected: true,
		},
		{
			name:     "negative integer",
			input:    "-123",
			expected: true,
		},
		{
			name:     "contains letters",
			input:    "123abc",
			expected: false,
		},
		{
			name:     "pure letters",
			input:    "abc",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "floating point",
			input:    "123.45",
			expected: false,
		},
		{
			name:     "contains spaces",
			input:    "123 456",
			expected: false,
		},
		{
			name:     "leading zeros",
			input:    "0123",
			expected: true,
		},
		{
			name:     "only negative sign",
			input:    "-",
			expected: false,
		},
		{
			name:     "positive sign prefix",
			input:    "+123",
			expected: true, // strconv.Atoi can actually parse "+123"
		},
		{
			name:     "very large number",
			input:    "999999999999",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNumeric(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

