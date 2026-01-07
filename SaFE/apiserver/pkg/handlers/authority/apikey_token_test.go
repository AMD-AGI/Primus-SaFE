/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// TestGenerateApiKey tests the API key generation function
func TestGenerateApiKey(t *testing.T) {
	tests := []struct {
		name     string
		validate func(*testing.T, string, error)
	}{
		{
			name: "successful generation",
			validate: func(t *testing.T, key string, err error) {
				assert.NoError(t, err)
				assert.NotEmpty(t, key)
				assert.True(t, IsApiKey(key))
				assert.True(t, len(key) > len(ApiKeyPrefix))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := GenerateApiKey()
			tt.validate(t, key, err)
		})
	}

	// Test uniqueness
	t.Run("generates unique keys", func(t *testing.T) {
		keys := make(map[string]bool)
		for i := 0; i < 100; i++ {
			key, err := GenerateApiKey()
			assert.NoError(t, err)
			assert.False(t, keys[key], "duplicate key generated")
			keys[key] = true
		}
	})
}

// TestHashApiKey tests the API key hashing function
func TestHashApiKey(t *testing.T) {
	testSecret := []byte("test-secret-1234")

	tests := []struct {
		name   string
		apiKey string
	}{
		{
			name:   "standard api key",
			apiKey: "ak-dGVzdC1rZXktMTIzNDU2Nzg5MA",
		},
		{
			name:   "short api key",
			apiKey: "ak-abc",
		},
		{
			name:   "empty string",
			apiKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := HashApiKey(tt.apiKey, testSecret)
			// Hash should be 64 characters (HMAC-SHA-256 produces 32 bytes = 64 hex chars)
			assert.Equal(t, 64, len(hash))
			// Same input should produce same hash
			assert.Equal(t, hash, HashApiKey(tt.apiKey, testSecret))
		})
	}

	// Test that different inputs produce different hashes
	t.Run("different inputs produce different hashes", func(t *testing.T) {
		hash1 := HashApiKey("ak-key-1", testSecret)
		hash2 := HashApiKey("ak-key-2", testSecret)
		assert.NotEqual(t, hash1, hash2)
	})

	// Test that hash is deterministic
	t.Run("hash is deterministic", func(t *testing.T) {
		apiKey := "ak-dGVzdC1rZXktMTIzNDU2Nzg5MA"
		for i := 0; i < 100; i++ {
			assert.Equal(t, HashApiKey(apiKey, testSecret), HashApiKey(apiKey, testSecret))
		}
	})

	// Test with nil secret (fallback to SHA-256)
	t.Run("nil secret uses SHA-256 fallback", func(t *testing.T) {
		apiKey := "ak-test-key"
		hash := HashApiKey(apiKey, nil)
		assert.Equal(t, 64, len(hash))
		assert.Equal(t, hash, HashApiKey(apiKey, nil))
	})

	// Test that different secrets produce different hashes
	t.Run("different secrets produce different hashes", func(t *testing.T) {
		apiKey := "ak-test-key"
		secret1 := []byte("secret-1")
		secret2 := []byte("secret-2")
		hash1 := HashApiKey(apiKey, secret1)
		hash2 := HashApiKey(apiKey, secret2)
		assert.NotEqual(t, hash1, hash2)
	})
}

// TestGenerateKeyHint tests the key hint generation
func TestGenerateKeyHint(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "standard api key",
			apiKey:   "ak-dGVzdC1rZXktMTIzNDU2Nzg5MA",
			expected: "dG-g5MA", // first 2 + last 4 of body
		},
		{
			name:     "short key body",
			apiKey:   "ak-abc",
			expected: "abc", // too short, return as-is
		},
		{
			name:     "minimum length key",
			apiKey:   "ak-123456",
			expected: "12-3456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := GenerateKeyHint(tt.apiKey)
			assert.Equal(t, tt.expected, hint)
		})
	}
}

