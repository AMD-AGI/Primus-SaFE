/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

// A conversion marker records the shared cache, which a custom image's conversion does not have.
func TestConvertMarkerRegistersTheCache(t *testing.T) {
	in := newTestIngestion(100)
	feed(in, stageLine("a1", "convert", "running", ""),
		stageLine("a1", "convert", "skipped", `{"cacheKey":"slime-qwen3-8B-m1-abc",`+
			`"dir":"/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m1-abc","fp8":true,"sizeBytes":42}`))
	require.Equal(t, &conversionDetail{CacheKey: "slime-qwen3-8B-m1-abc",
		Dir: "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m1-abc", SizeBytes: 42, FP8: true}, in.conversion)
	require.Equal(t, "convert", in.u.stage)

	plain := newTestIngestion(100)
	feed(plain, stageLine("a1", "convert", "done", ""))
	require.Nil(t, plain.conversion, "a custom image's conversion has no cache key")

	run := testRunRow("Running", "convert")
	run.Framework = "slime"
	run.Recipe = `{"convert":{"cacheKey":"slime-qwen3-8B-m1-abc",` +
		`"dir":"/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m1-abc/torch_dist"}}`
	r, db, _ := newTestReconciler(t, run)
	db.EXPECT().UpsertPostTrainingConvertCache(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, cache *dbclient.PostTrainingConvertCache) error {
			require.Equal(t, dbclient.PostTrainingConvertCache{Workspace: "ws1", Framework: "slime",
				CacheKey: "slime-qwen3-8B-m1-abc", ModelID: "qwen3-8b", Status: dbclient.ConvertCacheReady,
				Path: "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m1-abc", SizeBytes: 42, FP8Ready: true},
				*cache, "the FP8 weights the launcher found are on record for the next FP8 rollout")
			return nil
		})
	r.registerConversion(context.Background(), run, in.conversion)

	for _, forged := range []*conversionDetail{
		{CacheKey: "slime-qwen3-8B-m1-abc", Dir: "/wekafs/custom/models/pt-other-run"},
		{CacheKey: "x", Dir: "/wekafs/custom/models/post-training-cache/x"},
	} {
		r.registerConversion(context.Background(), run, forged)
	}
	run.Recipe = `{"output":{"base":"/wekafs/custom/models/pt-1"}}`
	r.registerConversion(context.Background(), run, in.conversion)
}

// The sweep reclaims only the cache of a deleted model and removes the finished cleanup job.
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
		[]*dbclient.PostTrainingConvertCache{gone, live, retired}, nil).Times(3)
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
	require.Equal(t, cacheSweepRequeue, res.RequeueAfter, "the sweep comes back for the cache it marked")
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(ctx, jobs))
	require.Empty(t, jobs.Items, "a marked cache waits one sweep, for a run submitted on it a moment before")

	gone.Status = dbclient.ConvertCacheInvalid
	res, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	require.Equal(t, cacheSweepRequeue, res.RequeueAfter, "the sweep comes back for the running cleanup")
	require.NoError(t, r.List(ctx, jobs))
	require.Len(t, jobs.Items, 1, "only the cache of the deleted model goes; a retired commit in use stays")
	job := &jobs.Items[0]
	require.Equal(t, gone.Path, job.GetParameter(v1.ParameterDestPath).Value)
	require.Equal(t, "1", job.Labels[v1.PostTrainingConvertCacheLabel])
	require.Greater(t, time.Duration(job.Spec.TTLSecondsAfterFinished)*time.Second, cacheReclaimRetry,
		"a failed job outlives the wait before the next try, or it would be made again as soon as it is gone")

	job.Status.Phase = v1.OpsJobSucceeded
	require.NoError(t, r.Update(ctx, job))
	db.EXPECT().DeletePostTrainingConvertCache(gomock.Any(), int64(1)).Return(nil)
	res, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	require.Zero(t, res.RequeueAfter)
	require.NoError(t, r.List(ctx, jobs))
	require.Empty(t, jobs.Items, "the finished cleanup job is removed")
}

// A cache is reclaimed only if no run uses it after it was marked invalid.
func TestReclaimChecksTheRunsAgainOnceTheCacheIsMarked(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	r, db, _ := newTestReconciler(t, testRunRow("Running", "train"), ws)
	loadChartRegistry(t, r)
	gone := &dbclient.PostTrainingConvertCache{ID: 1, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-gone-540ea91",
		ModelID: "m-gone", Path: "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m-gone-540ea91",
		Status: dbclient.ConvertCacheReady}
	late := testRunRow("Pending", "")
	late.Recipe = `{"convert":{"cacheKey":"slime-qwen3-8B-m-gone-540ea91"}}`
	db.EXPECT().ListPostTrainingConvertCaches(gomock.Any(), gomock.Any()).Return(
		[]*dbclient.PostTrainingConvertCache{gone}, nil).Times(2)
	listed := 0
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, dbclient.PostTrainingRunFilter) ([]*dbclient.PostTrainingRun, int64, error) {
			if listed++; listed == 1 {
				return nil, 0, nil
			}
			return []*dbclient.PostTrainingRun{late}, 1, nil
		}).AnyTimes()
	db.EXPECT().UpdatePostTrainingConvertCache(gomock.Any(), int64(1),
		map[string]interface{}{"status": dbclient.ConvertCacheInvalid}).Return(nil)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	gone.Status = dbclient.ConvertCacheInvalid
	_, err = r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(context.Background(), jobs))
	require.Empty(t, jobs.Items, "a run submitted before the cache was marked keeps it")
}

