/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"database/sql"
	"fmt"
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
		name string
		key  string
	}{
		{
			name: "standard api key",
			key:  "ak-dGVzdC1rZXktMTIzNDU2Nzg5MA",
		},
		{
			name: "short api key",
			key:  "ak-abc",
		},
		{
			name: "empty string",
			key:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := HashApiKey(tt.key, testSecret)
			// Hash should be 64 characters (HMAC-SHA-256 produces 32 bytes = 64 hex chars)
			assert.Equal(t, 64, len(hash))
			// Same input should produce same hash
			assert.Equal(t, hash, HashApiKey(tt.key, testSecret))
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
		key := "ak-test1234xyz1"
		for i := 0; i < 100; i++ {
			assert.Equal(t, HashApiKey(key, testSecret), HashApiKey(key, testSecret))
		}
	})

	// Test with nil secret (fallback to SHA-256)
	t.Run("nil secret uses SHA-256 fallback", func(t *testing.T) {
		key := "ak-test-key"
		hash := HashApiKey(key, nil)
		assert.Equal(t, 64, len(hash))
		assert.Equal(t, hash, HashApiKey(key, nil))
	})

	// Test that different secrets produce different hashes
	t.Run("different secrets produce different hashes", func(t *testing.T) {
		key := "ak-test-key"
		secret1 := []byte("secret-1")
		secret2 := []byte("secret-2")
		hash1 := HashApiKey(key, secret1)
		hash2 := HashApiKey(key, secret2)
		assert.NotEqual(t, hash1, hash2)
	})
}

