// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Stub for local development without clang. Deleted during Docker build
// after go generate produces the real bpf_bpfel_*.go files.

//go:build !ebpf_generated

package rdmactl

import "github.com/cilium/ebpf"

type bpfObjects struct {
	TraceIbModifyQp *ebpf.Program `ebpf:"trace_ib_modify_qp"`
	Events          *ebpf.Map     `ebpf:"events"`
}

func loadBpf() (*ebpf.CollectionSpec, error) {
	panic("rdmactl: run 'go generate' with clang to compile BPF programs")
}
