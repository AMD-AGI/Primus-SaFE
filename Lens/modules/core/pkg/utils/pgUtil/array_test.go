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
			name:     "empty array",
			input:    []string{},
			expected: "{}",
		},
		{
			name:     "single element",
			input:    []string{"tag1"},
			expected: `{"tag1"}`, // pq.Array always quotes strings
		},
		{
			name:     "multiple elements",
			input:    []string{"tag1", "tag2", "tag3"},
			expected: `{"tag1","tag2","tag3"}`, // pq.Array always quotes strings
		},
		{
			name:     "element with spaces",
			input:    []string{"tag with space", "normal"},
			expected: `{"tag with space","normal"}`, // pq.Array always quotes strings
		},
		{
			name:     "element with special characters",
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
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "empty array",
			input:    "{}",
			expected: []string{},
		},
		{
			name:     "single element",
			input:    "{tag1}",
			expected: []string{"tag1"},
		},
		{
			name:     "multiple elements",
			input:    "{tag1,tag2,tag3}",
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "element with spaces",
			input:    `{"tag with space",normal}`,
			expected: []string{"tag with space", "normal"},
		},
		{
			name:     "element with special characters",
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
			name:  "empty array round trip",
			input: []string{},
		},
		{
			name:  "simple array round trip",
			input: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:  "complex array round trip",
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

