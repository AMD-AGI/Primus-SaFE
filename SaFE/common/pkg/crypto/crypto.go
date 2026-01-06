/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package crypto

import (
	"fmt"
	"sync"

	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/crypto"
)

// Provides AES encryption/decryption functionality with singleton pattern
type Crypto struct {
	key string
}

// once - Ensures singleton instance creation
// instance - Singleton instance of Crypto
var (
	once     sync.Once
	instance *Crypto
)

// AESKeyLen - AES key length requirement (16 bytes for AES-128)
const (
	AESKeyLen = 16
)

// NewCrypto creates and returns a singleton instance of Crypto.
// It initializes the crypto key from configuration if crypto is enabled and validates key length requirements.
func NewCrypto() *Crypto {
	once.Do(func() {
		key := ""
		if commonconfig.IsCryptoEnable() {
			var err error
			key = commonconfig.GetCryptoKey()
			if key == "" {
				klog.ErrorS(err, "failed to get private key for crypto")
				return
			} else if len(key) != AESKeyLen {
				klog.ErrorS(err, fmt.Sprintf("invalid crypto key, the length must be %d", AESKeyLen))
				return
			}
		}
		instance = &Crypto{
			key: key,
		}
	})
	return instance
}

// Encrypt encrypts plaintext data using AES encryption.
// Returns the encrypted string or the original string if crypto is disabled.
// Returns an error if encryption fails or the key is missing.
func (c *Crypto) Encrypt(plainText []byte) (string, error) {
	if !commonconfig.IsCryptoEnable() {
		return string(plainText), nil
	}
	if c.key == "" {
		return "", fmt.Errorf("failed to get crypto key")
	}
	return crypto.Encrypt(plainText, []byte(c.key))
}

// Decrypt decrypts ciphertext data using AES decryption.
// Returns the decrypted string or the original string if crypto is disabled.
// Returns an error if decryption fails or the key is missing.
func (c *Crypto) Decrypt(ciphertext string) (string, error) {
	if !commonconfig.IsCryptoEnable() {
		return ciphertext, nil
	}
	if c.key == "" {
		return "", fmt.Errorf("failed to get crypto key")
	}
	data, err := crypto.Decrypt(ciphertext, []byte(c.key))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
