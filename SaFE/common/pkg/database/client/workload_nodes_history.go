/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"encoding/json"
	"fmt"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

// maxWorkloadNodesHistory is the number of past runs kept in nodes_history.
const maxWorkloadNodesHistory = 10

var (
	selectWorkloadForNodesArchiveCmd = fmt.Sprintf(`SELECT
		workload_id, dispatch_count, phase, start_time, end_time, nodes, nodes_history
		FROM %s WHERE workload_id = $1 FOR UPDATE`, TWorkload)
	listWorkloadDispatchNodesForArchiveCmd = fmt.Sprintf(
		`SELECT * FROM %s WHERE workload_id = $1 ORDER BY dispatch_index`, TWorkloadDispatchNode)
	updateWorkloadNodesHistoryCmd = fmt.Sprintf(
		`UPDATE %s SET nodes_history = $1, nodes = NULL, ranks = NULL WHERE workload_id = $2`, TWorkload)
	deleteWorkloadDispatchNodesForArchiveCmd = fmt.Sprintf(
		`DELETE FROM %s WHERE workload_id = $1`, TWorkloadDispatchNode)
)

// WorkloadNodesHistoryEntry is the node assignment of one run of a workload id,
// together with the minimal context needed to tell the runs apart. A resumed
// workload reuses its workload id, so the stored run is archived here before the
// exporter overwrites the row.
type WorkloadNodesHistoryEntry struct {
	DispatchCount int        `json:"dispatchCount"`
	Phase         string     `json:"phase,omitempty"`
	StartTime     string     `json:"startTime,omitempty"`
	EndTime       string     `json:"endTime,omitempty"`
	Nodes         [][]string `json:"nodes"`
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
	if len(nodes) == 0 {
		if raw := dbutils.ParseNullString(old.Nodes); raw != "" {
			if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
				return nil, fmt.Errorf("decode workload nodes: %w", err)
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
	}, nil
}

// appendWorkloadNodesHistory appends entry to raw and keeps the most recent
// maxWorkloadNodesHistory runs.
func appendWorkloadNodesHistory(raw string, entry *WorkloadNodesHistoryEntry) (string, error) {
	var entries []WorkloadNodesHistoryEntry
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return "", fmt.Errorf("decode workload nodes history: %w", err)
		}
	}
	entries = append(entries, *entry)
	if len(entries) > maxWorkloadNodesHistory {
		entries = entries[len(entries)-maxWorkloadNodesHistory:]
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ArchiveWorkloadNodesForResume moves the node assignment of the stored run
// into nodes_history immediately before a resumed workload is created. The
// archive and dispatch-row cleanup are atomic, so a failed cleanup is retried
// without exposing a partially archived run.
func (c *Client) ArchiveWorkloadNodesForResume(ctx context.Context, workloadId string) error {
	if workloadId == "" {
		return fmt.Errorf("workloadId is empty")
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

	old := &Workload{}
	if err = tx.GetContext(ctx, old, selectWorkloadForNodesArchiveCmd, workloadId); err != nil {
		return err
	}
	var rows []*WorkloadDispatchNode
	if err = tx.SelectContext(ctx, &rows, listWorkloadDispatchNodesForArchiveCmd, workloadId); err != nil {
		return err
	}
	entry, err := buildWorkloadNodesHistoryEntry(old, rows)
	if err != nil {
		return err
	}
	if entry != nil {
		history, err := appendWorkloadNodesHistory(dbutils.ParseNullString(old.NodesHistory), entry)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, updateWorkloadNodesHistoryCmd, history, workloadId); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, deleteWorkloadDispatchNodesForArchiveCmd, workloadId); err != nil {
		return err
	}
	return tx.Commit()
}
