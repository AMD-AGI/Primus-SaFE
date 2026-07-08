// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rdmactl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
)

func TestEncodeDecodeConnKey(t *testing.T) {
	k := ConnKey{PID: 12345, QPN: 67}
	s := encodeConnKey(k)
	if s != "12345-67" {
		t.Fatalf("encode: got %q", s)
	}
	k2, err := decodeConnKey(s)
	if err != nil {
		t.Fatal(err)
	}
	if k2 != k {
		t.Fatalf("roundtrip: got %+v", k2)
	}
}

func TestDecodeConnKeyBad(t *testing.T) {
	for _, bad := range []string{"", "abc", "1", "a-b"} {
		if _, err := decodeConnKey(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	h := &Handler{
		connMap: map[ConnKey]model.ConnectionInfo{
			{PID: 100, QPN: 10}: {
				QPN: 10, RemoteQPN: 20, RemoteGID: "fe80::1",
				RemoteLID: 5, PortNum: 1, PID: 100,
				LastSeen: time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
			},
			{PID: 200, QPN: 30}: {
				QPN: 30, RemoteQPN: 40, RemoteGID: "fe80::2",
				PID: 200, LastSeen: time.Date(2026, 3, 24, 11, 0, 0, 0, time.UTC),
			},
		},
		persistPath: path,
	}

	if err := h.SaveState(); err != nil {
		t.Fatal(err)
	}

	h2 := &Handler{connMap: make(map[ConnKey]model.ConnectionInfo)}
	if err := h2.LoadState(path); err != nil {
		t.Fatal(err)
	}
	if len(h2.connMap) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(h2.connMap))
	}

	info := h2.connMap[ConnKey{PID: 100, QPN: 10}]
	if info.RemoteGID != "fe80::1" || info.RemoteQPN != 20 {
		t.Fatalf("data mismatch: %+v", info)
	}
}

func TestLoadStateNotExist(t *testing.T) {
	h := &Handler{connMap: make(map[ConnKey]model.ConnectionInfo)}
	if err := h.LoadState("/nonexistent/path.json"); err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
}

func TestLoadStateBadEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{"connections":{"bad-key-xxx":{"qpn":1},"100-10":{"qpn":10,"remote_gid":"ok"}},"saved_at":"2026-03-24T12:00:00Z"}`), 0644)

	h := &Handler{connMap: make(map[ConnKey]model.ConnectionInfo)}
	if err := h.LoadState(path); err != nil {
		t.Fatal(err)
	}
	if len(h.connMap) != 1 {
		t.Fatalf("expected 1 (bad entry skipped), got %d", len(h.connMap))
	}
}

func TestReplayTriggersCallback(t *testing.T) {
	var called []uint32
	h := &Handler{
		connMap: map[ConnKey]model.ConnectionInfo{
			{PID: 1, QPN: 10}: {QPN: 10, PID: 1},
			{PID: 2, QPN: 20}: {QPN: 20, PID: 2},
		},
		onConnection: func(pid, qpn uint32, info model.ConnectionInfo) {
			called = append(called, qpn)
		},
	}
	h.ReplayConnections()
	if len(called) != 2 {
		t.Fatalf("expected 2 callbacks, got %d", len(called))
	}
}

func TestCleanStale(t *testing.T) {
	now := time.Now()
	h := &Handler{
		connMap: map[ConnKey]model.ConnectionInfo{
			{PID: 1, QPN: 10}: {QPN: 10, LastSeen: now.Add(-25 * time.Hour)},
			{PID: 2, QPN: 20}: {QPN: 20, LastSeen: now.Add(-1 * time.Hour)},
			{PID: 3, QPN: 30}: {QPN: 30, LastSeen: now},
		},
	}
	h.CleanStale(24 * time.Hour)
	if len(h.connMap) != 2 {
		t.Fatalf("expected 2 after clean, got %d", len(h.connMap))
	}
	if _, ok := h.connMap[ConnKey{PID: 1, QPN: 10}]; ok {
		t.Fatal("stale entry should have been removed")
	}
}
