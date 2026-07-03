/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TWorkloadPod = "workload_pod"
)

var (
	// upsertWorkloadPodCmd inserts or updates a single pod row keyed by
	// (workload_id, pod_id). resource_id / pod_id form the natural identity.
	upsertWorkloadPodCmd = `INSERT INTO ` + TWorkloadPod + ` (
		workload_id, pod_id, resource_id, admin_node_name, host_ip, pod_ip, rank,
		group_id, phase, start_time, end_time, failed_message, containers,
		dispatch_count, updated_at
	) VALUES (
		:workload_id, :pod_id, :resource_id, :admin_node_name, :host_ip, :pod_ip, :rank,
		:group_id, :phase, :start_time, :end_time, :failed_message, :containers,
		:dispatch_count, :updated_at
	) ON CONFLICT (workload_id, pod_id) DO UPDATE SET
		resource_id = EXCLUDED.resource_id,
		admin_node_name = EXCLUDED.admin_node_name,
		host_ip = EXCLUDED.host_ip,
		pod_ip = EXCLUDED.pod_ip,
		rank = EXCLUDED.rank,
		group_id = EXCLUDED.group_id,
		phase = EXCLUDED.phase,
		start_time = EXCLUDED.start_time,
		end_time = EXCLUDED.end_time,
		failed_message = EXCLUDED.failed_message,
		containers = EXCLUDED.containers,
		dispatch_count = EXCLUDED.dispatch_count,
		updated_at = EXCLUDED.updated_at`

	listWorkloadPodsCmd = fmt.Sprintf(`SELECT * FROM %s WHERE workload_id = $1 ORDER BY pod_id`, TWorkloadPod)
)

// UpsertWorkloadPod inserts or updates a single workload pod row.
func (c *Client) UpsertWorkloadPod(ctx context.Context, pod *WorkloadPod) error {
	if pod == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	setWorkloadPodUpdatedAt(pod)
	if _, err = db.NamedExecContext(ctx, upsertWorkloadPodCmd, pod); err != nil {
		klog.ErrorS(err, "failed to upsert workload pod", "workloadId", pod.WorkloadId, "podId", pod.PodId)
	}
	return err
}

// BatchUpsertWorkloadPods upserts many pods of a workload in a single
// transaction so the etcd-offloaded pod set is written atomically.
func (c *Client) BatchUpsertWorkloadPods(ctx context.Context, pods []*WorkloadPod) error {
	if len(pods) == 0 {
		return nil
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		setWorkloadPodUpdatedAt(pod)
		if _, err = tx.NamedExecContext(ctx, upsertWorkloadPodCmd, pod); err != nil {
			klog.ErrorS(err, "failed to batch upsert workload pod", "workloadId", pod.WorkloadId, "podId", pod.PodId)
			return err
		}
	}
	return tx.Commit()
}

// ListWorkloadPods returns all pods of a workload ordered by pod id.
func (c *Client) ListWorkloadPods(ctx context.Context, workloadId string) ([]*WorkloadPod, error) {
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("workloadId is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	var pods []*WorkloadPod
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &pods, listWorkloadPodsCmd, workloadId)
	} else {
		err = db.SelectContext(ctx, &pods, listWorkloadPodsCmd, workloadId)
	}
	return pods, err
}

// DeleteWorkloadPods removes all pods of a workload (e.g. on workload deletion).
func (c *Client) DeleteWorkloadPods(ctx context.Context, workloadId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`DELETE FROM %s WHERE workload_id = $1`, TWorkloadPod)
	if _, err = db.ExecContext(ctx, cmd, workloadId); err != nil {
		klog.ErrorS(err, "failed to delete workload pods", "workloadId", workloadId)
	}
	return err
}

// DeleteWorkloadPodsNotIn removes pods of a workload whose pod_id is not in
// keepPodIds, used to reconcile the DB pod set with the live cluster state. When
// keepPodIds is empty it removes every pod of the workload.
func (c *Client) DeleteWorkloadPodsNotIn(ctx context.Context, workloadId string, keepPodIds []string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	if len(keepPodIds) == 0 {
		return c.DeleteWorkloadPods(ctx, workloadId)
	}
	query, args, err := sqlx.In(
		fmt.Sprintf(`DELETE FROM %s WHERE workload_id = ? AND pod_id NOT IN (?)`, TWorkloadPod),
		workloadId, keepPodIds)
	if err != nil {
		return err
	}
	query = db.Rebind(query)
	if _, err = db.ExecContext(ctx, query, args...); err != nil {
		klog.ErrorS(err, "failed to delete stale workload pods", "workloadId", workloadId)
	}
	return err
}

// setWorkloadPodUpdatedAt stamps updated_at with the current time when unset so
// callers do not have to manage it.
func setWorkloadPodUpdatedAt(pod *WorkloadPod) {
	if !pod.UpdatedAt.Valid {
		pod.UpdatedAt = pq.NullTime{Time: time.Now().UTC(), Valid: true}
	}
}
