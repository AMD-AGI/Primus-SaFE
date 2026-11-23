/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

const (
	cacheSweepInterval = time.Hour
	// cacheSweepRequest is the work queue key of a sweep of the conversion caches; run ids never take this form.
	cacheSweepRequest = "#convert-cache-sweep"
	cacheSweepRequeue = time.Minute
	// cacheReclaimRetry is how long a failed reclaim job is kept before the cache is reclaimed again.
	cacheReclaimRetry = 30 * time.Minute
	// convertCacheDir is the directory the compiler keeps shared conversions in, one <cache key> directory each.
	convertCacheDir = "post-training-cache"
	// leftoverWait is how long the workload of a deleted job may take to go before a job of its name is made
	// all the same.
	leftoverWait = 10 * time.Minute
)

// registerConversion records the shared conversion a run's launcher reused or made as ready, so other runs of
// the model find it and it is reclaimed with the model. The key and the directory come from the run's recipe,
// which the server wrote; a marker naming others is ignored.
func (r *RunReconciler) registerConversion(ctx context.Context, run *dbclient.PostTrainingRun, conversion *conversionDetail) {
	rec := parseRecipe(run).Convert
	if rec == nil || rec.CacheKey == "" || conversion.CacheKey != rec.CacheKey || conversion.Dir != path.Dir(rec.Dir) {
		klog.InfoS("ignoring a convert marker that does not match the recipe", "run", run.RunID,
			"cacheKey", conversion.CacheKey, "dir", conversion.Dir)
		return
	}
	cache := &dbclient.PostTrainingConvertCache{
		Workspace: run.Workspace,
		Framework: run.Framework,
		CacheKey:  rec.CacheKey,
		ModelID:   run.BaseModelID,
		Path:      path.Dir(rec.Dir),
		Status:    dbclient.ConvertCacheReady,
		FP8Ready:  conversion.FP8,
		SizeBytes: conversion.SizeBytes,
	}
	if cache.Framework == "" {
		cache.Framework = "slime"
	}
	if err := r.db.UpsertPostTrainingConvertCache(ctx, cache); err != nil {
		klog.ErrorS(err, "failed to register a conversion cache", "run", run.RunID, "cacheKey", conversion.CacheKey)
	}
}

// runCacheSweeps queues a sweep of the conversion caches every hour until ctx ends.
func (r *RunReconciler) runCacheSweeps(ctx context.Context) error {
	ticker := time.NewTicker(cacheSweepInterval)
	defer ticker.Stop()
	for {
		r.queueCacheSweep()
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// queueCacheSweep asks for a sweep of the conversion caches; it is dropped while the queue is full.
func (r *RunReconciler) queueCacheSweep() {
	obj := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: cacheSweepRequest}}
	select {
	case r.recovered <- event.GenericEvent{Object: obj}:
	default:
	}
}

// sweepCaches reclaims the conversion caches whose source model is gone, or whose framework commit no image of
// the registry uses any more, once no unfinished run uses them.
func (r *RunReconciler) sweepCaches(ctx context.Context) (ctrlruntime.Result, error) {
	caches, err := r.db.ListPostTrainingConvertCaches(ctx, dbclient.PostTrainingConvertCacheFilter{})
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	commits := registryCommits(r.registry.get(ctx, r.reader))
	pending := false
	for _, cache := range caches {
		if !r.cacheReclaimable(ctx, cache, commits) {
			continue
		}
		done, err := r.reclaimCache(ctx, cache)
		if err != nil {
			klog.ErrorS(err, "failed to reclaim a conversion cache", "cache", cache.CacheKey, "path", cache.Path)
		}
		pending = pending || !done
	}
	if pending {
		return ctrlruntime.Result{RequeueAfter: cacheSweepRequeue}, nil
	}
	return ctrlruntime.Result{}, nil
}

// registryCommits returns the framework commits the images of a registry name, nil without a registry. An image
// that names none, as an unverified one may, is left out: with only such images no cache counts as retired.
func registryCommits(reg *registry.Registry) map[string]bool {
	if reg == nil {
		return nil
	}
	commits := map[string]bool{}
	for _, img := range reg.Images {
		if img.FrameworkCommit != "" {
			commits[img.FrameworkCommit] = true
		}
	}
	return commits
}

// cacheReclaimable reports whether a cache can go: its model is deleted, its commit is retired or its
// reclaiming has begun, and no unfinished run uses it.
func (r *RunReconciler) cacheReclaimable(ctx context.Context, cache *dbclient.PostTrainingConvertCache, commits map[string]bool) bool {
	model := &v1.Model{}
	err := r.Get(ctx, client.ObjectKey{Name: cache.ModelID}, model)
	gone := apierrors.IsNotFound(err) || (err == nil && !model.GetDeletionTimestamp().IsZero())
	retired := len(commits) > 0 && commitRetired(cache.CacheKey, commits)
	if !gone && !retired && cache.Status != dbclient.ConvertCacheInvalid {
		return false
	}
	inUse, err := r.cacheInUse(ctx, cache)
	return err == nil && !inUse
}

// cacheInUse reports whether an unfinished run uses the cache's key, in any workspace, as the workspaces on one
// volume share the cache directory.
func (r *RunReconciler) cacheInUse(ctx context.Context, cache *dbclient.PostTrainingConvertCache) (bool, error) {
	runs, _, err := r.db.ListPostTrainingRuns(ctx, dbclient.PostTrainingRunFilter{BaseModelID: cache.ModelID,
		Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}})
	if err != nil {
		return false, err
	}
	for _, run := range runs {
		if rec := parseRecipe(run); rec.Convert != nil && rec.Convert.CacheKey == cache.CacheKey {
			return true, nil
		}
	}
	return false, nil
}

