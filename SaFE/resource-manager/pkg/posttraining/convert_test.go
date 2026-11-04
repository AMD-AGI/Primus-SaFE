/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestConvertMarkerRegistersTheCache(t *testing.T) {
	in := newTestIngestion(100)
	feed(in, stageLine("a1", "convert", "running", ""),
		stageLine("a1", "convert", "skipped", `{"cacheKey":"slime-qwen3-8B-m1-abc",`+
			`"dir":"/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m1-abc","sizeBytes":42}`))
	require.Equal(t, &conversionDetail{CacheKey: "slime-qwen3-8B-m1-abc",
		Dir: "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m1-abc", SizeBytes: 42}, in.conversion)
	require.Equal(t, "convert", in.u.stage)

	plain := newTestIngestion(100)
	feed(plain, stageLine("a1", "convert", "done", ""))
	require.Nil(t, plain.conversion, "a custom image's conversion has no cache key")

	run := testRunRow("Running", "convert")
	run.Framework = "slime"
	r, db, _ := newTestReconciler(t, run)
	db.EXPECT().UpsertPostTrainingConvertCache(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, cache *dbclient.PostTrainingConvertCache) error {
			require.Equal(t, dbclient.PostTrainingConvertCache{Workspace: "ws1", Framework: "slime",
				CacheKey: "slime-qwen3-8B-m1-abc", ModelID: "qwen3-8b", Status: dbclient.ConvertCacheReady,
				Path: "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m1-abc", SizeBytes: 42}, *cache)
			return nil
		})
	r.registerConversion(context.Background(), run, in.conversion)
}
