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
			name:     "字符串存在于列表中",
			s:        "apple",
			strs:     []string{"apple", "banana", "orange"},
			expected: true,
		},
		{
			name:     "字符串不存在于列表中",
			s:        "grape",
			strs:     []string{"apple", "banana", "orange"},
			expected: false,
		},
		{
			name:     "空列表",
			s:        "apple",
			strs:     []string{},
			expected: false,
		},
		{
			name:     "空字符串存在于列表中",
			s:        "",
			strs:     []string{"", "apple"},
			expected: true,
		},
		{
			name:     "空字符串不存在于列表中",
			s:        "",
			strs:     []string{"apple", "banana"},
			expected: false,
		},
		{
			name:     "单个元素列表-匹配",
			s:        "apple",
			strs:     []string{"apple"},
			expected: true,
		},
		{
			name:     "单个元素列表-不匹配",
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
			name:     "两个相同的列表",
			s1:       []string{"apple", "banana", "orange"},
			s2:       []string{"apple", "banana", "orange"},
			expected: true,
		},
		{
			name:     "两个相同元素但顺序不同的列表",
			s1:       []string{"apple", "banana", "orange"},
			s2:       []string{"orange", "banana", "apple"},
			expected: true,
		},
		{
			name:     "长度不同的列表",
			s1:       []string{"apple", "banana"},
			s2:       []string{"apple", "banana", "orange"},
			expected: false,
		},
		{
			name:     "元素不同的列表",
			s1:       []string{"apple", "banana"},
			s2:       []string{"apple", "grape"},
			expected: false,
		},
		{
			name:     "两个空列表",
			s1:       []string{},
			s2:       []string{},
			expected: true,
		},
		{
			name:     "一个空列表和一个非空列表",
			s1:       []string{},
			s2:       []string{"apple"},
			expected: false,
		},
		{
			name:     "包含重复元素的列表",
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
			name:     "a是b的子集",
			a:        []string{"apple", "banana"},
			b:        []string{"apple", "banana", "orange", "grape"},
			expected: true,
		},
		{
			name:     "a不是b的子集",
			a:        []string{"apple", "mango"},
			b:        []string{"apple", "banana", "orange"},
			expected: false,
		},
		{
			name:     "a和b相同",
			a:        []string{"apple", "banana"},
			b:        []string{"apple", "banana"},
			expected: true,
		},
		{
			name:     "a为空集",
			a:        []string{},
			b:        []string{"apple", "banana"},
			expected: true,
		},
		{
			name:     "b为空集但a不为空",
			a:        []string{"apple"},
			b:        []string{},
			expected: false,
		},
		{
			name:     "a和b都为空",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "a包含重复元素都在b中",
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

