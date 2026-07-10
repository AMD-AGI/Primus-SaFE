/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apikey

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lib/pq"
	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	apiKeyPrefix      = "ak-"
	apiKeyTokenLength = 32
	keyTypePlatform   = "platform"
	platformKeyName   = "platform-key"
)

// GetOrCreatePlatformKey returns the plaintext platform key for a user.
// If no platform key exists, it creates one automatically.
func GetOrCreatePlatformKey(ctx context.Context, db dbclient.Interface, userId, userName string) (string, error) {
	if db == nil {
		return "", commonerrors.NewInternalError("database client not initialized")
	}

	record, err := db.GetPlatformKeyByUserId(ctx, userId)
	if err != nil && err != sql.ErrNoRows {
		klog.ErrorS(err, "failed to query platform key", "userId", userId)
		return "", commonerrors.NewInternalError("failed to query platform key")
	}

	if record != nil {
		if record.EncryptedKey == nil || *record.EncryptedKey == "" {
			return "", commonerrors.NewInternalError("platform key has no encrypted value")
		}
		plaintext, err := decryptApiKey(*record.EncryptedKey, getApiKeySecret())
		if err != nil {
			klog.ErrorS(err, "failed to decrypt platform key", "userId", userId)
			return "", commonerrors.NewInternalError("failed to decrypt platform key")
		}
		return plaintext, nil
	}

	apiKey, err := generateApiKey()
	if err != nil {
		return "", commonerrors.NewInternalError("failed to generate platform key")
	}

	secret := getApiKeySecret()
	hashedKey := hashApiKey(apiKey, secret)
	keyHint := generateKeyHint(apiKey)
	encryptedKey, err := encryptApiKey(apiKey, secret)
	if err != nil {
		return "", commonerrors.NewInternalError("failed to encrypt platform key")
	}

	now := time.Now().UTC()
	farFuture := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

	newRecord := &dbclient.ApiKey{
		Name:           platformKeyName,
		UserId:         userId,
		UserName:       userName,
		ApiKey:         hashedKey,
		KeyHint:        keyHint,
		ExpirationTime: pq.NullTime{Time: farFuture, Valid: true},
		CreationTime:   pq.NullTime{Time: now, Valid: true},
		Whitelist:      "[]",
		Deleted:        false,
		KeyType:        keyTypePlatform,
		EncryptedKey:   &encryptedKey,
	}

	if err := db.InsertApiKey(ctx, newRecord); err != nil {
		klog.ErrorS(err, "failed to insert platform key", "userId", userId)
		return "", commonerrors.NewInternalError("failed to create platform key")
	}

	klog.Infof("created platform key for user %s, apiKeyId: %d", userId, newRecord.Id)
	return apiKey, nil
}

func getApiKeySecret() []byte {
	secret := commonconfig.GetCryptoKey()
	if secret == "" {
		return nil
	}
	return []byte(secret)
}

func generateApiKey() (string, error) {
	bytes := make([]byte, apiKeyTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(bytes)
	return apiKeyPrefix + encoded, nil
}

func hashApiKey(apiKey string, secret []byte) string {
	if len(secret) == 0 {
		hash := sha256.Sum256([]byte(apiKey))
		return hex.EncodeToString(hash[:])
	}
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(apiKey))
	return hex.EncodeToString(h.Sum(nil))
}

func generateKeyHint(apiKey string) string {
	keyBody := strings.TrimPrefix(apiKey, apiKeyPrefix)
	if len(keyBody) < 6 {
		return apiKeyPrefix + keyBody
	}
	return apiKeyPrefix + keyBody[:2] + "****" + keyBody[len(keyBody)-4:]
}

func encryptApiKey(plaintext string, secret []byte) (string, error) {
	key := deriveAESKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func decryptApiKey(encrypted string, secret []byte) (string, error) {
	key := deriveAESKey(secret)
	data, err := base64.RawURLEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := aesGCM.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	return string(plaintext), nil
}

func deriveAESKey(secret []byte) []byte {
	hash := sha256.Sum256(secret)
	return hash[:]
}
