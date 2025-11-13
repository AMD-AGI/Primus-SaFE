package stringUtil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnakeCaseToCamelCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "简单的蛇形命名",
			input:    "hello_world",
			expected: "helloWorld",
		},
		{
			name:     "多个下划线",
			input:    "this_is_a_test",
			expected: "thisIsATest",
		},
		{
			name:     "单个单词",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "两个单词",
			input:    "user_name",
			expected: "userName",
		},
		{
			name:     "包含数字",
			input:    "user_id_123",
			expected: "userId123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SnakeCaseToCamelCase(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSnakeCaseToUpperCamelCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "简单的蛇形命名",
			input:    "hello_world",
			expected: "HelloWorld",
		},
		{
			name:     "多个下划线",
			input:    "this_is_a_test",
			expected: "ThisIsATest",
		},
		{
			name:     "单个单词",
			input:    "hello",
			expected: "Hello",
		},
		{
			name:     "两个单词",
			input:    "user_name",
			expected: "UserName",
		},
		{
			name:     "包含数字",
			input:    "user_id_123",
			expected: "UserId123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SnakeCaseToUpperCamelCase(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCamelCaseToSnakeCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "简单的驼峰命名",
			input:    "helloWorld",
			expected: "hello_world",
		},
		{
			name:     "大写开头的驼峰",
			input:    "HelloWorld",
			expected: "hello_world",
		},
		{
			name:     "多个大写字母",
			input:    "thisIsATest",
			expected: "this_is_a_test",
		},
		{
			name:     "单个单词小写",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "单个单词大写",
			input:    "Hello",
			expected: "hello",
		},
		{
			name:     "包含数字",
			input:    "userId123",
			expected: "user_id123",
		},
		{
			name:     "连续大写字母",
			input:    "HTTPServer",
			expected: "h_t_t_p_server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CamelCaseToSnakeCase(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoundTripConversion(t *testing.T) {
	tests := []struct {
		name      string
		snakeCase string
		camelCase string
	}{
		{
			name:      "简单转换",
			snakeCase: "hello_world",
			camelCase: "helloWorld",
		},
		{
			name:      "多单词转换",
			snakeCase: "this_is_a_test",
			camelCase: "thisIsATest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// snake -> camel -> snake
			camel := SnakeCaseToCamelCase(tt.snakeCase)
			assert.Equal(t, tt.camelCase, camel)

			snake := CamelCaseToSnakeCase(camel)
			assert.Equal(t, tt.snakeCase, snake)
		})
	}
}

