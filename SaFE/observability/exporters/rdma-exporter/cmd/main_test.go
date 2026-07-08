// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/discovery"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
)

// --- fakes ---

type fakeQPLister struct {
	qps []model.RDMAQP
	err error
}

func (f *fakeQPLister) ListQPs() ([]model.RDMAQP, error) { return f.qps, f.err }

type fakeUprobeManager struct {
	attached  map[int]string // pid -> libPath
	detached  []int
}

func newFakeUprobeManager(initialPIDs ...int) *fakeUprobeManager {
	m := &fakeUprobeManager{attached: make(map[int]string)}
	for _, pid := range initialPIDs {
		m.attached[pid] = "/fake/lib.so"
	}
	return m
}

func (f *fakeUprobeManager) AttachTo(pid int, libPath string, sym string) error {
	f.attached[pid] = libPath
	return nil
}

func (f *fakeUprobeManager) AttachedPIDs() map[int]struct{} {
	result := make(map[int]struct{}, len(f.attached))
	for pid := range f.attached {
		result[pid] = struct{}{}
	}
	return result
}

func (f *fakeUprobeManager) DetachPID(pid int) {
	delete(f.attached, pid)
	f.detached = append(f.detached, pid)
}

type fakeDiscoverer struct {
	result []discovery.RDMAProcess
}

func (f *fakeDiscoverer) FindRDMAProcesses(pids []int) []discovery.RDMAProcess {
	var out []discovery.RDMAProcess
	pidSet := make(map[int]struct{}, len(pids))
	for _, p := range pids {
		pidSet[p] = struct{}{}
	}
	for _, r := range f.result {
		if _, ok := pidSet[r.PID]; ok {
			out = append(out, r)
		}
	}
	return out
}

// --- tests ---

func TestDiscoverAndAttach_EmptyQPs_DetachAll(t *testing.T) {
	lister := &fakeQPLister{qps: []model.RDMAQP{}}
	flow := newFakeUprobeManager(100, 200, 300)
	disc := &fakeDiscoverer{}

	discoverAndAttach(lister, flow, disc)

	if len(flow.attached) != 0 {
		t.Fatalf("expected 0 attached after empty QPs, got %d: %v", len(flow.attached), flow.attached)
	}
	if len(flow.detached) != 3 {
		t.Fatalf("expected 3 detachments, got %d", len(flow.detached))
	}
}

func TestDiscoverAndAttach_SubsetDetach(t *testing.T) {
	lister := &fakeQPLister{qps: []model.RDMAQP{
		{LQPN: 10, Type: "RC", PID: 100, Comm: "train"},
		{LQPN: 20, Type: "RC", PID: 300, Comm: "train"},
	}}
	flow := newFakeUprobeManager(100, 200, 300)
	disc := &fakeDiscoverer{result: []discovery.RDMAProcess{
		{PID: 100, LibPath: "/lib/a.so", PostSendSym: "fn"},
		{PID: 300, LibPath: "/lib/b.so", PostSendSym: "fn"},
	}}

	discoverAndAttach(lister, flow, disc)

	if _, ok := flow.attached[200]; ok {
		t.Fatal("PID 200 should have been detached")
	}
	if len(flow.detached) != 1 || flow.detached[0] != 200 {
		t.Fatalf("expected [200] detached, got %v", flow.detached)
	}
	if _, ok := flow.attached[100]; !ok {
		t.Fatal("PID 100 should still be attached")
	}
	if _, ok := flow.attached[300]; !ok {
		t.Fatal("PID 300 should still be attached")
	}
}

func TestDiscoverAndAttach_NewPIDAttached(t *testing.T) {
	lister := &fakeQPLister{qps: []model.RDMAQP{
		{LQPN: 10, Type: "RC", PID: 100, Comm: "train"},
		{LQPN: 20, Type: "RC", PID: 400, Comm: "new-job"},
	}}
	flow := newFakeUprobeManager(100)
	disc := &fakeDiscoverer{result: []discovery.RDMAProcess{
		{PID: 100, LibPath: "/lib/a.so", PostSendSym: "fn"},
		{PID: 400, LibPath: "/lib/b.so", PostSendSym: "fn"},
	}}

	discoverAndAttach(lister, flow, disc)

	if _, ok := flow.attached[400]; !ok {
		t.Fatal("PID 400 should have been attached")
	}
	if len(flow.detached) != 0 {
		t.Fatalf("expected no detachments, got %v", flow.detached)
	}
}

func TestDiscoverAndAttach_GSIAndZeroPIDSkipped(t *testing.T) {
	lister := &fakeQPLister{qps: []model.RDMAQP{
		{LQPN: 1, Type: "GSI", PID: 0, Comm: "ib_core"},
		{LQPN: 10, Type: "RC", PID: 500, Comm: "train"},
	}}
	flow := newFakeUprobeManager()
	disc := &fakeDiscoverer{result: []discovery.RDMAProcess{
		{PID: 500, LibPath: "/lib/a.so", PostSendSym: "fn"},
	}}

	discoverAndAttach(lister, flow, disc)

	if _, ok := flow.attached[500]; !ok {
		t.Fatal("PID 500 should have been attached")
	}
	if len(flow.attached) != 1 {
		t.Fatalf("expected exactly 1 attached, got %d", len(flow.attached))
	}
}

func TestListenWithRetry_InterruptedByCancel(t *testing.T) {
	// Occupy a port so Listen always fails
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	occupiedAddr := ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, retryErr := listenWithRetry(ctx, occupiedAddr, 30, 2*time.Second)
	elapsed := time.Since(start)

	if retryErr == nil {
		t.Fatal("expected error from cancelled context")
	}
	if elapsed > 1*time.Second {
		t.Fatalf("expected fast exit on cancel, took %v", elapsed)
	}
}
