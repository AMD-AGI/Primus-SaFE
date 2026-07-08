// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
	"github.com/prometheus/client_golang/prometheus"
)

func newTestCommMetrics() *CommMetrics {
	return &CommMetrics{
		connMap:     make(map[ConnKey]model.ConnectionInfo),
		qpIndex:     make(map[ConnKey]model.QPInfo),
		activePairs: make(map[ConnKey]pairLabels),
		seenTx:      make(map[txSeriesKey][]string),
		nodeName:    "test-node",
		txBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_rdma_comm_tx_bytes_total",
			Help: "test",
		}, commLabels),
		txOps: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_rdma_comm_tx_ops_total",
			Help: "test",
		}, commLabels),
		activePair: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_rdma_comm_active_pair_info",
			Help: "test",
		}, activePairLabels),
	}
}

func TestConnKeyNonConflict(t *testing.T) {
	cm := newTestCommMetrics()

	k1 := ConnKey{PID: 100, QPN: 5}
	k2 := ConnKey{PID: 200, QPN: 5}

	cm.connMap[k1] = model.ConnectionInfo{RemoteGID: "gid-a"}
	cm.connMap[k2] = model.ConnectionInfo{RemoteGID: "gid-b"}

	if cm.connMap[k1].RemoteGID != "gid-a" {
		t.Fatalf("key (100,5) overwritten: got %s", cm.connMap[k1].RemoteGID)
	}
	if cm.connMap[k2].RemoteGID != "gid-b" {
		t.Fatalf("key (200,5) wrong: got %s", cm.connMap[k2].RemoteGID)
	}
	if len(cm.connMap) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cm.connMap))
	}
}

func TestJoinHitAndMiss(t *testing.T) {
	cm := newTestCommMetrics()

	cm.connMap[ConnKey{PID: 10, QPN: 42}] = model.ConnectionInfo{
		RemoteQPN: 99, RemoteGID: "fe80::1", RemoteLID: 1,
	}
	cm.qpIndex[ConnKey{PID: 10, QPN: 42}] = model.QPInfo{
		Device: "bnxt_re0", Port: 1, Comm: "python3",
	}

	hit := ConnKey{PID: 10, QPN: 42}
	cm.mu.RLock()
	conn, hasConn := cm.connMap[hit]
	cm.mu.RUnlock()
	if !hasConn || conn.RemoteGID != "fe80::1" {
		t.Fatalf("join hit failed: hasConn=%v gid=%s", hasConn, conn.RemoteGID)
	}

	miss := ConnKey{PID: 10, QPN: 999}
	cm.mu.RLock()
	_, hasMiss := cm.connMap[miss]
	cm.mu.RUnlock()
	if hasMiss {
		t.Fatal("expected join miss for unknown QPN")
	}

	qpInfo := cm.lookupQPInfo(hit)
	if qpInfo.Device != "bnxt_re0" || qpInfo.Comm != "python3" {
		t.Fatalf("qpInfo lookup failed: %+v", qpInfo)
	}

	missInfo := cm.lookupQPInfo(miss)
	if missInfo.Device != "" {
		t.Fatalf("expected empty QPInfo for miss, got %+v", missInfo)
	}
}

func TestUpdateQPIndex(t *testing.T) {
	cm := newTestCommMetrics()

	qps := []model.RDMAQP{
		{IfName: "bnxt_re0", Port: 1, LQPN: 10, Type: "RC", PID: 100, Comm: "train"},
		{IfName: "bnxt_re1", Port: 1, LQPN: 20, Type: "RC", PID: 200, Comm: "infer"},
		{IfName: "bnxt_re0", Port: 1, LQPN: 1, Type: "GSI", PID: 0, Comm: "ib_core"},
	}
	cm.UpdateQPIndex(qps)

	if len(cm.qpIndex) != 2 {
		t.Fatalf("expected 2 entries (GSI skipped), got %d", len(cm.qpIndex))
	}
	info := cm.lookupQPInfo(ConnKey{PID: 100, QPN: 10})
	if info.Device != "bnxt_re0" || info.Comm != "train" {
		t.Fatalf("unexpected QPInfo: %+v", info)
	}
}

