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
			name: "正常解码-简单字符串",
			data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("secret123"),
			},
			key:         "username",
			expected:    "admin",
			expectError: false,
		},
		{
			name: "正常解码-密码",
			data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("secret123"),
			},
			key:         "password",
			expected:    "secret123",
			expectError: false,
		},
		{
			name: "键不存在",
			data: map[string][]byte{
				"username": []byte("admin"),
			},
			key:         "password",
			expected:    "",
			expectError: true,
		},
		{
			name: "空值",
			data: map[string][]byte{
				"username": []byte(""),
			},
			key:         "username",
			expected:    "",
			expectError: false,
		},
		{
			name:        "空map",
			data:        map[string][]byte{},
			key:         "username",
			expected:    "",
			expectError: true,
		},
		{
			name: "包含特殊字符",
			data: map[string][]byte{
				"host": []byte("postgres.example.com:5432"),
			},
			key:         "host",
			expected:    "postgres.example.com:5432",
			expectError: false,
		},
		{
			name: "包含中文字符",
			data: map[string][]byte{
				"description": []byte("数据库连接配置"),
			},
			key:         "description",
			expected:    "数据库连接配置",
			expectError: false,
		},
		{
			name: "包含换行符",
			data: map[string][]byte{
				"multiline": []byte("line1\nline2\nline3"),
			},
			key:         "multiline",
			expected:    "line1\nline2\nline3",
			expectError: false,
		},
		{
			name: "包含空格",
			data: map[string][]byte{
				"spaced": []byte("  value with spaces  "),
			},
			key:         "spaced",
			expected:    "  value with spaces  ",
			expectError: false,
		},
		{
			name: "数字字符串",
			data: map[string][]byte{
				"port": []byte("5432"),
			},
			key:         "port",
			expected:    "5432",
			expectError: false,
		},
		{
			name: "布尔字符串",
			data: map[string][]byte{
				"enabled": []byte("true"),
			},
			key:         "enabled",
			expected:    "true",
			expectError: false,
		},
		{
			name: "JSON字符串",
			data: map[string][]byte{
				"config": []byte(`{"key": "value", "number": 123}`),
			},
			key:         "config",
			expected:    `{"key": "value", "number": 123}`,
			expectError: false,
		},
		{
			name: "Base64编码的数据",
			data: map[string][]byte{
				"token": []byte("dGVzdC10b2tlbi0xMjM0NTY="),
			},
			key:         "token",
			expected:    "dGVzdC10b2tlbi0xMjM0NTY=",
			expectError: false,
		},
		{
			name: "包含特殊符号",
			data: map[string][]byte{
				"special": []byte("!@#$%^&*()_+-=[]{}|;:,.<>?"),
			},
			key:         "special",
			expected:    "!@#$%^&*()_+-=[]{}|;:,.<>?",
			expectError: false,
		},
		{
			name: "nil byte数组",
			data: map[string][]byte{
				"nil_value": nil,
			},
			key:         "nil_value",
			expected:    "",
			expectError: false,
		},
		{
			name: "大小写敏感的键",
			data: map[string][]byte{
				"Username": []byte("admin"),
				"username": []byte("user"),
			},
			key:         "username",
			expected:    "user",
			expectError: false,
		},
		{
			name: "键名包含特殊字符",
			data: map[string][]byte{
				"db.host": []byte("localhost"),
			},
			key:         "db.host",
			expected:    "localhost",
			expectError: false,
		},
		{
			name: "长字符串值",
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
			name: "PostgreSQL配置-完整数据",
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
			name: "PostgreSQL配置-获取端口",
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
			name: "PostgreSQL配置-缺少必需字段",
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