// TestGenerateKeyHint tests the key hint generation
func TestGenerateKeyHint(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "standard api key",
			key:      "ak-1234abcdxyz1",
			expected: "ak-12****xyz1", // ak- + first 2 + **** + last 4
		},
		{
			name:     "short key body",
			key:      "ak-abc",
			expected: "ak-abc", // too short, just add prefix
		},
		{
			name:     "minimum length key exactly 6 chars",
			key:      "ak-123456",
			expected: "ak-12****3456",
		},
		{
			name:     "key with dash in last 4 chars",
			key:      "ak-12345abcdefghxyz1",
			expected: "ak-12****xyz1", // correctly handles dash in last 4
		},
		{
			name:     "key with underscore in body",
			key:      "ak-abc_def_ghi_jkl",
			expected: "ak-ab****_jkl", // correctly handles underscore
		},
		{
			name:     "key with multiple dashes",
			key:      "ak-a-b-c-d-e-f-g",
			expected: "ak-a-****-f-g", // multiple dashes
		},
		{
			name:     "key ending with dash",
			key:      "ak-abcdefghij-",
			expected: "ak-ab****hij-", // dash at end (last 4: h,i,j,-)
		},
		{
			name:     "empty key body",
			key:      "ak-",
			expected: "ak-", // empty body
		},
		{
			name:     "empty string",
			key:      "",
			expected: "ak-", // empty input
		},
		{
			name:     "5 char body (boundary)",
			key:      "ak-12345",
			expected: "ak-12345", // less than 6, return as-is with prefix
		},
		{
			name:     "7 char body",
			key:      "ak-1234567",
			expected: "ak-12****4567",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := GenerateKeyHint(tt.key)
			assert.Equal(t, tt.expected, hint)
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

// TestValidateAndDeduplicateWhitelist tests the whitelist validation and deduplication function
func TestValidateAndDeduplicateWhitelist(t *testing.T) {
	tests := []struct {
		name           string
		whitelist      []string
		expectErr      bool
		expectedResult []string
	}{
		{
			name:           "empty whitelist",
			whitelist:      []string{},
			expectErr:      false,
			expectedResult: []string{},
		},
		{
			name:           "valid single IP",
			whitelist:      []string{"192.168.1.1"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.1"},
		},
		{
			name:           "valid multiple IPs",
			whitelist:      []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"},
		},
		{
			name:           "valid CIDR",
			whitelist:      []string{"192.168.1.0/24"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.0/24"},
		},
		{
			name:           "valid mixed IPs and CIDRs",
			whitelist:      []string{"192.168.1.1", "10.0.0.0/8", "172.16.0.1"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.1", "10.0.0.0/8", "172.16.0.1"},
		},
		{
			name:           "valid IPv6",
			whitelist:      []string{"::1", "2001:db8::1"},
			expectErr:      false,
			expectedResult: []string{"::1", "2001:db8::1"},
		},
		{
			name:           "valid IPv6 CIDR",
			whitelist:      []string{"2001:db8::/32"},
			expectErr:      false,
			expectedResult: []string{"2001:db8::/32"},
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
			name:           "empty entry ignored",
			whitelist:      []string{"192.168.1.1", "", "10.0.0.1"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.1", "10.0.0.1"},
		},
		{
			name:           "whitespace entry ignored",
			whitelist:      []string{"192.168.1.1", "   ", "10.0.0.1"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.1", "10.0.0.1"},
		},
		{
			name:           "duplicate IPs removed",
			whitelist:      []string{"192.168.1.1", "10.0.0.1", "192.168.1.1", "10.0.0.1"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.1", "10.0.0.1"},
		},
		{
			name:           "duplicate CIDRs removed",
			whitelist:      []string{"192.168.1.0/24", "10.0.0.0/8", "192.168.1.0/24"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.0/24", "10.0.0.0/8"},
		},
		{
			name:           "duplicate mixed removed",
			whitelist:      []string{"192.168.1.1", "192.168.1.0/24", "192.168.1.1", "192.168.1.0/24"},
			expectErr:      false,
			expectedResult: []string{"192.168.1.1", "192.168.1.0/24"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAndDeduplicateWhitelist(tt.whitelist)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

// TestMaskApiKey tests the API key masking function
func TestMaskApiKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "normal length key",
			key:      "ak-1234abcdexyz1",
			expected: "ak-1234a***xyz1",
		},
		{
			name:     "short key",
			key:      "ak-abc",
			expected: "***",
		},
		{
			name:     "very short key",
			key:      "ak-",
			expected: "***",
		},
		{
			name:     "empty key",
			key:      "",
			expected: "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskApiKey(tt.key)
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
	inst := &ApiKeyToken{
		dbClient: mockDB,
	}

	now := time.Now().UTC()

	tests := []struct {
		name      string
		key       string
		clientIP  string
		setupMock func()
		expectErr bool
		errMsg    string
	}{
		{
			name:     "valid api key without whitelist",
			key:      "ak-valid-key-123",
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
			key:      "ak-valid-key-456",
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
			key:      "ak-valid-key-789",
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
			key:      "ak-notfound",
			clientIP: "192.168.1.100",
			setupMock: func() {
				mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-notfound", nil)).Return(nil, sql.ErrNoRows)
			},
			expectErr: true,
			errMsg:    "invalid API key",
		},
		{
			name:     "deleted api key",
			key:      "ak-deleted",
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
			key:      "ak-expired",
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
			key:      "ak-ip-blocked",
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
			key:      "ak-empty-whitelist",
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
			key:      "ak-null-whitelist",
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
			userInfo, err := inst.ValidateApiKey(context.Background(), tt.key, tt.clientIP)
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
	inst := &ApiKeyToken{}

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
			err := inst.checkIPWhitelist(tt.whitelistJSON, tt.clientIP)
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
	inst := &ApiKeyToken{
		dbClient: nil,
	}

	userInfo, err := inst.ValidateApiKey(context.Background(), "ak-test", "192.168.1.1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database client not initialized")
	assert.Nil(t, userInfo)
}

// TestEncryptDecryptApiKey tests AES-GCM encrypt/decrypt roundtrip
func TestEncryptDecryptApiKey(t *testing.T) {
	secret := []byte("test-secret-for-aes-encryption")

	t.Run("roundtrip succeeds", func(t *testing.T) {
		plaintext := "ak-test"
		encrypted, err := EncryptApiKey(plaintext, secret)
		assert.NoError(t, err)
		assert.NotEmpty(t, encrypted)
		assert.NotEqual(t, plaintext, encrypted)

		decrypted, err := DecryptApiKey(encrypted, secret)
		assert.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("different plaintexts produce different ciphertexts", func(t *testing.T) {
		enc1, err := EncryptApiKey("ak-key-1", secret)
		assert.NoError(t, err)
		enc2, err := EncryptApiKey("ak-key-2", secret)
		assert.NoError(t, err)
		assert.NotEqual(t, enc1, enc2)
	})

	t.Run("same plaintext produces different ciphertexts (random nonce)", func(t *testing.T) {
		enc1, err := EncryptApiKey("ak-same-key", secret)
		assert.NoError(t, err)
		enc2, err := EncryptApiKey("ak-same-key", secret)
		assert.NoError(t, err)
		assert.NotEqual(t, enc1, enc2)

		dec1, _ := DecryptApiKey(enc1, secret)
		dec2, _ := DecryptApiKey(enc2, secret)
		assert.Equal(t, dec1, dec2)
	})

	t.Run("wrong secret fails to decrypt", func(t *testing.T) {
		encrypted, err := EncryptApiKey("ak-secret-test", secret)
		assert.NoError(t, err)

		_, err = DecryptApiKey(encrypted, []byte("wrong-secret"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt")
	})

	t.Run("invalid base64 fails to decrypt", func(t *testing.T) {
		_, err := DecryptApiKey("!!!invalid-base64!!!", secret)
		assert.Error(t, err)
	})

	t.Run("empty ciphertext fails to decrypt", func(t *testing.T) {
		_, err := DecryptApiKey("", secret)
		assert.Error(t, err)
	})

	t.Run("short ciphertext fails", func(t *testing.T) {
		_, err := DecryptApiKey("YWJj", secret)
		assert.Error(t, err)
	})

	t.Run("empty plaintext roundtrip", func(t *testing.T) {
		encrypted, err := EncryptApiKey("", secret)
		assert.NoError(t, err)
		decrypted, err := DecryptApiKey(encrypted, secret)
		assert.NoError(t, err)
		assert.Equal(t, "", decrypted)
	})
}

// TestDeriveAESKey tests the AES key derivation function
func TestDeriveAESKey(t *testing.T) {
	t.Run("produces 32-byte key", func(t *testing.T) {
		key := deriveAESKey([]byte("any-length-secret"))
		assert.Equal(t, 32, len(key))
	})

	t.Run("deterministic output", func(t *testing.T) {
		key1 := deriveAESKey([]byte("test-secret"))
		key2 := deriveAESKey([]byte("test-secret"))
		assert.Equal(t, key1, key2)
	})

	t.Run("different inputs produce different keys", func(t *testing.T) {
		key1 := deriveAESKey([]byte("secret-1"))
		key2 := deriveAESKey([]byte("secret-2"))
		assert.NotEqual(t, key1, key2)
	})
}

// TestValidateApiKeyPlatformKeySkipsExpiration tests that platform keys bypass expiration check
func TestValidateApiKeyPlatformKeySkipsExpiration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	inst := &ApiKeyToken{dbClient: mockDB}

	now := time.Now().UTC()

	t.Run("expired platform key is still valid", func(t *testing.T) {
		mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-platform-expired", nil)).Return(&dbclient.ApiKey{
			Id:             100,
			UserId:         "user-platform",
			UserName:       "platform-user",
			ApiKey:         HashApiKey("ak-platform-expired", nil),
			Deleted:        false,
			ExpirationTime: pq.NullTime{Time: now.Add(-48 * time.Hour), Valid: true},
			Whitelist:      "[]",
			KeyType:        KeyTypePlatform,
		}, nil)

		userInfo, err := inst.ValidateApiKey(context.Background(), "ak-platform-expired", "192.168.1.1")
		assert.NoError(t, err)
		assert.NotNil(t, userInfo)
		assert.Equal(t, "user-platform", userInfo.Id)
	})

	t.Run("expired user key is rejected", func(t *testing.T) {
		mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-user-expired", nil)).Return(&dbclient.ApiKey{
			Id:             101,
			UserId:         "user-regular",
			ApiKey:         HashApiKey("ak-user-expired", nil),
			Deleted:        false,
			ExpirationTime: pq.NullTime{Time: now.Add(-48 * time.Hour), Valid: true},
			Whitelist:      "[]",
			KeyType:        KeyTypeUser,
		}, nil)

		userInfo, err := inst.ValidateApiKey(context.Background(), "ak-user-expired", "192.168.1.1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API key expired")
		assert.Nil(t, userInfo)
	})

	t.Run("deleted platform key is still rejected", func(t *testing.T) {
		mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey("ak-platform-deleted", nil)).Return(&dbclient.ApiKey{
			Id:             102,
			UserId:         "user-platform",
			ApiKey:         HashApiKey("ak-platform-deleted", nil),
			Deleted:        true,
			ExpirationTime: pq.NullTime{Time: now.Add(24 * time.Hour), Valid: true},
			Whitelist:      "[]",
			KeyType:        KeyTypePlatform,
		}, nil)

		userInfo, err := inst.ValidateApiKey(context.Background(), "ak-platform-deleted", "192.168.1.1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Unavailable")
		assert.Nil(t, userInfo)
	})
}

// TestGetOrCreatePlatformKey tests the GetOrCreate logic for platform keys
func TestGetOrCreatePlatformKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	apiKeyToken := &ApiKeyToken{dbClient: mockDB}

	t.Run("nil db client returns error", func(t *testing.T) {
		nilToken := &ApiKeyToken{dbClient: nil}
		_, err := nilToken.GetOrCreatePlatformKey(context.Background(), "user-1", "testuser")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database client not initialized")
	})

	t.Run("returns existing platform key", func(t *testing.T) {
		plainKey := "ak-test-12345"
		encryptedKey, _ := EncryptApiKey(plainKey, nil)

		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-existing").Return(&dbclient.ApiKey{
			Id:           10,
			UserId:       "user-existing",
			KeyType:      KeyTypePlatform,
			EncryptedKey: &encryptedKey,
		}, nil)

		result, err := apiKeyToken.GetOrCreatePlatformKey(context.Background(), "user-existing", "testuser")
		assert.NoError(t, err)
		assert.Equal(t, plainKey, result)
	})

	t.Run("creates new platform key when not found", func(t *testing.T) {
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-new").Return(nil, sql.ErrNoRows)
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, apiKey *dbclient.ApiKey) error {
				assert.Equal(t, "user-new", apiKey.UserId)
				assert.Equal(t, "newuser", apiKey.UserName)
				assert.Equal(t, KeyTypePlatform, apiKey.KeyType)
				assert.Equal(t, PlatformKeyName, apiKey.Name)
				assert.NotNil(t, apiKey.EncryptedKey)
				assert.True(t, apiKey.ExpirationTime.Time.Year() == 9999)
				apiKey.Id = 99
				return nil
			},
		)

		result, err := apiKeyToken.GetOrCreatePlatformKey(context.Background(), "user-new", "newuser")
		assert.NoError(t, err)
		assert.True(t, IsApiKey(result))
	})

	t.Run("db query error returns error", func(t *testing.T) {
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-err").Return(nil, fmt.Errorf("db connection failed"))

		_, err := apiKeyToken.GetOrCreatePlatformKey(context.Background(), "user-err", "erruser")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query platform key")
	})

	t.Run("existing key with nil encrypted_key returns error", func(t *testing.T) {
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-nil-enc").Return(&dbclient.ApiKey{
			Id:           11,
			UserId:       "user-nil-enc",
			KeyType:      KeyTypePlatform,
			EncryptedKey: nil,
		}, nil)

		_, err := apiKeyToken.GetOrCreatePlatformKey(context.Background(), "user-nil-enc", "testuser")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "platform key has no encrypted value")
	})

	t.Run("insert failure returns error", func(t *testing.T) {
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-insert-fail").Return(nil, sql.ErrNoRows)
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Return(fmt.Errorf("unique constraint violation"))

		_, err := apiKeyToken.GetOrCreatePlatformKey(context.Background(), "user-insert-fail", "failuser")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create platform key")
	})
}

// TestPlatformKeyConstants tests that platform key constants are correctly defined
func TestPlatformKeyConstants(t *testing.T) {
	assert.Equal(t, "user", KeyTypeUser)
	assert.Equal(t, "platform", KeyTypePlatform)
	assert.Equal(t, "platform-key", PlatformKeyName)
}
