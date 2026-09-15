/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// PodsOf returns a workload's per-pod detail, preferring the DB workload_pod
// table and falling back to the etcd Status.Pods. A nil db, a query error, or an
// empty table all fall back to Status.Pods so reads never regress.
func PodsOf(ctx context.Context, db dbclient.Interface, w *v1.Workload) []v1.WorkloadPod {
	if w == nil {
		return nil
	}
	if db != nil {
		if rows, err := db.ListWorkloadPods(ctx, w.Name); err == nil && len(rows) > 0 {
			return dbclient.WorkloadPodsToV1(rows)
		}
	}
	return w.Status.Pods
}

// DispatchNodesOf returns the per-dispatch node history of a workload, preferring
// the DB workload_dispatch_node table and falling back to the etcd Status.Nodes.
func DispatchNodesOf(ctx context.Context, db dbclient.Interface, w *v1.Workload) [][]string {
	if w == nil {
		return nil
	}
	if db != nil {
		if rows, err := db.ListWorkloadDispatchNodes(ctx, w.Name); err == nil && len(rows) > 0 {
			return dbclient.DispatchNodesToV1(rows)
		}
	}
	return w.Status.Nodes
}