// commitRetired reports whether no framework commit of the registry ends a cache key; a commit may hold hyphens
// itself.
func commitRetired(key string, commits map[string]bool) bool {
	for commit := range commits {
		if commit != "" && strings.HasSuffix(key, "-"+commit) {
			return false
		}
	}
	return true
}

// reclaimCache removes a cache directory with a model-cleanup job: one sweep marks the record invalid, the next
// checks once more that no run took the cache meanwhile and starts the job, unless the workload of a deleted one
// holds it back, and the record is deleted once the job succeeded. A failed job, or one that is unfinished after
// its timeout, is kept for cacheReclaimRetry and then deleted, so a later sweep tries again. A record whose path is
// not a cache directory loses only the record. It reports whether the cache is gone.
func (r *RunReconciler) reclaimCache(ctx context.Context, cache *dbclient.PostTrainingConvertCache) (bool, error) {
	if path.Base(cache.Path) != cache.CacheKey || path.Base(path.Dir(cache.Path)) != convertCacheDir {
		klog.InfoS("dropping a conversion cache record whose path is not a cache directory", "cache", cache.CacheKey,
			"path", cache.Path)
		return true, r.db.DeletePostTrainingConvertCache(ctx, cache.ID)
	}
	job := &v1.OpsJob{}
	err := r.Get(ctx, client.ObjectKey{Name: resource.DirectoryCleanupJobName(cache.Workspace, cache.Path)}, job)
	if apierrors.IsNotFound(err) {
		ws := &v1.Workspace{}
		if err = r.Get(ctx, client.ObjectKey{Name: cache.Workspace}, ws); apierrors.IsNotFound(err) {
			return true, r.db.DeletePostTrainingConvertCache(ctx, cache.ID)
		} else if err != nil {
			return false, err
		}
		if job, err = resource.NewDirectoryCleanupOpsJob(ws, cache.Path, map[string]string{
			v1.PostTrainingConvertCacheLabel: strconv.FormatInt(cache.ID, 10)}); err != nil {
			return false, err
		}
		// A failed job has to outlive cacheReclaimRetry: once it is gone the next sweep makes it again.
		job.Spec.TTLSecondsAfterFinished = int(2 * cacheReclaimRetry / time.Second)
		if cache.Status != dbclient.ConvertCacheInvalid {
			if err = r.db.UpdatePostTrainingConvertCache(ctx, cache.ID,
				map[string]interface{}{"status": dbclient.ConvertCacheInvalid}); err != nil {
				return false, err
			}
			// A run submitted on the cache a moment before it was marked has its row by the next sweep, which
			// looks for the runs of the cache again before it removes anything.
			return false, nil
		}
		if inUse, err := r.cacheInUse(ctx, cache); err != nil || inUse {
			return false, err
		}
		// No job is made while the workload of a deleted one of its name holds it back.
		if left, err := r.jobWorkloadLeft(ctx, job.Name); err != nil || left {
			return false, err
		}
		return false, client.IgnoreAlreadyExists(r.Create(ctx, job))
	}
	if err != nil {
		return false, err
	}
	switch {
	case job.Status.Phase == v1.OpsJobSucceeded:
		if err = r.db.DeletePostTrainingConvertCache(ctx, cache.ID); err != nil {
			return false, err
		}
		return true, client.IgnoreNotFound(r.Delete(ctx, job))
	case job.Status.Phase == v1.OpsJobFailed, job.IsTimeout():
		if time.Since(jobEnd(job)) < cacheReclaimRetry {
			return false, nil
		}
		klog.Warningf("reclaiming conversion cache %s at %s failed: %s; trying again", cache.CacheKey, cache.Path,
			jobFailure(job))
		return false, client.IgnoreNotFound(r.Delete(ctx, job))
	}
	return false, nil
}

// jobEnd returns when an OpsJob finished; for one with no finish time, when its timeout ran out, or when it was
// created if it has not.
func jobEnd(job *v1.OpsJob) time.Time {
	if job.Status.FinishedAt != nil {
		return job.Status.FinishedAt.Time
	}
	if job.IsTimeout() {
		return job.CreationTimestamp.Add(time.Duration(job.Spec.TimeoutSecond) * time.Second)
	}
	return job.CreationTimestamp.Time
}

// jobFailure returns the reason a cleanup job failed: the message of its failed completion condition, or that it
// ran out of time for one that counts as failed because it was left unfinished past its timeout.
func jobFailure(job *v1.OpsJob) string {
	for _, c := range job.Status.Conditions {
		if c.Reason == "JobFailed" && c.Message != "" {
			return c.Message
		}
	}
	if job.Status.Phase != v1.OpsJobFailed {
		return "the cleanup job did not finish within its timeout"
	}
	return "the cleanup job failed"
}

// jobWorkloadLeft reports whether a workload of an OpsJob's name still holds a job of that name back. A job that
// was deleted leaves its workload, of the same name, until that is torn down, and a job of the name made meanwhile
// would take that workload for its own and never get one: it is made once the workload is gone. A workload that
// has been deleted for longer than leftoverWait is stuck and holds nothing back: the job is made and ends on its
// timeout.
func (r *RunReconciler) jobWorkloadLeft(ctx context.Context, name string) (bool, error) {
	w := &v1.Workload{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, w); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return w.GetDeletionTimestamp().IsZero() || time.Since(w.GetDeletionTimestamp().Time) < leftoverWait, nil
}
