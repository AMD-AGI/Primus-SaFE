/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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

// NewCrypto create or return singleton Crypto instance
// Initializes crypto key from configuration if crypto is enabled
// Validates key length requirements
// Returns: Singleton Crypto instance
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

// Encrypt plaintext data using AES encryption
// Parameters:
//
//	plainText: Byte array of data to encrypt
//
// Returns:
//
//	Encrypted string data or original string if crypto disabled
//	Error if encryption fails or key is missing
func (c *Crypto) Encrypt(plainText []byte) (string, error) {
	if !commonconfig.IsCryptoEnable() {
		return string(plainText), nil
	}
	if c.key == "" {
		return "", fmt.Errorf("failed to get crypto key")
	}
	return crypto.Encrypt(plainText, []byte(c.key))
}

// Decrypt ciphertext data using AES decryption
// Parameters:
//
//	ciphertext: Encrypted string data to decrypt
//
// Returns:
//
//	Decrypted string data or original string if crypto disabled
//	Error if decryption fails or key is missing
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
