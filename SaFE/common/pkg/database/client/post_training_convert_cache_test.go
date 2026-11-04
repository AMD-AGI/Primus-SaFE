/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPostTrainingConvertCacheCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 20)
	ctx := context.Background()

	_, _ = c.GetPostTrainingConvertCache(ctx, "ws", "slime-qwen3-8B-m1-540ea91")
	_, _ = c.ListPostTrainingConvertCaches(ctx, PostTrainingConvertCacheFilter{Workspace: "ws", ModelID: "m1",
		Status: ConvertCacheReady})
	_ = c.DeletePostTrainingConvertCache(ctx, 1)
}

func TestUpsertPostTrainingConvertCacheOnKey(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`INSERT INTO "post_training_convert_cache" .* ON CONFLICT \("workspace","cache_key"\) DO UPDATE SET ` +
		`"framework"="excluded"."framework","model_id"="excluded"."model_id","path"="excluded"."path",` +
		`"status"="excluded"."status"`).WillReturnRows(idRows(1))
	err := c.UpsertPostTrainingConvertCache(context.Background(), &PostTrainingConvertCache{Workspace: "ws",
		CacheKey: "k", ModelID: "m1", Path: "/wekafs/custom/models/post-training-cache/k", Status: ConvertCacheReady})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdatePostTrainingConvertCacheNotFound(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec(`UPDATE "post_training_convert_cache" SET`).WillReturnResult(sqlmock.NewResult(0, 0))
	err := c.UpdatePostTrainingConvertCache(context.Background(), 7, map[string]interface{}{"status": ConvertCacheInvalid})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, c.UpdatePostTrainingConvertCache(context.Background(), 7, nil))
}

func TestPostTrainingConvertCacheInputs(t *testing.T) {
	c := &Client{}
	require.Error(t, c.UpsertPostTrainingConvertCache(context.Background(), nil))
	_, err := c.GetPostTrainingConvertCache(context.Background(), "ws", "k")
	require.Error(t, err, "expected error when gorm not initialized")
}
