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
			name:     "simple snake case",
			input:    "hello_world",
			expected: "helloWorld",
		},
		{
			name:     "multiple underscores",
			input:    "this_is_a_test",
			expected: "thisIsATest",
		},
		{
			name:     "single word",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "two words",
			input:    "user_name",
			expected: "userName",
		},
		{
			name:     "contains numbers",
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
			name:     "simple snake case",
			input:    "hello_world",
			expected: "HelloWorld",
		},
		{
			name:     "multiple underscores",
			input:    "this_is_a_test",
			expected: "ThisIsATest",
		},
		{
			name:     "single word",
			input:    "hello",
			expected: "Hello",
		},
		{
			name:     "two words",
			input:    "user_name",
			expected: "UserName",
		},
		{
			name:     "contains numbers",
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
			name:     "simple camel case",
			input:    "helloWorld",
			expected: "hello_world",
		},
		{
			name:     "upper camel case",
			input:    "HelloWorld",
			expected: "hello_world",
		},
		{
			name:     "multiple capital letters",
			input:    "thisIsATest",
			expected: "this_is_a_test",
		},
		{
			name:     "single word lowercase",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "single word uppercase",
			input:    "Hello",
			expected: "hello",
		},
		{
			name:     "contains numbers",
			input:    "userId123",
			expected: "user_id123",
		},
		{
			name:     "consecutive uppercase letters",
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
			name:      "simple conversion",
			snakeCase: "hello_world",
			camelCase: "helloWorld",
		},
		{
			name:      "multi-word conversion",
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

