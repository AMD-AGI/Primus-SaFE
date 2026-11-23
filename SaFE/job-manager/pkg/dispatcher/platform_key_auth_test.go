/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"gotest.tools/assert"

	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestLookupPlatformKeyRespectsAuthMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl)

	t.Run("external does not create", func(t *testing.T) {
		prev := viper.Get("auth.mode")
		viper.Set("auth.mode", "external")
		t.Cleanup(func() { viper.Set("auth.mode", prev) })

		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-1").Return(nil, sql.ErrNoRows)
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Times(0)

		got, err := lookupPlatformKey(context.Background(), mockDB, "user-1", "alice")
		assert.NilError(t, err)
		assert.Equal(t, "", got)
	})

	t.Run("builtin creates a missing key", func(t *testing.T) {
		prev := viper.Get("auth.mode")
		viper.Set("auth.mode", "builtin")
		t.Cleanup(func() { viper.Set("auth.mode", prev) })

		mockDB.EXPECT().GetPlatformKeyByUserId(gomock.Any(), "user-1").Return(nil, sql.ErrNoRows)
		mockDB.EXPECT().InsertApiKey(gomock.Any(), gomock.Any()).Return(nil)

		got, err := lookupPlatformKey(context.Background(), mockDB, "user-1", "alice")
		assert.NilError(t, err)
		assert.Assert(t, got != "")
	})
}
