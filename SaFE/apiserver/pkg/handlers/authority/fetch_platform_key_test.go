/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestFetchPlatformKeyModes(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl)

	prevEnabled, prevClient := platformDBEnabled, platformDBClient
	platformDBEnabled = func() bool { return true }
	platformDBClient = func() dbclient.Interface { return mockDB }
	t.Cleanup(func() {
		platformDBEnabled, platformDBClient = prevEnabled, prevClient
	})

	t.Run("external does not create and returns empty", func(t *testing.T) {
		prev := viper.Get("auth.mode")
		viper.Set("auth.mode", "external")
		t.Cleanup(func() { viper.Set("auth.mode", prev) })

		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-new").Return(nil, sql.ErrNoRows)
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Times(0)

		got, err := fetchPlatformKey(context.Background(), "user-new", "alice")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("builtin creates a missing key", func(t *testing.T) {
		prev := viper.Get("auth.mode")
		viper.Set("auth.mode", "builtin")
		t.Cleanup(func() { viper.Set("auth.mode", prev) })

		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-new").Return(nil, sql.ErrNoRows)
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Return(nil)

		got, err := fetchPlatformKey(context.Background(), "user-new", "alice")
		require.NoError(t, err)
		assert.NotEmpty(t, got)
		assert.Contains(t, got, "ak-")
	})
}
