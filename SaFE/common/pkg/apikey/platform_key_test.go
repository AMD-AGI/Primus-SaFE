/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apikey

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestGetOrCreatePlatformKeyInsertConflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_client.NewMockInterface(ctrl)
	plainKey := "ak-conflict-test-key"
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

func TestIsUniqueViolation(t *testing.T) {
	assert.True(t, isUniqueViolation(&pq.Error{Code: "23505"}))
	assert.True(t, isUniqueViolation(fmt.Errorf("wrapped: %w", &pq.Error{Code: "23505"})))
	assert.False(t, isUniqueViolation(fmt.Errorf("other db error")))
}