// A failed reclaim job is kept for a while before the reclaim is tried again.
func TestAFailedReclaimWaitsBeforeItRunsAgain(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	dir := "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m-gone-540ea91"
	ended := metav1.NewTime(time.Now().Add(-time.Minute))
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: resource.DirectoryCleanupJobName("ws1", dir)}}
	job.Status.Phase, job.Status.FinishedAt = v1.OpsJobFailed, &ended
	r, db, _ := newTestReconciler(t, testRunRow("Running", "train"), ws, job)
	loadChartRegistry(t, r)
	gone := &dbclient.PostTrainingConvertCache{ID: 1, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-gone-540ea91",
		ModelID: "m-gone", Path: dir, Status: dbclient.ConvertCacheInvalid}
	db.EXPECT().ListPostTrainingConvertCaches(gomock.Any(), gomock.Any()).Return(
		[]*dbclient.PostTrainingConvertCache{gone}, nil).Times(2)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(job), &v1.OpsJob{}),
		"the failed job stays, so the next sweep in a minute does not start the cleanup again")

	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(job), job))
	long := metav1.NewTime(time.Now().Add(-time.Hour))
	job.Status.FinishedAt = &long
	require.NoError(t, r.Update(ctx, job))
	_, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	require.Error(t, r.Get(ctx, client.ObjectKeyFromObject(job), &v1.OpsJob{}), "after the wait it is tried again")
}

// A reclaim job that is unfinished after its timeout has failed as well: it is kept for the same wait from the
// timeout on and then removed for the next try.
func TestAReclaimJobLeftUnfinishedPastItsTimeoutIsTriedAgain(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	dir := "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m-gone-540ea91"
	name := client.ObjectKey{Name: resource.DirectoryCleanupJobName("ws1", dir)}
	for label, c := range map[string]struct {
		age  time.Duration
		gone bool
	}{
		"within its timeout":            {time.Hour - time.Minute, false},
		"within the wait after it":      {time.Hour + cacheReclaimRetry - time.Minute, false},
		"past the timeout and the wait": {time.Hour + cacheReclaimRetry + time.Minute, true},
	} {
		job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: name.Name,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-c.age))}, Spec: v1.OpsJobSpec{TimeoutSecond: 3600}}
		r, db, _ := newTestReconciler(t, testRunRow("Running", "train"), ws, job)
		loadChartRegistry(t, r)
		db.EXPECT().ListPostTrainingConvertCaches(gomock.Any(), gomock.Any()).Return(
			[]*dbclient.PostTrainingConvertCache{{ID: 1, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-gone-540ea91",
				ModelID: "m-gone", Path: dir, Status: dbclient.ConvertCacheInvalid}}, nil)
		db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
		ctx := context.Background()
		_, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
		require.NoError(t, err, label)
		require.Equal(t, c.gone, r.Get(ctx, name, &v1.OpsJob{}) != nil, label)
	}
}

// A reclaim job that was deleted leaves its workload, of the same name, for a while: the job is not made again
// while that workload holds it back.
func TestNoReclaimJobIsMadeWhileTheWorkloadOfADeletedOneIsLeft(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	dir := "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m-gone-540ea91"
	name := client.ObjectKey{Name: resource.DirectoryCleanupJobName("ws1", dir)}
	left := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name.Name, Finalizers: []string{v1.WorkloadFinalizer},
		DeletionTimestamp: &metav1.Time{Time: time.Now()}}}
	r, db, _ := newTestReconciler(t, testRunRow("Running", "train"), ws, left)
	loadChartRegistry(t, r)
	gone := &dbclient.PostTrainingConvertCache{ID: 1, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-gone-540ea91",
		ModelID: "m-gone", Path: dir, Status: dbclient.ConvertCacheInvalid}
	db.EXPECT().ListPostTrainingConvertCaches(gomock.Any(), gomock.Any()).Return(
		[]*dbclient.PostTrainingConvertCache{gone}, nil).Times(2)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	ctx := context.Background()
	sweep := ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}}
	res, err := r.Reconcile(ctx, sweep)
	require.NoError(t, err)
	require.Equal(t, cacheSweepRequeue, res.RequeueAfter, "the sweep comes back for the cache")
	require.Error(t, r.Get(ctx, name, &v1.OpsJob{}), "the workload of the deleted job is still there")
	left.Finalizers = nil
	require.NoError(t, r.Update(ctx, left))
	_, err = r.Reconcile(ctx, sweep)
	require.NoError(t, err)
	require.NoError(t, r.Get(ctx, name, &v1.OpsJob{}), "the job is made once the workload is gone")
}

