/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const nodeInformerRestartTimeout = 11 * time.Minute

var restartNodeInformer func(context.Context, *v1.Cluster) error

// RegisterNodeInformerRestarter registers a callback to (re)start the node informer after client rebuild.
func RegisterNodeInformerRestarter(fn func(context.Context, *v1.Cluster) error) {
	restartNodeInformer = fn
}

func tryRestartNodeInformer(_ context.Context, cluster *v1.Cluster) {
	if restartNodeInformer == nil || cluster == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), nodeInformerRestartTimeout)
		defer cancel()
		if err := restartNodeInformer(ctx, cluster); err != nil {
			klog.ErrorS(err, "failed to restart node informer", "cluster", cluster.Name)
		}
	}()
}
