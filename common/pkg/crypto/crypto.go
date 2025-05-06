/*
 * Copyright © AMD. 2025-2026. All rights reserved.
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

const (
	CryptoFileName = "crypto_file"
)

type Crypto struct {
	key string
}

var (
	once     sync.Once
	instance *Crypto
)

func Instance() *Crypto {
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

func getCryptoKey() (string, error) {
	value := os.Getenv("Crypto")
	if value != "" {
		return value, nil
	}
	keyFile := os.Getenv(CryptoFileName)
	if keyFile == "" {
		return "", fmt.Errorf("%s of environment is not set", CryptoFileName)
	}
	f, err := os.Open(keyFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data := make([]byte, 1024)
	n, err := f.Read(data)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(data[:n]), "\n"), nil
}
