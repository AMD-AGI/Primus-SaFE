// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

//go:build !ebpf_generated

package rdmaflow

import "github.com/cilium/ebpf"

type bpfObjects struct {
	TracePostSend *ebpf.Program `ebpf:"trace_post_send"`
	Events        *ebpf.Map     `ebpf:"events"`
}

func loadBpf() (*ebpf.CollectionSpec, error) {
	panic("rdmaflow: run 'go generate' with clang to compile BPF programs")
}
