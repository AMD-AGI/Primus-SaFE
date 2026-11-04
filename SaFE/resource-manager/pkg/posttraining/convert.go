/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"

	"k8s.io/klog/v2"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// registerConversion records the shared conversion a run's launcher reused or made as ready, so other runs of
// the model find it and it is reclaimed with the model.
func (r *RunReconciler) registerConversion(ctx context.Context, run *dbclient.PostTrainingRun, conversion *conversionDetail) {
	cache := &dbclient.PostTrainingConvertCache{
		Workspace: run.Workspace,
		Framework: run.Framework,
		CacheKey:  conversion.CacheKey,
		ModelID:   run.BaseModelID,
		Path:      conversion.Dir,
		Status:    dbclient.ConvertCacheReady,
		SizeBytes: conversion.SizeBytes,
	}
	if cache.Framework == "" {
		cache.Framework = "slime"
	}
	if err := r.db.UpsertPostTrainingConvertCache(ctx, cache); err != nil {
		klog.ErrorS(err, "failed to register a conversion cache", "run", run.RunID, "cacheKey", conversion.CacheKey)
	}
}
