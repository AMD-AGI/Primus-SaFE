// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rdmactl

// RdmaCtrlEvent matches the BPF struct rdma_ctrl_event in rdmactl.bpf.c.
type RdmaCtrlEvent struct {
	QPN       uint32
	RemoteQPN uint32
	RemoteGID [16]byte
	RemoteLID uint16
	PortNum   uint8
	Pad       uint8
	PID       uint32
	AttrMask  uint32
}
