package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeSecretData(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string][]byte
		key         string
		expected    string
		expectError bool
	}{
		{
			name: "normal decoding - simple string",
			data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("secret123"),
			},
			key:         "username",
			expected:    "admin",
			expectError: false,
		},
		{
			name: "normal decoding - password",
			data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("secret123"),
			},
			key:         "password",
			expected:    "secret123",
			expectError: false,
		},
		{
			name: "key not exists",
			data: map[string][]byte{
				"username": []byte("admin"),
			},
			key:         "password",
			expected:    "",
			expectError: true,
		},
		{
			name: "empty value",
			data: map[string][]byte{
				"username": []byte(""),
			},
			key:         "username",
			expected:    "",
			expectError: false,
		},
		{
			name:        "empty map",
			data:        map[string][]byte{},
			key:         "username",
			expected:    "",
			expectError: true,
		},
		{
			name: "contains special characters",
			data: map[string][]byte{
				"host": []byte("postgres.example.com:5432"),
			},
			key:         "host",
			expected:    "postgres.example.com:5432",
			expectError: false,
		},
		{
			name: "contains UTF-8 characters",
			data: map[string][]byte{
				"description": []byte("database connection config"),
			},
			key:         "description",
			expected:    "database connection config",
			expectError: false,
		},
		{
			name: "contains newlines",
			data: map[string][]byte{
				"multiline": []byte("line1\nline2\nline3"),
			},
			key:         "multiline",
			expected:    "line1\nline2\nline3",
			expectError: false,
		},
		{
			name: "contains spaces",
			data: map[string][]byte{
				"spaced": []byte("  value with spaces  "),
			},
			key:         "spaced",
			expected:    "  value with spaces  ",
			expectError: false,
		},
		{
			name: "numeric string",
			data: map[string][]byte{
				"port": []byte("5432"),
			},
			key:         "port",
			expected:    "5432",
			expectError: false,
		},
		{
			name: "boolean string",
			data: map[string][]byte{
				"enabled": []byte("true"),
			},
			key:         "enabled",
			expected:    "true",
			expectError: false,
		},
		{
			name: "JSON string",
			data: map[string][]byte{
				"config": []byte(`{"key": "value", "number": 123}`),
			},
			key:         "config",
			expected:    `{"key": "value", "number": 123}`,
			expectError: false,
		},
		{
			name: "Base64 encoded data",
			data: map[string][]byte{
				"token": []byte("dGVzdC10b2tlbi0xMjM0NTY="),
			},
			key:         "token",
			expected:    "dGVzdC10b2tlbi0xMjM0NTY=",
			expectError: false,
		},
		{
			name: "contains special symbols",
			data: map[string][]byte{
				"special": []byte("!@#$%^&*()_+-=[]{}|;:,.<>?"),
			},
			key:         "special",
			expected:    "!@#$%^&*()_+-=[]{}|;:,.<>?",
			expectError: false,
		},
		{
			name: "nil byte array",
			data: map[string][]byte{
				"nil_value": nil,
			},
			key:         "nil_value",
			expected:    "",
			expectError: false,
		},
		{
			name: "case sensitive keys",
			data: map[string][]byte{
				"Username": []byte("admin"),
				"username": []byte("user"),
			},
			key:         "username",
			expected:    "user",
			expectError: false,
		},
		{
			name: "key name contains special characters",
			data: map[string][]byte{
				"db.host": []byte("localhost"),
			},
			key:         "db.host",
			expected:    "localhost",
			expectError: false,
		},
		{
			name: "long string value",
			data: map[string][]byte{
				"long": []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."),
			},
			key:         "long",
			expected:    "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeSecretData(tt.data, tt.key)

			if tt.expectError {
				assert.Error(t, err, "Expected an error but got none")
				assert.Contains(t, err.Error(), "not found", "Error message should indicate key not found")
			} else {
				assert.NoError(t, err, "Expected no error but got one")
				assert.Equal(t, tt.expected, result, "Decoded value mismatch")
			}
		})
	}
}

func TestDecodeSecretData_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string][]byte
		key         string
		expected    string
		expectError bool
	}{
		{
			name: "PostgreSQL config - complete data",
			data: map[string][]byte{
				"dbname":   []byte("primus_safe"),
				"host":     []byte("postgres.primus-safe.svc.cluster.local"),
				"password": []byte("P@ssw0rd!2024"),
				"port":     []byte("5432"),
				"user":     []byte("primus_safe_user"),
			},
			key:         "host",
			expected:    "postgres.primus-safe.svc.cluster.local",
			expectError: false,
		},
		{
			name: "PostgreSQL config - get port",
			data: map[string][]byte{
				"dbname":   []byte("primus_safe"),
				"host":     []byte("postgres.primus-safe.svc.cluster.local"),
				"password": []byte("P@ssw0rd!2024"),
				"port":     []byte("5432"),
				"user":     []byte("primus_safe_user"),
			},
			key:         "port",
			expected:    "5432",
			expectError: false,
		},
		{
			name: "PostgreSQL config - missing required field",
			data: map[string][]byte{
				"dbname": []byte("primus_safe"),
				"host":   []byte("postgres.primus-safe.svc.cluster.local"),
			},
			key:         "password",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeSecretData(tt.data, tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