// TestFormatKeyHint tests the key hint formatting for display
func TestFormatKeyHint(t *testing.T) {
	tests := []struct {
		name     string
		hint     string
		expected string
	}{
		{
			name:     "standard hint",
			hint:     "dG-890A",
			expected: "ak-dG****890A",
		},
		{
			name:     "empty hint",
			hint:     "",
			expected: "",
		},
		{
			name:     "hint without separator",
			hint:     "abc",
			expected: "ak-abc",
		},
		{
			name:     "minimum hint",
			hint:     "12-3456",
			expected: "ak-12****3456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := FormatKeyHint(tt.hint)
			assert.Equal(t, tt.expected, formatted)
		})
	}
}

// TestIsApiKey tests the API key format validation
func TestIsApiKey(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "valid api key",
			token:    "ak-abc123xyz",
			expected: true,
		},
		{
			name:     "valid generated key",
			token:    "ak-dGVzdC1rZXktMTIzNDU2Nzg5MA",
			expected: true,
		},
		{
			name:     "invalid prefix - sk",
			token:    "sk-abc123xyz",
			expected: false,
		},
		{
			name:     "no prefix",
			token:    "abc123xyz",
			expected: false,
		},
		{
			name:     "empty string",
			token:    "",
			expected: false,
		},
		{
			name:     "user token format",
			token:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: false,
		},
		{
			name:     "only prefix",
			token:    "ak-",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsApiKey(tt.token)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractApiKeyFromRequest tests extracting API key from Authorization header
func TestExtractApiKeyFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		expected   string
	}{
		{
			name:       "valid bearer with api key",
			authHeader: "Bearer ak-test123456",
			expected:   "ak-test123456",
		},
		{
			name:       "bearer lowercase",
			authHeader: "bearer ak-test123456",
			expected:   "ak-test123456",
		},
		{
			name:       "bearer with user token (not api key)",
			authHeader: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected:   "",
		},
		{
			name:       "empty header",
			authHeader: "",
			expected:   "",
		},
		{
			name:       "bearer without token",
			authHeader: "Bearer ",
			expected:   "",
		},
		{
			name:       "bearer only",
			authHeader: "Bearer",
			expected:   "",
		},
		{
			name:       "basic auth",
			authHeader: "Basic dXNlcjpwYXNz",
			expected:   "",
		},
		{
			name:       "extra spaces",
			authHeader: "Bearer  ak-test123456",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractApiKeyFromRequest(tt.authHeader)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestValidateWhitelist tests the whitelist validation function
func TestValidateWhitelist(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		expectErr bool
	}{
		{
			name:      "empty whitelist",
			whitelist: []string{},
			expectErr: false,
		},
		{
			name:      "valid single IP",
			whitelist: []string{"192.168.1.1"},
			expectErr: false,
		},
		{
			name:      "valid multiple IPs",
			whitelist: []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"},
			expectErr: false,
		},
		{
			name:      "valid CIDR",
			whitelist: []string{"192.168.1.0/24"},
			expectErr: false,
		},
		{
			name:      "valid mixed IPs and CIDRs",
			whitelist: []string{"192.168.1.1", "10.0.0.0/8", "172.16.0.1"},
			expectErr: false,
		},
		{
			name:      "valid IPv6",
			whitelist: []string{"::1", "2001:db8::1"},
			expectErr: false,
		},
		{
			name:      "valid IPv6 CIDR",
			whitelist: []string{"2001:db8::/32"},
			expectErr: false,
		},
		{
			name:      "invalid IP format",
			whitelist: []string{"invalid-ip"},
			expectErr: true,
		},
		{
			name:      "invalid CIDR format",
			whitelist: []string{"192.168.1.0/33"},
			expectErr: true,
		},
		{
			name:      "partial invalid",
			whitelist: []string{"192.168.1.1", "invalid", "10.0.0.1"},
			expectErr: true,
		},
		{
			name:      "empty entry ignored",
			whitelist: []string{"192.168.1.1", "", "10.0.0.1"},
			expectErr: false,
		},
		{
			name:      "whitespace entry ignored",
			whitelist: []string{"192.168.1.1", "   ", "10.0.0.1"},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWhitelist(tt.whitelist)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMaskApiKey tests the API key masking function
func TestMaskApiKey(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "normal length key",
			apiKey:   "ak-dGVzdC1rZXktMTIzNDU2Nzg5MA",
			expected: "ak-dGVzd***g5MA",
		},
		{
			name:     "short key",
			apiKey:   "ak-abc",
			expected: "***",
		},
		{
			name:     "very short key",
			apiKey:   "ak-",
			expected: "***",
		},
		{
			name:     "empty key",
			apiKey:   "",
			expected: "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskApiKey(tt.apiKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestApiKeyTokenValidateApiKey tests the API key validation logic
func TestApiKeyTokenValidateApiKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	// Create a test instance (bypass singleton for testing)
	apiKeyToken := &ApiKeyToken{
		dbClient: mockDB,
	}

	now := time.Now().UTC()

	tests := []struct {
		name      string
		apiKey    string
		clientIP  string
		setupMock func()
		expectErr bool
		errMsg    string
	}{
		{
			name:     "valid api key without whitelist",
			apiKey:   "ak-valid-key-123",
			clientIP: "192.168.1.100",
			setupMock: func() {
				// Database stores hashed key, so mock expects the hash
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-valid-key-123", nil)).Return(&dbclient.ApiKey{
					Id:             1,
					UserId:         "user-123",
					ApiKey:         HashApiKey("ak-valid-key-123", nil),
					Deleted:        false,
					ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
					Whitelist:      "[]",
				}, nil)
			},
			expectErr: false,
		},
		{
			name:     "valid api key with matching IP in whitelist",
			apiKey:   "ak-valid-key-456",
			clientIP: "192.168.1.100",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-valid-key-456", nil)).Return(&dbclient.ApiKey{
					Id:             2,
					UserId:         "user-456",
					ApiKey:         HashApiKey("ak-valid-key-456", nil),
					Deleted:        false,
					ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
					Whitelist:      `["192.168.1.100", "10.0.0.0/8"]`,
				}, nil)
			},
			expectErr: false,
		},
		{
			name:     "valid api key with matching CIDR in whitelist",
			apiKey:   "ak-valid-key-789",
			clientIP: "10.0.0.50",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-valid-key-789", nil)).Return(&dbclient.ApiKey{
					Id:             3,
					UserId:         "user-789",
					ApiKey:         HashApiKey("ak-valid-key-789", nil),
					Deleted:        false,
					ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
					Whitelist:      `["10.0.0.0/8"]`,
				}, nil)
			},
			expectErr: false,
		},
		{
			name:     "api key not found",
			apiKey:   "ak-notfound",
			clientIP: "192.168.1.100",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-notfound", nil)).Return(nil, sql.ErrNoRows)
			},
			expectErr: true,
			errMsg:    "invalid API key",
		},
		{
			name:     "deleted api key",
			apiKey:   "ak-deleted",
			clientIP: "192.168.1.100",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-deleted", nil)).Return(&dbclient.ApiKey{
					Id:             4,
					UserId:         "user-deleted",
					ApiKey:         HashApiKey("ak-deleted", nil),
					Deleted:        true,
					ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
					Whitelist:      "[]",
				}, nil)
			},
			expectErr: true,
			errMsg:    "Unavailable",
		},
		{
			name:     "expired api key",
			apiKey:   "ak-expired",
			clientIP: "192.168.1.100",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-expired", nil)).Return(&dbclient.ApiKey{
					Id:             5,
					UserId:         "user-expired",
					ApiKey:         HashApiKey("ak-expired", nil),
					Deleted:        false,
					ExpirationTime: pq.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
					Whitelist:      "[]",
				}, nil)
			},
			expectErr: true,
			errMsg:    "API key expired",
		},
		{
			name:     "ip not in whitelist",
			apiKey:   "ak-ip-blocked",
			clientIP: "192.168.2.100",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-ip-blocked", nil)).Return(&dbclient.ApiKey{
					Id:             6,
					UserId:         "user-blocked",
					ApiKey:         HashApiKey("ak-ip-blocked", nil),
					Deleted:        false,
					ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
					Whitelist:      `["192.168.1.0/24"]`,
				}, nil)
			},
			expectErr: true,
			errMsg:    "IP not allowed",
		},
		{
			name:     "empty whitelist allows all",
			apiKey:   "ak-empty-whitelist",
			clientIP: "1.2.3.4",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-empty-whitelist", nil)).Return(&dbclient.ApiKey{
					Id:             7,
					UserId:         "user-any",
					ApiKey:         HashApiKey("ak-empty-whitelist", nil),
					Deleted:        false,
					ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
					Whitelist:      "",
				}, nil)
			},
			expectErr: false,
		},
		{
			name:     "null whitelist allows all",
			apiKey:   "ak-null-whitelist",
			clientIP: "1.2.3.4",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-null-whitelist", nil)).Return(&dbclient.ApiKey{
					Id:             8,
					UserId:         "user-null",
					ApiKey:         HashApiKey("ak-null-whitelist", nil),
					Deleted:        false,
					ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
					Whitelist:      "null",
				}, nil)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			userInfo, err := apiKeyToken.ValidateApiKey(context.Background(), tt.apiKey, tt.clientIP)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, userInfo)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, userInfo)
			}
		})
	}
}

