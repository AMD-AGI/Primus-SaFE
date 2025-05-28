/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package crypto

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/crypto"
)

type Crypto struct {
	key string
}

var (
	once     sync.Once
	instance *Crypto
)

func NewCrypto() *Crypto {
	once.Do(func() {
		key := ""
		if commonconfig.IsCryptoEnable() {
			var err error
			key, err = getCryptoKey()
			if err != nil {
				klog.ErrorS(err, "failed to get crypto key")
				return
			}
		}
		instance = &Crypto{
			key: key,
		}
	})
	return instance
}

func (c *Crypto) Encrypt(plainText []byte) (string, error) {
	if !commonconfig.IsCryptoEnable() {
		return string(plainText), nil
	}
	if c.key == "" {
		return "", fmt.Errorf("failed to get crypto key")
	}
	return crypto.Encrypt(plainText, []byte(c.key))
}

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

// The crypto_file is created during deployment and acts as the global key for the entire system
func getCryptoKey() (string, error) {
	keyFile := commonconfig.GetCryptoKey()
	if keyFile == "" {
		return "", fmt.Errorf("global.crypto_key of config is not set")
	}
	f, err := os.Open(keyFile)
	if err != nil {
		return "", err
	}
	defer func() {
		if err = f.Close(); err != nil {
			klog.ErrorS(err, "failed to close file")
		}
	}()

	data := make([]byte, 1024)
	n, err := f.Read(data)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(data[:n]), "\n"), nil
}
