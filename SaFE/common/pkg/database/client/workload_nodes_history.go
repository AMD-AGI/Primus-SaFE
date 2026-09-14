/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"encoding/json"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

// maxWorkloadNodesHistory is the number of past runs kept in nodes_history.
const maxWorkloadNodesHistory = 10

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

// isNewWorkloadRun reports whether cur is a new run of an already stored
// workload id, which is what a resume produces: the same id with a fresh UID.
func isNewWorkloadRun(old, cur *Workload) bool {
	oldUid := dbutils.ParseNullString(old.WorkloadUId)
	curUid := dbutils.ParseNullString(cur.WorkloadUId)
	return oldUid != "" && curUid != "" && oldUid != curUid
}

// buildWorkloadNodesHistoryEntry snapshots the node assignment of the stored
// run. Nodes come from the dispatch rows when the run was offloaded, and from
// the mirrored nodes column otherwise. It returns nil when the run never got
// any node, which leaves nothing worth archiving.
func buildWorkloadNodesHistoryEntry(old *Workload, rows []*WorkloadDispatchNode) *WorkloadNodesHistoryEntry {
	nodes := DispatchNodesToV1(rows)
	if len(nodes) == 0 {
		if raw := dbutils.ParseNullString(old.Nodes); raw != "" {
			if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
				nodes = nil
			}
		}
	}
	if len(nodes) == 0 {
		return nil
	}
	return &WorkloadNodesHistoryEntry{
		DispatchCount: old.DispatchCount,
		Phase:         dbutils.ParseNullString(old.Phase),
		StartTime:     dbutils.ParseNullTimeToString(old.StartTime),
		EndTime:       dbutils.ParseNullTimeToString(old.EndTime),
		Nodes:         nodes,
	}
}

// appendWorkloadNodesHistory appends entry to raw and keeps the most recent
// maxWorkloadNodesHistory runs.
func appendWorkloadNodesHistory(raw string, entry *WorkloadNodesHistoryEntry) (string, error) {
	entries := append(DecodeWorkloadNodesHistory(raw), *entry)
	if len(entries) > maxWorkloadNodesHistory {
		entries = entries[len(entries)-maxWorkloadNodesHistory:]
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// prepareWorkloadNodesHistory fills cur.NodesHistory so the update that
// overwrites the stored run carries the history forward, appending that run
// when cur starts a new one. It reports whether a new run was detected, in
// which case the caller drops the dispatch rows of the previous run once the
// update lands. Carrying the column in the same update keeps the archive tied
// to the UID change: a failed update archives nothing and is retried as a whole.
func (c *Client) prepareWorkloadNodesHistory(ctx context.Context, old, cur *Workload) (bool, error) {
	cur.NodesHistory = old.NodesHistory
	if !isNewWorkloadRun(old, cur) {
		return false, nil
	}
	rows, err := c.ListWorkloadDispatchNodes(ctx, old.WorkloadId)
	if err != nil {
		return false, err
	}
	entry := buildWorkloadNodesHistoryEntry(old, rows)
	if entry == nil {
		return true, nil
	}
	history, err := appendWorkloadNodesHistory(dbutils.ParseNullString(old.NodesHistory), entry)
	if err != nil {
		return false, err
	}
	cur.NodesHistory = dbutils.NullString(history)
	return true, nil
}
