/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/klog/v2"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

// maxWorkloadNodesHistory is the number of past runs kept in nodes_history.
const maxWorkloadNodesHistory = 10

var (
	selectWorkloadForNodesArchiveCmd = fmt.Sprintf(
		`SELECT workload_id, nodes_history FROM %s WHERE workload_id = $1`, TWorkload)
	updateWorkloadNodesHistoryCmd = fmt.Sprintf(
		`UPDATE %s SET nodes_history = $1 WHERE workload_id = $2`, TWorkload)
	deleteWorkloadDispatchNodesForResumeCmd = fmt.Sprintf(
		`DELETE FROM %s WHERE workload_id = $1`, TWorkloadDispatchNode)
	deleteWorkloadPodsForResumeCmd = fmt.Sprintf(
		`DELETE FROM %s WHERE workload_id = $1`, TWorkloadPod)
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
	if !dispatchNodesContainAssignment(nodes) {
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
	if !dispatchNodesContainAssignment(nodes) {
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

// dispatchNodesContainAssignment reports whether any dispatch contains a node.
func dispatchNodesContainAssignment(nodes [][]string) bool {
	for i := range nodes {
		if len(nodes[i]) > 0 {
			return true
		}
	}
	return false
}

// appendWorkloadNodesHistory appends one run and keeps the newest entries.
func appendWorkloadNodesHistory(
	raw string, entry *WorkloadNodesHistoryEntry,
) (string, int, error) {
	entries := append(DecodeWorkloadNodesHistory(raw), *entry)
	dropped := 0
	if len(entries) > maxWorkloadNodesHistory {
		dropped = len(entries) - maxWorkloadNodesHistory
		entries = entries[dropped:]
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", 0, err
	}
	return string(encoded), dropped, nil
}

// ArchiveWorkloadNodesForResume best-effort archives a pre-create dispatch
// snapshot and deletes the previous run's detail rows after CR creation.
func (c *Client) ArchiveWorkloadNodesForResume(
	ctx context.Context, previous *Workload, rows []*WorkloadDispatchNode,
) error {
	if previous == nil || previous.WorkloadId == "" {
		return fmt.Errorf("workloadId is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	var archiveErrors []error
	appended := false
	dropped := 0
	entry, buildErr := buildWorkloadNodesHistoryEntry(previous, rows)
	if buildErr != nil {
		archiveErrors = append(archiveErrors, buildErr)
	}
	current := &Workload{}
	if err = db.GetContext(ctx, current, selectWorkloadForNodesArchiveCmd, previous.WorkloadId); err != nil {
		archiveErrors = append(archiveErrors, err)
	} else if entry != nil {
		var raw string
		raw, dropped, err = appendWorkloadNodesHistory(dbutils.ParseNullString(current.NodesHistory), entry)
		if err != nil {
			archiveErrors = append(archiveErrors, err)
		} else {
			if _, err = db.ExecContext(ctx, updateWorkloadNodesHistoryCmd, raw, previous.WorkloadId); err != nil {
				archiveErrors = append(archiveErrors, err)
			} else {
				appended = true
			}
		}
	}

	var dispatchN, podN int64
	dispatchRes, dispatchErr := db.ExecContext(ctx, deleteWorkloadDispatchNodesForResumeCmd, previous.WorkloadId)
	if dispatchErr != nil {
		archiveErrors = append(archiveErrors, dispatchErr)
	} else {
		dispatchN, _ = dispatchRes.RowsAffected()
	}
	podRes, podErr := db.ExecContext(ctx, deleteWorkloadPodsForResumeCmd, previous.WorkloadId)
	if podErr != nil {
		archiveErrors = append(archiveErrors, podErr)
	} else {
		podN, _ = podRes.RowsAffected()
	}
	klog.Infof("archived previous workload run, workloadId=%s historyEntries=%d dispatchDeleted=%d podsDeleted=%d",
		previous.WorkloadId, boolToInt(appended), dispatchN, podN)
	if dropped > 0 {
		klog.Infof("workload nodes history dropped %d older run(s) at the cap, workloadId=%s",
			dropped, previous.WorkloadId)
	}
	return errors.Join(archiveErrors...)
}

// boolToInt renders an archive outcome as a log counter.
func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
