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
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

const (
	cacheSweepInterval = time.Hour
	// cacheSweepRequest is the work queue key of a sweep of the conversion caches; run ids never take this form.
	cacheSweepRequest = "#convert-cache-sweep"
	cacheSweepRequeue = time.Minute
	// convertCacheDir is the directory the compiler keeps shared conversions in, one <cache key> directory each.
	convertCacheDir = "post-training-cache"
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
	var commits map[string]bool
	if reg := r.registry.get(ctx, r.reader); reg != nil {
		commits = map[string]bool{}
		for _, img := range reg.Images {
			commits[img.FrameworkCommit] = true
		}
	}
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

// cacheReclaimable reports whether a cache can go: its model is deleted, its commit is retired or its
// reclaiming has begun, and no unfinished run of its workspace uses it.
func (r *RunReconciler) cacheReclaimable(ctx context.Context, cache *dbclient.PostTrainingConvertCache, commits map[string]bool) bool {
	model := &v1.Model{}
	err := r.Get(ctx, client.ObjectKey{Name: cache.ModelID}, model)
	gone := apierrors.IsNotFound(err) || (err == nil && !model.GetDeletionTimestamp().IsZero())
	retired := commits != nil && !commits[cacheCommit(cache.CacheKey)]
	if !gone && !retired && cache.Status != dbclient.ConvertCacheInvalid {
		return false
	}
	runs, _, err := r.db.ListPostTrainingRuns(ctx, dbclient.PostTrainingRunFilter{BaseModelID: cache.ModelID,
		Workspace: cache.Workspace, Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}})
	if err != nil {
		return false
	}
	for _, run := range runs {
		if rec := parseRecipe(run); rec.Convert != nil && rec.Convert.CacheKey == cache.CacheKey {
			return false
		}
	}
	return true
}

// cacheCommit returns the framework commit that ends a cache key.
func cacheCommit(key string) string {
	return key[strings.LastIndex(key, "-")+1:]
}

// reclaimCache removes a cache directory with a model-cleanup job, marking the record invalid meanwhile, and
// deletes the record once the job succeeded. A failed job is deleted, so the next sweep tries again. A record
// whose path is not a cache directory loses only the record. It reports whether the cache is gone.
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
		if cache.Status != dbclient.ConvertCacheInvalid {
			if err = r.db.UpdatePostTrainingConvertCache(ctx, cache.ID,
				map[string]interface{}{"status": dbclient.ConvertCacheInvalid}); err != nil {
				return false, err
			}
		}
		return false, client.IgnoreAlreadyExists(r.Create(ctx, job))
	}
	if err != nil {
		return false, err
	}
	switch job.Status.Phase {
	case v1.OpsJobSucceeded:
		if err = r.db.DeletePostTrainingConvertCache(ctx, cache.ID); err != nil {
			return false, err
		}
		return true, client.IgnoreNotFound(r.Delete(ctx, job))
	case v1.OpsJobFailed:
		return false, client.IgnoreNotFound(r.Delete(ctx, job))
	}
	return false, nil
}
