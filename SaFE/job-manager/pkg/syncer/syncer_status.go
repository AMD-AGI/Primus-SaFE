/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// hydrateWorkloadStatusFromDB loads per-pod and dispatch history from DB into
// the in-memory workload when status offload is enabled. DB rows override etcd
// only when present so workloads not yet migrated keep their etcd detail.
func (r *SyncerReconciler) hydrateWorkloadStatusFromDB(ctx context.Context, workloadId string, w *v1.Workload) {
	if !commonconfig.IsDBEnable() || r.dbClient == nil || w == nil || !v1.IsWorkloadStatusOffloadEnabled(w) {
		return
	}
	pods, err := r.dbClient.ListWorkloadPods(ctx, workloadId)
	if err != nil {
		klog.ErrorS(err, "failed to list workload pods from DB for hydration", "workloadId", workloadId)
	} else if len(pods) > 0 {
		w.Status.Pods = dbclient.WorkloadPodsToV1(pods)
	}
	rows, err := r.dbClient.ListWorkloadDispatchNodes(ctx, workloadId)
	if err != nil {
		klog.ErrorS(err, "failed to list workload dispatch nodes from DB for hydration", "workloadId", workloadId)
	} else if len(rows) > 0 {
		w.Status.Nodes = dbclient.DispatchNodesToV1(rows)
		w.Status.Ranks = dbclient.DispatchRanksToV1(rows)
	}
}

// persistWorkloadStatus writes workload status to etcd. When offload is enabled
// it mirrors pod/dispatch detail to DB, stores the O(node) NodeUsage aggregate in
// etcd, and clears the large per-pod arrays from etcd.
func (r *SyncerReconciler) persistWorkloadStatus(ctx context.Context, w *v1.Workload) error {
	if !commonconfig.IsDBEnable() || r.dbClient == nil || !v1.IsWorkloadStatusOffloadEnabled(w) {
		return jobutils.UpdateWorkloadStatusWithRetry(ctx, r.Client, w)
	}
	if err := r.writeWorkloadStatusToDB(ctx, w); err != nil {
		return err
	}
	w.Status.NodeUsage = commonworkload.BuildNodeUsage(w)
	w.Status.Pods = nil
	w.Status.Nodes = nil
	w.Status.Ranks = nil
	return jobutils.UpdateWorkloadStatusWithRetry(ctx, r.Client, w)
}

// writeWorkloadStatusToDB upserts the live pod set and dispatch history rows.
func (r *SyncerReconciler) writeWorkloadStatusToDB(ctx context.Context, w *v1.Workload) error {
	if r.dbClient == nil {
		return fmt.Errorf("database client unavailable while workload status offload is enabled")
	}
	dispatchCount := v1.GetWorkloadDispatchCnt(w)
	pods := make([]*dbclient.WorkloadPod, 0, len(w.Status.Pods))
	keepPodIds := make([]string, 0, len(w.Status.Pods))
	for i := range w.Status.Pods {
		p := &w.Status.Pods[i]
		pods = append(pods, dbclient.WorkloadPodFromV1(w.Name, dispatchCount, p))
		keepPodIds = append(keepPodIds, p.PodId)
	}
	if err := r.dbClient.BatchUpsertWorkloadPods(ctx, pods); err != nil {
		return err
	}
	if err := r.dbClient.DeleteWorkloadPodsNotIn(ctx, w.Name, keepPodIds); err != nil {
		return err
	}
	for _, row := range dbclient.WorkloadDispatchNodesFromV1(w.Name, w.Status.Nodes, w.Status.Ranks) {
		if row == nil {
			continue
		}
		if err := r.dbClient.UpsertWorkloadDispatchNode(ctx, row); err != nil {
			return err
		}
	}
	return nil
}
