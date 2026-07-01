/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"database/sql"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

// WorkloadPodFromV1 maps a Workload status pod into its DB row. It is the single
// serialization contract shared by the writer and readers so the round-trip with
// ToV1 stays lossless.
func WorkloadPodFromV1(workloadId string, dispatchCount int, p *v1.WorkloadPod) *WorkloadPod {
	if p == nil {
		return nil
	}
	row := &WorkloadPod{
		WorkloadId:    workloadId,
		PodId:         p.PodId,
		ResourceId:    int(p.ResourceId),
		AdminNodeName: dbutils.NullString(p.AdminNodeName),
		HostIP:        dbutils.NullString(p.HostIp),
		PodIP:         dbutils.NullString(p.PodIp),
		Rank:          dbutils.NullString(p.Rank),
		GroupId:       int(p.GroupId),
		Phase:         dbutils.NullString(string(p.Phase)),
		StartTime:     dbutils.NullString(p.StartTime),
		EndTime:       dbutils.NullString(p.EndTime),
		FailedMessage: dbutils.NullString(p.FailedMessage),
		DispatchCount: dispatchCount,
	}
	if len(p.Containers) > 0 {
		if b, err := json.Marshal(p.Containers); err == nil {
			row.Containers = dbutils.NullString(string(b))
		}
	}
	return row
}

// ToV1 reconstructs an etcd-shaped WorkloadPod from a DB row. Containers are
// decoded from the JSON column; a decode failure yields no containers rather
// than an error so a single bad row cannot break a whole list read.
func (p *WorkloadPod) ToV1() v1.WorkloadPod {
	out := v1.WorkloadPod{
		PodId:         p.PodId,
		ResourceId:    int8(p.ResourceId),
		AdminNodeName: dbutils.ParseNullString(p.AdminNodeName),
		Phase:         corev1.PodPhase(dbutils.ParseNullString(p.Phase)),
		HostIp:        dbutils.ParseNullString(p.HostIP),
		PodIp:         dbutils.ParseNullString(p.PodIP),
		Rank:          dbutils.ParseNullString(p.Rank),
		StartTime:     dbutils.ParseNullString(p.StartTime),
		EndTime:       dbutils.ParseNullString(p.EndTime),
		FailedMessage: dbutils.ParseNullString(p.FailedMessage),
		GroupId:       int8(p.GroupId),
	}
	if s := dbutils.ParseNullString(p.Containers); s != "" {
		var cs []v1.Container
		if err := json.Unmarshal([]byte(s), &cs); err == nil {
			out.Containers = cs
		}
	}
	return out
}

// WorkloadPodsToV1 converts DB rows into an etcd-shaped pod slice.
func WorkloadPodsToV1(rows []*WorkloadPod) []v1.WorkloadPod {
	if len(rows) == 0 {
		return nil
	}
	pods := make([]v1.WorkloadPod, 0, len(rows))
	for _, r := range rows {
		if r == nil {
			continue
		}
		pods = append(pods, r.ToV1())
	}
	return pods
}

// WorkloadDispatchNodesFromV1 maps the per-dispatch Nodes/Ranks history into DB
// rows (one row per dispatch index).
func WorkloadDispatchNodesFromV1(workloadId string, nodes, ranks [][]string) []*WorkloadDispatchNode {
	n := len(nodes)
	if len(ranks) > n {
		n = len(ranks)
	}
	rows := make([]*WorkloadDispatchNode, 0, n)
	for i := 0; i < n; i++ {
		row := &WorkloadDispatchNode{WorkloadId: workloadId, DispatchIndex: i}
		if i < len(nodes) {
			if b, err := json.Marshal(nodes[i]); err == nil {
				row.Nodes = dbutils.NullString(string(b))
			}
		}
		if i < len(ranks) {
			if b, err := json.Marshal(ranks[i]); err == nil {
				row.Ranks = dbutils.NullString(string(b))
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// DispatchNodesToV1 rebuilds the [][]string dispatch-node history (ordered by
// dispatch index) from DB rows.
func DispatchNodesToV1(rows []*WorkloadDispatchNode) [][]string {
	if len(rows) == 0 {
		return nil
	}
	maxIdx := -1
	for _, r := range rows {
		if r != nil && r.DispatchIndex > maxIdx {
			maxIdx = r.DispatchIndex
		}
	}
	if maxIdx < 0 {
		return nil
	}
	out := make([][]string, maxIdx+1)
	for _, r := range rows {
		if r == nil || r.DispatchIndex < 0 || r.DispatchIndex > maxIdx {
			continue
		}
		out[r.DispatchIndex] = decodeStringSlice(r.Nodes)
	}
	return out
}

// LatestDispatchNodes returns the node list of the highest dispatch index row.
func LatestDispatchNodes(rows []*WorkloadDispatchNode) []string {
	var latest *WorkloadDispatchNode
	for i := range rows {
		if rows[i] == nil {
			continue
		}
		if latest == nil || rows[i].DispatchIndex > latest.DispatchIndex {
			latest = rows[i]
		}
	}
	if latest == nil {
		return nil
	}
	return decodeStringSlice(latest.Nodes)
}

// decodeStringSlice decodes a JSON []string column, returning nil on absence or
// decode error.
func decodeStringSlice(ns sql.NullString) []string {
	s := dbutils.ParseNullString(ns)
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
