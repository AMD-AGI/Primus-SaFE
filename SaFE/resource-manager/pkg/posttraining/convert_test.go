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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
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

func TestSweepReclaimsCachesOfDeletedModels(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	kept := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-kept"}}
	run := testRunRow("Running", "train")
	r, db, _ := newTestReconciler(t, run, ws, kept)
	loadChartRegistry(t, r)
	cacheDir := "/wekafs/custom/models/post-training-cache/"
	gone := &dbclient.PostTrainingConvertCache{ID: 1, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-gone-540ea91",
		ModelID: "m-gone", Path: cacheDir + "slime-qwen3-8B-m-gone-540ea91", Status: dbclient.ConvertCacheReady}
	live := &dbclient.PostTrainingConvertCache{ID: 2, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-kept-540ea91",
		ModelID: "m-kept", Path: cacheDir + "slime-qwen3-8B-m-kept-540ea91", Status: dbclient.ConvertCacheReady}
	retired := &dbclient.PostTrainingConvertCache{ID: 3, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-kept-0ld",
		ModelID: "m-kept", Path: cacheDir + "slime-qwen3-8B-m-kept-0ld", Status: dbclient.ConvertCacheReady}
	user := testRunRow("Running", "train")
	user.Recipe = `{"convert":{"cacheKey":"slime-qwen3-8B-m-kept-0ld"}}`
	db.EXPECT().ListPostTrainingConvertCaches(gomock.Any(), gomock.Any()).Return(
		[]*dbclient.PostTrainingConvertCache{gone, live, retired}, nil).Times(2)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, f dbclient.PostTrainingRunFilter) ([]*dbclient.PostTrainingRun, int64, error) {
			if f.BaseModelID == "m-kept" {
				return []*dbclient.PostTrainingRun{user}, 1, nil
			}
			return nil, 0, nil
		}).AnyTimes()
	db.EXPECT().UpdatePostTrainingConvertCache(gomock.Any(), int64(1),
		map[string]interface{}{"status": dbclient.ConvertCacheInvalid}).Return(nil)

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	require.Equal(t, cacheSweepRequeue, res.RequeueAfter, "the sweep comes back for the running cleanup")
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(ctx, jobs))
	require.Len(t, jobs.Items, 1, "only the cache of the deleted model goes; a retired commit in use stays")
	job := &jobs.Items[0]
	require.Equal(t, gone.Path, job.GetParameter(v1.ParameterDestPath).Value)
	require.Equal(t, "1", job.Labels[v1.PostTrainingConvertCacheLabel])

	gone.Status = dbclient.ConvertCacheInvalid
	job.Status.Phase = v1.OpsJobSucceeded
	require.NoError(t, r.Update(ctx, job))
	db.EXPECT().DeletePostTrainingConvertCache(gomock.Any(), int64(1)).Return(nil)
	res, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	require.Zero(t, res.RequeueAfter)
	require.NoError(t, r.List(ctx, jobs))
	require.Empty(t, jobs.Items, "the finished cleanup job is removed")
}

func TestCacheCommit(t *testing.T) {
	require.Equal(t, "540ea91", cacheCommit("slime-qwen3-4B-Instruct-2507-m-1-540ea91"))
}
