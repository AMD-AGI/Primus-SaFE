/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
			key = commonconfig.GetCryptoKey()
			if key == "" {
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
