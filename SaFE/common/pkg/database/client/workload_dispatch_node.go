/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TWorkloadDispatchNode = "workload_dispatch_node"
)

var (
	// upsertWorkloadDispatchNodeCmd inserts or updates one dispatch's node/rank
	// assignment keyed by (workload_id, dispatch_index).
	upsertWorkloadDispatchNodeCmd = `INSERT INTO ` + TWorkloadDispatchNode + ` (
		workload_id, dispatch_index, nodes, ranks, updated_at
	) VALUES (
		:workload_id, :dispatch_index, :nodes, :ranks, :updated_at
	) ON CONFLICT (workload_id, dispatch_index) DO UPDATE SET
		nodes = EXCLUDED.nodes,
		ranks = EXCLUDED.ranks,
		updated_at = EXCLUDED.updated_at`

	listWorkloadDispatchNodesCmd = fmt.Sprintf(
		`SELECT * FROM %s WHERE workload_id = $1 ORDER BY dispatch_index`, TWorkloadDispatchNode)
)

// UpsertWorkloadDispatchNode inserts or updates one dispatch's node/rank row.
func (c *Client) UpsertWorkloadDispatchNode(ctx context.Context, dn *WorkloadDispatchNode) error {
	if dn == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	if !dn.UpdatedAt.Valid {
		dn.UpdatedAt = pq.NullTime{Time: time.Now().UTC(), Valid: true}
	}
	if _, err = db.NamedExecContext(ctx, upsertWorkloadDispatchNodeCmd, dn); err != nil {
		klog.ErrorS(err, "failed to upsert workload dispatch node",
			"workloadId", dn.WorkloadId, "dispatchIndex", dn.DispatchIndex)
	}
	return err
}

// ListWorkloadDispatchNodes returns all dispatch rows of a workload ordered by
// dispatch index (ascending; the last element is the latest dispatch).
func (c *Client) ListWorkloadDispatchNodes(ctx context.Context, workloadId string) ([]*WorkloadDispatchNode, error) {
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("workloadId is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	var rows []*WorkloadDispatchNode
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &rows, listWorkloadDispatchNodesCmd, workloadId)
	} else {
		err = db.SelectContext(ctx, &rows, listWorkloadDispatchNodesCmd, workloadId)
	}
	return rows, err
}

// DeleteWorkloadDispatchNodes removes all dispatch rows of a workload.
func (c *Client) DeleteWorkloadDispatchNodes(ctx context.Context, workloadId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`DELETE FROM %s WHERE workload_id = $1`, TWorkloadDispatchNode)
	if _, err = db.ExecContext(ctx, cmd, workloadId); err != nil {
		klog.ErrorS(err, "failed to delete workload dispatch nodes", "workloadId", workloadId)
	}
	return err
}
