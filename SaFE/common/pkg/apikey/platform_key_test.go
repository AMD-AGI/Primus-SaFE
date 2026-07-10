/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apikey

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func testPlainToken(repeat byte) string {
	return tokenPrefix + strings.Repeat(string(repeat), 12)
}

func TestGetOrCreatePlatformKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)

	t.Run("nil db client returns error", func(t *testing.T) {
		_, err := GetOrCreatePlatformKey(context.Background(), nil, "user-1", "testuser")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database client not initialized")
	})

	t.Run("returns existing platform key", func(t *testing.T) {
		plainKey := testPlainToken('e')
		encryptedKey, err := encryptPlainToken(plainKey, nil)
		assert.NoError(t, err)

		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-existing").Return(&dbclient.ApiKey{
			Id:           10,
			UserId:       "user-existing",
			KeyType:      keyTypePlatform,
			EncryptedKey: &encryptedKey,
		}, nil)

		result, err := GetOrCreatePlatformKey(context.Background(), mockDB, "user-existing", "testuser")
		assert.NoError(t, err)
		assert.Equal(t, plainKey, result)
	})

	t.Run("creates new platform key when not found", func(t *testing.T) {
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-new").Return(nil, sql.ErrNoRows)
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, record *dbclient.ApiKey) error {
				assert.Equal(t, "user-new", record.UserId)
				assert.Equal(t, "newuser", record.UserName)
				assert.Equal(t, keyTypePlatform, record.KeyType)
				assert.Equal(t, platformKeyName, record.Name)
				assert.NotNil(t, record.EncryptedKey)
				assert.True(t, record.ExpirationTime.Time.Year() == 9999)
				record.Id = 99
				return nil
			},
		)

		result, err := GetOrCreatePlatformKey(context.Background(), mockDB, "user-new", "newuser")
		assert.NoError(t, err)
		assert.True(t, strings.HasPrefix(result, tokenPrefix))
	})

	t.Run("db query error returns error", func(t *testing.T) {
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-err").Return(nil, fmt.Errorf("db connection failed"))

		_, err := GetOrCreatePlatformKey(context.Background(), mockDB, "user-err", "erruser")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query platform key")
	})

	t.Run("existing key with nil encrypted_key returns error", func(t *testing.T) {
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-nil-enc").Return(&dbclient.ApiKey{
			Id:           11,
			UserId:       "user-nil-enc",
			KeyType:      keyTypePlatform,
			EncryptedKey: nil,
		}, nil)

		_, err := GetOrCreatePlatformKey(context.Background(), mockDB, "user-nil-enc", "testuser")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "platform key has no encrypted value")
	})

	t.Run("insert failure returns error", func(t *testing.T) {
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-insert-fail").Return(nil, sql.ErrNoRows)
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Return(fmt.Errorf("insert failed"))

		_, err := GetOrCreatePlatformKey(context.Background(), mockDB, "user-insert-fail", "failuser")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create platform key")
	})
}

func TestGetOrCreatePlatformKeyInsertConflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	plainKey := testPlainToken('c')
	encryptedKey, err := encryptPlainToken(plainKey, nil)
	assert.NoError(t, err)

	gomock.InOrder(
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-conflict").Return(nil, sql.ErrNoRows),
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to insert api key: %w", &pq.Error{Code: "23505"})),
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-conflict").Return(&dbclient.ApiKey{
			Id:           42,
			UserId:       "user-conflict",
			KeyType:      keyTypePlatform,
			EncryptedKey: &encryptedKey,
		}, nil),
	)

	result, err := GetOrCreatePlatformKey(context.Background(), mockDB, "user-conflict", "conflict-user")
	assert.NoError(t, err)
	assert.Equal(t, plainKey, result)
}

func TestGetOrCreatePlatformKeyInsertConflictRequeryFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	gomock.InOrder(
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-conflict-err").Return(nil, sql.ErrNoRows),
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to insert api key: %w", &pq.Error{Code: "23505"})),
		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-conflict-err").Return(nil, fmt.Errorf("requery failed")),
	)

	_, err := GetOrCreatePlatformKey(context.Background(), mockDB, "user-conflict-err", "conflict-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query platform key")
}

func TestPlatformKeyCryptoHelpers(t *testing.T) {
	plainKey := testPlainToken('h')
	secret := []byte("test-secret")

	hash := hashPlainToken(plainKey, secret)
	assert.NotEmpty(t, hash)
	assert.Equal(t, hash, hashPlainToken(plainKey, secret))

	hint := generateKeyHint(plainKey)
	assert.Contains(t, hint, tokenPrefix)
	assert.Contains(t, hint, "****")

	encrypted, err := encryptPlainToken(plainKey, secret)
	assert.NoError(t, err)
	decrypted, err := decryptPlainToken(encrypted, secret)
	assert.NoError(t, err)
	assert.Equal(t, plainKey, decrypted)

	generated, err := generatePlainToken()
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(generated, tokenPrefix))
}

func TestIsUniqueViolation(t *testing.T) {
	assert.True(t, isUniqueViolation(&pq.Error{Code: "23505"}))
	assert.True(t, isUniqueViolation(fmt.Errorf("wrapped: %w", &pq.Error{Code: "23505"})))
	assert.False(t, isUniqueViolation(fmt.Errorf("other db error")))
}
