package stringUtil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIn(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		strs     []string
		expected bool
	}{
		{
			name:     "string exists in list",
			s:        "apple",
			strs:     []string{"apple", "banana", "orange"},
			expected: true,
		},
		{
			name:     "string does not exist in list",
			s:        "grape",
			strs:     []string{"apple", "banana", "orange"},
			expected: false,
		},
		{
			name:     "empty list",
			s:        "apple",
			strs:     []string{},
			expected: false,
		},
		{
			name:     "empty string exists in list",
			s:        "",
			strs:     []string{"", "apple"},
			expected: true,
		},
		{
			name:     "empty string does not exist in list",
			s:        "",
			strs:     []string{"apple", "banana"},
			expected: false,
		},
		{
			name:     "single element list - match",
			s:        "apple",
			strs:     []string{"apple"},
			expected: true,
		},
		{
			name:     "single element list - no match",
			s:        "banana",
			strs:     []string{"apple"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := In(tt.s, tt.strs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSliceEqual(t *testing.T) {
	tests := []struct {
		name     string
		s1       []string
		s2       []string
		expected bool
	}{
		{
			name:     "two identical lists",
			s1:       []string{"apple", "banana", "orange"},
			s2:       []string{"apple", "banana", "orange"},
			expected: true,
		},
		{
			name:     "two lists with same elements but different order",
			s1:       []string{"apple", "banana", "orange"},
			s2:       []string{"orange", "banana", "apple"},
			expected: true,
		},
		{
			name:     "lists with different lengths",
			s1:       []string{"apple", "banana"},
			s2:       []string{"apple", "banana", "orange"},
			expected: false,
		},
		{
			name:     "lists with different elements",
			s1:       []string{"apple", "banana"},
			s2:       []string{"apple", "grape"},
			expected: false,
		},
		{
			name:     "two empty lists",
			s1:       []string{},
			s2:       []string{},
			expected: true,
		},
		{
			name:     "one empty list and one non-empty list",
			s1:       []string{},
			s2:       []string{"apple"},
			expected: false,
		},
		{
			name:     "list with duplicate elements",
			s1:       []string{"apple", "apple", "banana"},
			s2:       []string{"apple", "banana"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceEqual(tt.s1, tt.s2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSubset(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "a is subset of b",
			a:        []string{"apple", "banana"},
			b:        []string{"apple", "banana", "orange", "grape"},
			expected: true,
		},
		{
			name:     "a is not subset of b",
			a:        []string{"apple", "mango"},
			b:        []string{"apple", "banana", "orange"},
			expected: false,
		},
		{
			name:     "a and b are identical",
			a:        []string{"apple", "banana"},
			b:        []string{"apple", "banana"},
			expected: true,
		},
		{
			name:     "a is empty set",
			a:        []string{},
			b:        []string{"apple", "banana"},
			expected: true,
		},
		{
			name:     "b is empty but a is not",
			a:        []string{"apple"},
			b:        []string{},
			expected: false,
		},
		{
			name:     "both a and b are empty",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "a contains duplicate elements all in b",
			a:        []string{"apple", "apple"},
			b:        []string{"apple", "banana"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSubset(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

