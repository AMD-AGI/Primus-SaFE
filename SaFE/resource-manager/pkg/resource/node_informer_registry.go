/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"sync/atomic"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const nodeInformerRestartTimeout = 11 * time.Minute

// The node informer lives in NodeK8sReconciler while the client factory it binds to is rebuilt by
// ClusterReconciler. These callbacks bridge the two without a direct dependency. They are stored
// atomically so registration is safe even if it ever moves off the startup path.
var (
	restartNodeInformer           atomic.Pointer[func(context.Context, *v1.Cluster) error]
	clearNodeInformerRegistration atomic.Pointer[func(string)]
)

// RegisterNodeInformerRestarter registers a callback to (re)start the node informer after client rebuild.
func RegisterNodeInformerRestarter(fn func(context.Context, *v1.Cluster) error) {
	if fn == nil {
		restartNodeInformer.Store(nil)
		return
	}
	restartNodeInformer.Store(&fn)
}

// RegisterNodeInformerClearer registers a callback to drop cached node informer registrations.
func RegisterNodeInformerClearer(fn func(string)) {
	if fn == nil {
		clearNodeInformerRegistration.Store(nil)
		return
	}
	clearNodeInformerRegistration.Store(&fn)
}

func clearNodeInformerForCluster(clusterName string) {
	if clear := clearNodeInformerRegistration.Load(); clear != nil {
		(*clear)(clusterName)
	}
}

// tryRestartNodeInformer synchronously re-attaches the node informer to the rebuilt client factory.
func tryRestartNodeInformer(ctx context.Context, cluster *v1.Cluster) error {
	restart := restartNodeInformer.Load()
	if restart == nil || cluster == nil {
		return nil
	}
	restartCtx, cancel := context.WithTimeout(ctx, nodeInformerRestartTimeout)
	defer cancel()
	return (*restart)(restartCtx, cluster)
}
