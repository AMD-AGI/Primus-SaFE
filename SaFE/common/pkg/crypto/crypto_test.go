/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package crypto

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// resetSingleton resets the singleton instance for testing
func resetSingleton() {
	once = sync.Once{}
	instance = nil
}

func TestNewCrypto(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func(t *testing.T) func()
		expectInstance bool
		expectKey      bool
	}{
		{
			name: "crypto disabled",
			setupFunc: func(t *testing.T) func() {
				resetSingleton()
				commonconfig.SetValue("crypto.enable", "false")
				return func() {
					commonconfig.SetValue("crypto.enable", "")
				}
			},
			expectInstance: true,
			expectKey:      false,
		},
		{
			name: "crypto enabled with valid key",
			setupFunc: func(t *testing.T) func() {
				resetSingleton()
				// Create temp secret file with valid 16-byte key
				tmpDir := t.TempDir()
				secretPath := filepath.Join(tmpDir, "crypto")
				err := os.MkdirAll(secretPath, 0755)
				assert.NoError(t, err)
				err = os.WriteFile(filepath.Join(secretPath, "key"), []byte("1234567890123456"), 0644)
				assert.NoError(t, err)

				commonconfig.SetValue("crypto.enable", "true")
				commonconfig.SetValue("crypto.secret_path", secretPath)
				return func() {
					commonconfig.SetValue("crypto.enable", "")
					commonconfig.SetValue("crypto.secret_path", "")
				}
			},
			expectInstance: true,
			expectKey:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setupFunc(t)
			defer cleanup()

			crypto := NewCrypto()
			if tt.expectInstance {
				assert.NotNil(t, crypto)
			}
			if tt.expectKey {
				assert.NotEmpty(t, crypto.key)
				assert.Equal(t, AESKeyLen, len(crypto.key))
			}
		})
	}
}

func TestCrypto_EncryptDecrypt_Disabled(t *testing.T) {
	resetSingleton()
	commonconfig.SetValue("crypto.enable", "false")
	defer commonconfig.SetValue("crypto.enable", "")

	crypto := NewCrypto()
	assert.NotNil(t, crypto)

	plainText := "hello world"

	// When crypto is disabled, Encrypt should return original text
	encrypted, err := crypto.Encrypt([]byte(plainText))
	assert.NoError(t, err)
	assert.Equal(t, plainText, encrypted)

	// When crypto is disabled, Decrypt should return original text
	decrypted, err := crypto.Decrypt(encrypted)
	assert.NoError(t, err)
	assert.Equal(t, plainText, decrypted)
}

func TestCrypto_EncryptDecrypt_Enabled(t *testing.T) {
	resetSingleton()

	// Create temp secret file with valid 16-byte key
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "crypto")
	err := os.MkdirAll(secretPath, 0755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(secretPath, "key"), []byte("1234567890123456"), 0644)
	assert.NoError(t, err)

	commonconfig.SetValue("crypto.enable", "true")
	commonconfig.SetValue("crypto.secret_path", secretPath)
	defer func() {
		commonconfig.SetValue("crypto.enable", "")
		commonconfig.SetValue("crypto.secret_path", "")
	}()

	crypto := NewCrypto()
	assert.NotNil(t, crypto)

	testCases := []struct {
		name      string
		plainText string
	}{
		{"simple text", "hello world"},
		{"empty string", ""},
		{"special characters", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
		{"unicode text", "你好世界"},
		{"long text", "This is a longer text that spans multiple characters and tests the encryption capability of the system."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encrypted, err := crypto.Encrypt([]byte(tc.plainText))
			assert.NoError(t, err)

			// Encrypted text should be different from plain text (unless empty)
			if tc.plainText != "" {
				assert.NotEqual(t, tc.plainText, encrypted)
			}

			decrypted, err := crypto.Decrypt(encrypted)
			assert.NoError(t, err)
			assert.Equal(t, tc.plainText, decrypted)
		})
	}
}

func TestCrypto_Decrypt_InvalidCiphertext(t *testing.T) {
	resetSingleton()

	// Create temp secret file with valid 16-byte key
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "crypto")
	err := os.MkdirAll(secretPath, 0755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(secretPath, "key"), []byte("1234567890123456"), 0644)
	assert.NoError(t, err)

	commonconfig.SetValue("crypto.enable", "true")
	commonconfig.SetValue("crypto.secret_path", secretPath)
	defer func() {
		commonconfig.SetValue("crypto.enable", "")
		commonconfig.SetValue("crypto.secret_path", "")
	}()

	crypto := NewCrypto()
	assert.NotNil(t, crypto)

	// Should return error for invalid ciphertext
	_, err = crypto.Decrypt("invalid-base64!@#")
	assert.Error(t, err)

	// Should return error for valid base64 but invalid ciphertext
	_, err = crypto.Decrypt("aGVsbG8gd29ybGQ=") // "hello world" in base64
	assert.Error(t, err)
}

func TestCrypto_Singleton(t *testing.T) {
	resetSingleton()
	commonconfig.SetValue("crypto.enable", "false")
	defer commonconfig.SetValue("crypto.enable", "")

	crypto1 := NewCrypto()
	crypto2 := NewCrypto()

	// Should return the same instance
	assert.Same(t, crypto1, crypto2)
}

func TestAESKeyLen(t *testing.T) {
	// AES-128 requires 16-byte key
	assert.Equal(t, 16, AESKeyLen)
}
