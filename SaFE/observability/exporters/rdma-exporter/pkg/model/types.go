// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

// RDMADevice represents an RDMA device from "rdma dev show -j"
type RDMADevice struct {
	IfIndex      int    `json:"ifindex"`
	IfName       string `json:"ifname"`
	NodeType     string `json:"node_type"`
	FW           string `json:"fw"`
	NodeGUID     string `json:"node_guid"`
	SysImageGUID string `json:"sys_image_guid"`
}

// RDMAStat represents statistics for an RDMA device/port
type RDMAStat struct {
	Device string
	Port   string
	Stats  map[string]int64
}

// RDMAQP represents a Queue Pair from "rdma res show qp -j"
type RDMAQP struct {
	IfIndex int    `json:"ifindex"`
	IfName  string `json:"ifname"`
	Port    int    `json:"port"`
	LQPN    int    `json:"lqpn"`
	RQPN    int    `json:"rqpn,omitempty"`
	Type    string `json:"type"`
	State   string `json:"state"`
	SQPSN   int    `json:"sq-psn"`
	Comm    string `json:"comm,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

// ConnectionInfo holds the remote endpoint info captured by the kprobe on ib_modify_qp.
type ConnectionInfo struct {
	QPN       uint32    `json:"qpn"`
	RemoteQPN uint32    `json:"remote_qpn"`
	RemoteGID string    `json:"remote_gid"`
	RemoteLID uint16    `json:"remote_lid"`
	PortNum   uint8     `json:"port_num"`
	PID       uint32    `json:"pid"`
	LastSeen  time.Time `json:"last_seen"`
}

// RDMASendEvent is the Go representation of a BPF uprobe event from bnxt_re_post_send.
type RDMASendEvent struct {
	QPN    uint32
	PID    uint32
	Opcode uint32
	Bytes  uint64
}

// QPInfo holds local-side QP metadata resolved from rdma res.
type QPInfo struct {
	Device string
	Port   int
	Comm   string
}
