package pgUtil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringArrayToPgArray(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "空数组",
			input:    []string{},
			expected: "{}",
		},
		{
			name:     "单个元素",
			input:    []string{"tag1"},
			expected: `{"tag1"}`, // pq.Array 总是给字符串加引号
		},
		{
			name:     "多个元素",
			input:    []string{"tag1", "tag2", "tag3"},
			expected: `{"tag1","tag2","tag3"}`, // pq.Array 总是给字符串加引号
		},
		{
			name:     "包含空格的元素",
			input:    []string{"tag with space", "normal"},
			expected: `{"tag with space","normal"}`, // pq.Array 总是给字符串加引号
		},
		{
			name:     "包含特殊字符的元素",
			input:    []string{"tag,comma", "tag{brace}"},
			expected: `{"tag,comma","tag{brace}"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringArrayToPgArray(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPgArrayToStringArray(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "空字符串",
			input:    "",
			expected: []string{},
		},
		{
			name:     "空数组",
			input:    "{}",
			expected: []string{},
		},
		{
			name:     "单个元素",
			input:    "{tag1}",
			expected: []string{"tag1"},
		},
		{
			name:     "多个元素",
			input:    "{tag1,tag2,tag3}",
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "包含空格的元素",
			input:    `{"tag with space",normal}`,
			expected: []string{"tag with space", "normal"},
		},
		{
			name:     "包含特殊字符的元素",
			input:    `{"tag,comma","tag{brace}"}`,
			expected: []string{"tag,comma", "tag{brace}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PgArrayToStringArray(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input []string
	}{
		{
			name:  "空数组往返",
			input: []string{},
		},
		{
			name:  "简单数组往返",
			input: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:  "复杂数组往返",
			input: []string{"tag with space", "tag,comma", "normal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgArray := StringArrayToPgArray(tt.input)
			result := PgArrayToStringArray(pgArray)
			assert.Equal(t, tt.input, result)
		})
	}
}

