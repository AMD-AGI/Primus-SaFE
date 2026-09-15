/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

// maxWorkloadNodesHistory is the number of past runs kept in nodes_history.
const maxWorkloadNodesHistory = 10

// maxWorkloadNodesHistoryBytes bounds the JSON stored in nodes_history so a
// detail or bulk SELECT cannot pull unbounded node-name payloads.
const maxWorkloadNodesHistoryBytes = 64 * 1024

var (
	selectWorkloadForNodesArchiveCmd = fmt.Sprintf(
		`SELECT workload_id, nodes_history FROM %s WHERE workload_id = $1 FOR UPDATE`, TWorkload)
	listWorkloadDispatchNodesForArchiveCmd = listWorkloadRunSQL(TWorkloadDispatchNode, "dispatch_index")
	updateWorkloadNodesHistoryCmd          = fmt.Sprintf(
		`UPDATE %s SET nodes_history = $1 WHERE workload_id = $2`, TWorkload)
	deleteWorkloadDispatchNodesOfOtherRunsCmd = fmt.Sprintf(
		`DELETE FROM %s WHERE workload_id = $1 AND workload_uid <> $2`, TWorkloadDispatchNode)
	deleteWorkloadPodsOfOtherRunsCmd = fmt.Sprintf(
		`DELETE FROM %s WHERE workload_id = $1 AND workload_uid <> $2`, TWorkloadPod)
)

// WorkloadNodesHistoryEntry is the node assignment of one run of a workload id,
// together with the minimal context needed to tell the runs apart. A resumed
// workload reuses its workload id, so the previous CR generation is archived
// after the new object exists.
type WorkloadNodesHistoryEntry struct {
	DispatchCount int        `json:"dispatchCount"`
	Phase         string     `json:"phase,omitempty"`
	StartTime     string     `json:"startTime,omitempty"`
	EndTime       string     `json:"endTime,omitempty"`
	Nodes         [][]string `json:"nodes"`
	Ranks         [][]string `json:"ranks,omitempty"`
}

// DecodeWorkloadNodesHistory decodes the nodes_history column, oldest run
// first. It returns nil when the column is empty or unreadable.
func DecodeWorkloadNodesHistory(raw string) []WorkloadNodesHistoryEntry {
	if raw == "" {
		return nil
	}
	var entries []WorkloadNodesHistoryEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	return entries
}

// buildWorkloadNodesHistoryEntry snapshots the node assignment of the stored
// run. Nodes come from the dispatch rows when the run was offloaded, and from
// the mirrored nodes column otherwise. It returns nil when the run never got
// any node, which leaves nothing worth archiving.
func buildWorkloadNodesHistoryEntry(old *Workload, rows []*WorkloadDispatchNode) (*WorkloadNodesHistoryEntry, error) {
	nodes := DispatchNodesToV1(rows)
	ranks := DispatchRanksToV1(rows)
	if len(nodes) == 0 {
		if raw := dbutils.ParseNullString(old.Nodes); raw != "" {
			if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
				nodes = nil
			}
		}
		if raw := dbutils.ParseNullString(old.Ranks); raw != "" {
			if err := json.Unmarshal([]byte(raw), &ranks); err != nil {
				ranks = nil
			}
		}
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	return &WorkloadNodesHistoryEntry{
		DispatchCount: old.DispatchCount,
		Phase:         dbutils.ParseNullString(old.Phase),
		StartTime:     dbutils.ParseNullTimeToString(old.StartTime),
		EndTime:       dbutils.ParseNullTimeToString(old.EndTime),
		Nodes:         nodes,
		Ranks:         ranks,
	}, nil
}

// appendWorkloadNodesHistory appends entry to raw and keeps the most recent
// maxWorkloadNodesHistory runs.
func appendWorkloadNodesHistory(raw string, entry *WorkloadNodesHistoryEntry) (string, error) {
	entries := DecodeWorkloadNodesHistory(raw)
	dropped := 0
	entries = append(entries, *entry)
	if len(entries) > maxWorkloadNodesHistory {
		dropped = len(entries) - maxWorkloadNodesHistory
		entries = entries[dropped:]
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	for len(encoded) > maxWorkloadNodesHistoryBytes && len(entries) > 1 {
		entries = entries[1:]
		encoded, err = json.Marshal(entries)
		if err != nil {
			return "", err
		}
	}
	if len(encoded) > maxWorkloadNodesHistoryBytes {
		return "", fmt.Errorf("workload nodes history entry exceeds %d bytes", maxWorkloadNodesHistoryBytes)
	}
	return string(encoded), nil
}

// ArchiveWorkloadNodesForResume snapshots the previous CR generation into
// nodes_history and deletes older run rows. keepUid is the UID of the newly
// created object; its rows are left in place. Called after Create succeeds so a
// failed Create cannot drop the previous run.
func (c *Client) ArchiveWorkloadNodesForResume(ctx context.Context, previous *Workload, keepUid string) error {
	if previous == nil || previous.WorkloadId == "" {
		return fmt.Errorf("workloadId is empty")
	}
	if keepUid == "" {
		return fmt.Errorf("keepUid is empty")
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

	current := &Workload{}
	if err = tx.GetContext(ctx, current, selectWorkloadForNodesArchiveCmd, previous.WorkloadId); err != nil {
		return err
	}
	var rows []*WorkloadDispatchNode
	if err = tx.SelectContext(ctx, &rows, listWorkloadDispatchNodesForArchiveCmd,
		previous.WorkloadId, dbutils.ParseNullString(previous.WorkloadUId)); err != nil {
		return err
	}
	entry, err := buildWorkloadNodesHistoryEntry(previous, rows)
	if err != nil {
		return err
	}
	if entry == nil {
		klog.Infof("previous workload run has no node assignment; retaining its rows, workloadId=%s",
			previous.WorkloadId)
		return tx.Commit()
	}
	oldHistory := DecodeWorkloadNodesHistory(dbutils.ParseNullString(current.NodesHistory))
	history, err := appendWorkloadNodesHistory(dbutils.ParseNullString(current.NodesHistory), entry)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, updateWorkloadNodesHistoryCmd, history, previous.WorkloadId); err != nil {
		return err
	}
	dispatchRes, err := tx.ExecContext(ctx, deleteWorkloadDispatchNodesOfOtherRunsCmd, previous.WorkloadId, keepUid)
	if err != nil {
		return err
	}
	podRes, err := tx.ExecContext(ctx, deleteWorkloadPodsOfOtherRunsCmd, previous.WorkloadId, keepUid)
	if err != nil {
		return err
	}
	dispatchN, _ := dispatchRes.RowsAffected()
	podN, _ := podRes.RowsAffected()
	if err = tx.Commit(); err != nil {
		return err
	}
	dropped := len(oldHistory) + 1 - len(DecodeWorkloadNodesHistory(history))
	klog.Infof("archived previous workload run, workloadId=%s keepUid=%s historyEntries=%d dispatchDeleted=%d podsDeleted=%d",
		previous.WorkloadId, keepUid, 1, dispatchN, podN)
	if dropped > 0 {
		klog.Infof("workload nodes history dropped %d older run(s) at the cap, workloadId=%s",
			dropped, previous.WorkloadId)
	}
	return nil
}
