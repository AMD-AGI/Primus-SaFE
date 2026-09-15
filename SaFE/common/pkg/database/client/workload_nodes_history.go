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

// maxWorkloadNodesHistoryBytes bounds the JSON stored in nodes_history so a
// detail GET cannot pull unbounded node-name payloads.
const maxWorkloadNodesHistoryBytes = 64 * 1024

var errWorkloadNodesHistoryCorrupt = fmt.Errorf("workload nodes history is not valid JSON")

var (
	selectWorkloadForNodesArchiveCmd = fmt.Sprintf(
		`SELECT workload_id, nodes_history FROM %s WHERE workload_id = $1`, TWorkload)
	updateWorkloadNodesHistoryCmd = fmt.Sprintf(
		`UPDATE %s SET nodes_history = $1 WHERE workload_id = $2`, TWorkload)
	deleteWorkloadDispatchNodesForResumeCmd = fmt.Sprintf(
		`DELETE FROM %s WHERE workload_id = $1 AND workload_uid IS DISTINCT FROM $2`, TWorkloadDispatchNode)
	deleteWorkloadPodsForResumeCmd = fmt.Sprintf(
		`DELETE FROM %s WHERE workload_id = $1 AND workload_uid IS DISTINCT FROM $2`, TWorkloadPod)
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
	entries, err := decodeWorkloadNodesHistory(raw)
	if err != nil {
		return nil
	}
	return entries
}

func decodeWorkloadNodesHistory(raw string) ([]WorkloadNodesHistoryEntry, error) {
	if raw == "" {
		return nil, nil
	}
	var entries []WorkloadNodesHistoryEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, errWorkloadNodesHistoryCorrupt
	}
	return entries, nil
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
				return nil, fmt.Errorf("decode legacy workload nodes: %w", err)
			}
		}
		if raw := dbutils.ParseNullString(old.Ranks); raw != "" {
			if err := json.Unmarshal([]byte(raw), &ranks); err != nil {
				return nil, fmt.Errorf("decode legacy workload ranks: %w", err)
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
// Older runs are removed first to meet both the count and byte limits. A single
// latest run is retained in full even when it exceeds the soft byte limit.
func appendWorkloadNodesHistory(
	raw string, entry *WorkloadNodesHistoryEntry,
) (string, int, error) {
	entries, err := decodeWorkloadNodesHistory(raw)
	if err != nil {
		return "", 0, err
	}
	entries = append(entries, *entry)
	dropped := 0
	if len(entries) > maxWorkloadNodesHistory {
		dropped = len(entries) - maxWorkloadNodesHistory
		entries = entries[dropped:]
	}
	for {
		encoded, err := json.Marshal(entries)
		if err != nil {
			return "", 0, err
		}
		if len(encoded) <= maxWorkloadNodesHistoryBytes || len(entries) == 1 {
			return string(encoded), dropped, nil
		}
		entries = entries[1:]
		dropped++
	}
}

// ArchiveWorkloadNodesForResume best-effort archives a pre-create dispatch
// snapshot and deletes the previous run's detail rows after CR creation.
func (c *Client) ArchiveWorkloadNodesForResume(
	ctx context.Context, previous *Workload, currentUid string, rows []*WorkloadDispatchNode,
) error {
	if previous == nil || previous.WorkloadId == "" {
		return fmt.Errorf("workloadId is empty")
	}
	if currentUid == "" {
		return fmt.Errorf("current workload UID is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	var archiveErrors []error
	appended := false
	dropped := 0
	archiveBytes := 0
	entry, buildErr := buildWorkloadNodesHistoryEntry(previous, rows)
	if buildErr != nil {
		return buildErr
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
				archiveBytes = len(raw)
			}
		}
	}
	if entry != nil && !appended {
		return errors.Join(archiveErrors...)
	}

	var dispatchN, podN int64
	dispatchRes, dispatchErr := db.ExecContext(
		ctx, deleteWorkloadDispatchNodesForResumeCmd, previous.WorkloadId, currentUid)
	if dispatchErr != nil {
		archiveErrors = append(archiveErrors, dispatchErr)
	} else {
		dispatchN, _ = dispatchRes.RowsAffected()
	}
	podRes, podErr := db.ExecContext(ctx, deleteWorkloadPodsForResumeCmd, previous.WorkloadId, currentUid)
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
	if archiveBytes > maxWorkloadNodesHistoryBytes {
		klog.Infof("workload nodes history retains an oversized latest run, workloadId=%s bytes=%d",
			previous.WorkloadId, archiveBytes)
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