// TestApiKeyTokenCheckIPWhitelist tests the IP whitelist checking logic
func TestApiKeyTokenCheckIPWhitelist(t *testing.T) {
	apiKeyToken := &ApiKeyToken{}

	tests := []struct {
		name          string
		whitelistJSON string
		clientIP      string
		expectErr     bool
	}{
		{
			name:          "empty whitelist",
			whitelistJSON: "",
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "null whitelist",
			whitelistJSON: "null",
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "empty array whitelist",
			whitelistJSON: "[]",
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "exact IP match",
			whitelistJSON: `["192.168.1.100"]`,
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "CIDR match",
			whitelistJSON: `["192.168.1.0/24"]`,
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "multiple entries - first match",
			whitelistJSON: `["192.168.1.100", "10.0.0.1"]`,
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "multiple entries - second match",
			whitelistJSON: `["10.0.0.1", "192.168.1.100"]`,
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "no match",
			whitelistJSON: `["192.168.1.0/24"]`,
			clientIP:      "10.0.0.1",
			expectErr:     true,
		},
		{
			name:          "IP with port",
			whitelistJSON: `["192.168.1.100"]`,
			clientIP:      "192.168.1.100:8080",
			expectErr:     false,
		},
		{
			name:          "IPv6 match",
			whitelistJSON: `["::1"]`,
			clientIP:      "::1",
			expectErr:     false,
		},
		{
			name:          "invalid client IP",
			whitelistJSON: `["192.168.1.100"]`,
			clientIP:      "invalid-ip",
			expectErr:     true,
		},
		{
			name:          "malformed JSON - fail close",
			whitelistJSON: `{invalid`,
			clientIP:      "192.168.1.100",
			expectErr:     true,
		},
		{
			name:          "invalid CIDR in whitelist - skipped",
			whitelistJSON: `["192.168.1.0/99", "10.0.0.1"]`,
			clientIP:      "10.0.0.1",
			expectErr:     false,
		},
		{
			name:          "whitespace in entries",
			whitelistJSON: `["  192.168.1.100  "]`,
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "empty entry in whitelist",
			whitelistJSON: `["", "192.168.1.100"]`,
			clientIP:      "192.168.1.100",
			expectErr:     false,
		},
		{
			name:          "IP with invalid port format",
			whitelistJSON: `["192.168.1.100"]`,
			clientIP:      "192.168.1.100::",
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apiKeyToken.checkIPWhitelist(tt.whitelistJSON, tt.clientIP)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestApiKeyTokenNilDbClient tests behavior when database client is nil
func TestApiKeyTokenNilDbClient(t *testing.T) {
	apiKeyToken := &ApiKeyToken{
		dbClient: nil,
	}

	userInfo, err := apiKeyToken.ValidateApiKey(context.Background(), "ak-test", "192.168.1.1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database client not initialized")
	assert.Nil(t, userInfo)
}
