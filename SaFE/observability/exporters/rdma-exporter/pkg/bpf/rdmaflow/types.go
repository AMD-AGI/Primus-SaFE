// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rdmaflow

// RdmaSendEvent matches the BPF struct rdma_send_event in rdmaflow.bpf.c.
type RdmaSendEvent struct {
	QPN    uint32
	PID    uint32
	Opcode uint32
	Pad    uint32
	Bytes  uint64
}