// The cache of a listed commit stays, whatever characters the commit has.
func TestACommitWithAHyphenIsNotRetired(t *testing.T) {
	kept := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-kept"}}
	r, db, _ := newTestReconciler(t, testRunRow("Running", "train"), kept)
	loadChartRegistry(t, r)
	reg, err := r.registry.store.Get()
	require.NoError(t, err)
	reg.Images[0].FrameworkCommit = "540ea91-rc1"
	cache := &dbclient.PostTrainingConvertCache{ID: 4, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-kept-540ea91-rc1",
		ModelID: "m-kept", Path: "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m-kept-540ea91-rc1",
		Status: dbclient.ConvertCacheReady}
	db.EXPECT().ListPostTrainingConvertCaches(gomock.Any(), gomock.Any()).Return(
		[]*dbclient.PostTrainingConvertCache{cache}, nil)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	_, err = r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(context.Background(), jobs))
	require.Empty(t, jobs.Items, "the cache of a listed commit stays, whatever the commit looks like")
}

// A cache another workspace's run reads, or that a registry without images cannot retire, stays.
func TestACacheSharedByAnotherWorkspaceOrARegistryWithoutImagesStays(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	kept := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m-kept"}}
	r, db, _ := newTestReconciler(t, testRunRow("Running", "train"), ws, kept)
	loadChartRegistry(t, r)
	dir := "/wekafs/custom/models/post-training-cache/"
	retired := &dbclient.PostTrainingConvertCache{ID: 5, Workspace: "ws1", CacheKey: "slime-qwen3-8B-m-kept-0ld",
		ModelID: "m-kept", Path: dir + "slime-qwen3-8B-m-kept-0ld", Status: dbclient.ConvertCacheReady}
	other := testRunRow("Running", "train")
	other.Workspace, other.Recipe = "ws2", `{"convert":{"cacheKey":"slime-qwen3-8B-m-kept-0ld"}}`
	db.EXPECT().ListPostTrainingConvertCaches(gomock.Any(), gomock.Any()).Return(
		[]*dbclient.PostTrainingConvertCache{retired}, nil).Times(2)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, f dbclient.PostTrainingRunFilter) ([]*dbclient.PostTrainingRun, int64, error) {
			if f.Workspace == "" || f.Workspace == "ws2" {
				return []*dbclient.PostTrainingRun{other}, 1, nil
			}
			return nil, 0, nil
		}).Times(1)
	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(ctx, jobs))
	require.Empty(t, jobs.Items, "a run of another workspace on the same volume reads the same directory")

	reg, err := r.registry.store.Get()
	require.NoError(t, err)
	reg.Images = nil
	_, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	require.NoError(t, r.List(ctx, jobs))
	require.Empty(t, jobs.Items, "a registry that lists no image retires no commit")
}

// A cache record whose path is outside the cache directory loses the record, never the directory.
func TestSweepDropsRecordsOutsideTheCacheDirectory(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	r, db, _ := newTestReconciler(t, testRunRow("Running", "train"), ws)
	loadChartRegistry(t, r)
	forged := &dbclient.PostTrainingConvertCache{ID: 7, Workspace: "ws1", CacheKey: "x", ModelID: "m-gone",
		Path: "/wekafs/custom/models/pt-other-run", Status: dbclient.ConvertCacheReady}
	db.EXPECT().ListPostTrainingConvertCaches(gomock.Any(), gomock.Any()).Return(
		[]*dbclient.PostTrainingConvertCache{forged}, nil)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil)
	db.EXPECT().DeletePostTrainingConvertCache(gomock.Any(), int64(7)).Return(nil)

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
	require.NoError(t, err)
	require.Zero(t, res.RequeueAfter)
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(ctx, jobs))
	require.Empty(t, jobs.Items, "a record whose path is no cache directory loses the record, never the directory")
}

// The commits a registry builds are those its images name; an image without one names none.
func TestCommitsOfARegistryLeaveOutImagesWithoutOne(t *testing.T) {
	require.Nil(t, registryCommits(nil), "without a registry nothing is known to be retired")
	reg := &registry.Registry{Images: []registry.Image{{Image: "unverified:1"}}}
	require.Empty(t, registryCommits(reg), "an image that names no commit keeps no cache and retires none")
	reg.Images = append(reg.Images, registry.Image{Image: "img:1", FrameworkCommit: "abc1234"})
	require.Equal(t, map[string]bool{"abc1234": true}, registryCommits(reg))
}

// A cache key is retired only when it ends in none of the listed commits.
func TestCommitRetired(t *testing.T) {
	commits := map[string]bool{"540ea91": true, "540ea91-rc1": true}
	require.False(t, commitRetired("slime-qwen3-4B-Instruct-2507-m-1-540ea91", commits))
	require.False(t, commitRetired("slime-qwen3-4B-m-1-540ea91-rc1", commits))
	require.True(t, commitRetired("slime-qwen3-4B-m-1-0ld", commits))
}
