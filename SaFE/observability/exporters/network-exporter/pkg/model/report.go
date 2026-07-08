// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"fmt"
)

// ReportFlowKey uniquely identifies a flow by (pid, remote_addr, remote_port, direction).
type ReportFlowKey struct {
	Pid   uint32
	Raddr string
	Rport int
	// "egress" or "ingress"
	Direction string
}

func (k ReportFlowKey) String() string {
	return fmt.Sprintf("%d_%s_%d_%s", k.Pid, k.Raddr, k.Rport, k.Direction)
}

// ReportFlowValue holds aggregated traffic bytes for a single flow key.
type ReportFlowValue struct {
	EgressBytes  uint64
	IngressBytes uint64
}

// ReportFlowEntry is the final entry sent to fault-manager.
type ReportFlowEntry struct {
	ProcessName  string `json:"process_name"`
	Pid          uint32 `json:"pid"`
	NsPid        uint32 `json:"nspid"`
	RemoteAddr   string `json:"remote_addr"`
	RemotePort   int    `json:"remote_port"`
	EgressBytes  uint64 `json:"egress_bytes"`
	IngressBytes uint64 `json:"ingress_bytes"`
}

// ReportPayload is the top-level payload sent to fault-manager.
type ReportPayload struct {
	NodeName  string            `json:"node_name"`
	Timestamp int64             `json:"timestamp"`
	Flows     []ReportFlowEntry `json:"flows"`
}