func TestActivePairBackfillOnQPIndex(t *testing.T) {
	cm := newTestCommMetrics()

	// Connection arrives BEFORE QP metadata
	cm.UpdateConnection(100, 42, model.ConnectionInfo{
		RemoteQPN: 99, RemoteGID: "fe80::1", RemoteLID: 5,
	})

	pairs := cm.ActivePairsSnapshot()
	if pairs[ConnKey{PID: 100, QPN: 42}].LocalDevice != "" {
		t.Fatal("expected empty local_device before QP index")
	}

	// QP metadata arrives later
	cm.UpdateQPIndex([]model.RDMAQP{
		{IfName: "bnxt_re0", Port: 1, LQPN: 42, Type: "RC", PID: 100, Comm: "train"},
	})

	pairs = cm.ActivePairsSnapshot()
	p := pairs[ConnKey{PID: 100, QPN: 42}]
	if p.LocalDevice != "bnxt_re0" || p.Comm != "train" {
		t.Fatalf("backfill failed: %+v", p)
	}
}

func TestActivePairIncrementalDeleteStale(t *testing.T) {
	cm := newTestCommMetrics()

	cm.UpdateConnection(100, 10, model.ConnectionInfo{
		RemoteQPN: 50, RemoteGID: "fe80::a",
	})
	if len(cm.ActivePairsSnapshot()) != 1 {
		t.Fatal("expected 1 active pair after connection")
	}

	// Remove from connMap and sync via UpdateQPIndex
	cm.mu.Lock()
	delete(cm.connMap, ConnKey{PID: 100, QPN: 10})
	cm.mu.Unlock()

	cm.UpdateQPIndex([]model.RDMAQP{})

	pairs := cm.ActivePairsSnapshot()
	if len(pairs) != 0 {
		t.Fatalf("expected 0 active pairs after stale removal, got %d", len(pairs))
	}
}

func TestActivePairNoResetFlicker(t *testing.T) {
	cm := newTestCommMetrics()

	cm.UpdateConnection(100, 10, model.ConnectionInfo{
		RemoteQPN: 50, RemoteGID: "fe80::a",
	})
	cm.UpdateConnection(200, 20, model.ConnectionInfo{
		RemoteQPN: 60, RemoteGID: "fe80::b",
	})

	// UpdateQPIndex should NOT produce a moment where activePairs is empty
	// (the old Reset approach would). Verify pairs survive across sync.
	cm.UpdateQPIndex([]model.RDMAQP{
		{IfName: "bnxt_re0", Port: 1, LQPN: 10, Type: "RC", PID: 100, Comm: "train"},
		{IfName: "bnxt_re1", Port: 1, LQPN: 20, Type: "RC", PID: 200, Comm: "infer"},
	})

	pairs := cm.ActivePairsSnapshot()
	if len(pairs) != 2 {
		t.Fatalf("expected 2 active pairs preserved, got %d", len(pairs))
	}
	if pairs[ConnKey{PID: 100, QPN: 10}].Comm != "train" {
		t.Fatal("pair (100,10) label mismatch")
	}
	if pairs[ConnKey{PID: 200, QPN: 20}].Comm != "infer" {
		t.Fatal("pair (200,20) label mismatch")
	}
}

func TestCleanInactiveCountersSelectiveDelete(t *testing.T) {
	cm := newTestCommMetrics()

	cm.qpIndex[ConnKey{PID: 100, QPN: 10}] = model.QPInfo{Device: "ionic_0", Port: 1, Comm: "train"}
	cm.qpIndex[ConnKey{PID: 200, QPN: 20}] = model.QPInfo{Device: "ionic_2", Port: 1, Comm: "train"}

	cm.OnSendEvent(100, 10, 0, 1024)
	cm.OnSendEvent(200, 20, 2, 2048)

	if len(cm.seenTx) != 2 {
		t.Fatalf("expected 2 seenTx entries, got %d", len(cm.seenTx))
	}

	active := map[ConnKey]struct{}{
		{PID: 100, QPN: 10}: {},
	}
	cm.CleanInactiveCounters(active)

	if len(cm.seenTx) != 1 {
		t.Fatalf("expected 1 seenTx after clean, got %d", len(cm.seenTx))
	}
	for tk := range cm.seenTx {
		if tk.ConnKey.QPN != 10 {
			t.Fatalf("wrong surviving key: QPN=%d", tk.ConnKey.QPN)
		}
	}
}

func TestOpcodeString(t *testing.T) {
	tests := []struct {
		op   uint32
		want string
	}{
		{0, "RDMA_WRITE"},
		{2, "SEND"},
		{4, "RDMA_READ"},
		{99, "OP_99"},
	}
	for _, tt := range tests {
		got := opcodeString(tt.op)
		if got != tt.want {
			t.Errorf("opcodeString(%d)=%q, want %q", tt.op, got, tt.want)
		}
	}
}
