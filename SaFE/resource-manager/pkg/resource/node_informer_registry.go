/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const nodeInformerRestartTimeout = 11 * time.Minute

var (
	restartNodeInformer           func(context.Context, *v1.Cluster) error
	clearNodeInformerRegistration func(string)
)

// RegisterNodeInformerRestarter registers a callback to (re)start the node informer after client rebuild.
func RegisterNodeInformerRestarter(fn func(context.Context, *v1.Cluster) error) {
	restartNodeInformer = fn
}

// RegisterNodeInformerClearer registers a callback to drop cached node informer registrations.
func RegisterNodeInformerClearer(fn func(string)) {
	clearNodeInformerRegistration = fn
}

func clearNodeInformerForCluster(clusterName string) {
	if clearNodeInformerRegistration != nil {
		clearNodeInformerRegistration(clusterName)
	}
}

// tryRestartNodeInformer synchronously re-attaches the node informer to the rebuilt client factory.
func tryRestartNodeInformer(ctx context.Context, cluster *v1.Cluster) error {
	if restartNodeInformer == nil || cluster == nil {
		return nil
	}
	restartCtx, cancel := context.WithTimeout(ctx, nodeInformerRestartTimeout)
	defer cancel()
	return restartNodeInformer(restartCtx, cluster)
}
