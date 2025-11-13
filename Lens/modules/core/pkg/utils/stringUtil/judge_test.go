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
			name:     "正整数",
			input:    "123",
			expected: true,
		},
		{
			name:     "零",
			input:    "0",
			expected: true,
		},
		{
			name:     "负整数",
			input:    "-123",
			expected: true,
		},
		{
			name:     "包含字母",
			input:    "123abc",
			expected: false,
		},
		{
			name:     "纯字母",
			input:    "abc",
			expected: false,
		},
		{
			name:     "空字符串",
			input:    "",
			expected: false,
		},
		{
			name:     "浮点数",
			input:    "123.45",
			expected: false,
		},
		{
			name:     "包含空格",
			input:    "123 456",
			expected: false,
		},
		{
			name:     "前导零",
			input:    "0123",
			expected: true,
		},
		{
			name:     "仅有负号",
			input:    "-",
			expected: false,
		},
		{
			name:     "正号开头",
			input:    "+123",
			expected: true, // strconv.Atoi 实际上可以解析 "+123"
		},
		{
			name:     "很大的数字",
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

