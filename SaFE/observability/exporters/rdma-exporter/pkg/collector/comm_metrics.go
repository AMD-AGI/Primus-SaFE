// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
	"github.com/prometheus/client_golang/prometheus"
)

// ConnKey uniquely identifies a QP connection across processes.
type ConnKey struct {
	PID uint32
	QPN uint32
}

// pairLabels holds the full label set for one active pair so we can
// delete the exact series later without guessing.
type pairLabels struct {
	Node        string
	PID         string
	Comm        string
	LocalDevice string
	LocalPort   string
	LocalQPN    string
	RemoteQPN   string
	RemoteGID   string
	RemoteLID   string
}

func (p pairLabels) values() []string {
	return []string{p.Node, p.PID, p.Comm, p.LocalDevice, p.LocalPort, p.LocalQPN, p.RemoteQPN, p.RemoteGID, p.RemoteLID}
}

// txSeriesKey tracks a unique tx counter label combination for targeted deletion.
type txSeriesKey struct {
	ConnKey
	Opcode string
}

// CommMetrics is the aggregation layer that joins rdmactl connection info
// with rdmaflow send events to produce unified communication metrics.
type CommMetrics struct {
	mu          sync.RWMutex
	connMap     map[ConnKey]model.ConnectionInfo
	qpIndex     map[ConnKey]model.QPInfo
	activePairs map[ConnKey]pairLabels // tracks emitted active_pair label sets
	seenTx      map[txSeriesKey][]string // tracks emitted tx label values
	nodeName    string

	txBytes    *prometheus.GaugeVec
	txOps      *prometheus.GaugeVec
	activePair *prometheus.GaugeVec
}

// QPInfo holds local-side QP metadata from rdma res.
type QPInfo = model.QPInfo

var commLabels = []string{
	"node", "pid", "comm",
	"local_device", "local_port", "local_qpn",
	"remote_qpn", "remote_gid", "remote_lid",
	"opcode",
}

var activePairLabels = []string{
	"node", "pid", "comm",
	"local_device", "local_port", "local_qpn",
	"remote_qpn", "remote_gid", "remote_lid",
}

func NewCommMetrics(nodeName string) *CommMetrics {
	txBytes := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rdma_comm_tx_bytes_total",
		Help: "Cumulative bytes sent per RDMA communication pair",
	}, commLabels)

	txOps := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rdma_comm_tx_ops_total",
		Help: "Cumulative send operations per RDMA communication pair",
	}, commLabels)

	activePair := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rdma_comm_active_pair_info",
		Help: "Active RDMA communication pair (value is always 1)",
	}, activePairLabels)

	prometheus.MustRegister(txBytes, txOps, activePair)

	return &CommMetrics{
		connMap:     make(map[ConnKey]model.ConnectionInfo),
		qpIndex:     make(map[ConnKey]model.QPInfo),
		activePairs: make(map[ConnKey]pairLabels),
		seenTx:      make(map[txSeriesKey][]string),
		nodeName:    nodeName,
		txBytes:     txBytes,
		txOps:       txOps,
		activePair:  activePair,
	}
}

// UpdateConnection stores or updates the remote endpoint mapping for a (pid, qpn) pair.
func (cm *CommMetrics) UpdateConnection(pid, qpn uint32, info model.ConnectionInfo) {
	key := ConnKey{PID: pid, QPN: qpn}
	cm.mu.Lock()
	cm.connMap[key] = info
	cm.mu.Unlock()

	cm.upsertActivePair(key, info)

	slog.Debug("comm connection updated",
		"pid", pid, "qpn", qpn,
		"remote_qpn", info.RemoteQPN, "remote_gid", info.RemoteGID)
}

// OnSendEvent is called for each uprobe send event.
func (cm *CommMetrics) OnSendEvent(pid, qpn, opcode uint32, bytes uint64) {
	key := ConnKey{PID: pid, QPN: qpn}
	cm.mu.RLock()
	conn, hasConn := cm.connMap[key]
	cm.mu.RUnlock()

	qpInfo := cm.lookupQPInfo(key)

	remoteQPN := ""
	remoteGID := ""
	remoteLID := ""
	if hasConn {
		remoteQPN = fmt.Sprint(conn.RemoteQPN)
		remoteGID = conn.RemoteGID
		remoteLID = fmt.Sprint(conn.RemoteLID)
	}

	labels := []string{
		cm.nodeName,
		fmt.Sprint(pid), qpInfo.Comm,
		qpInfo.Device, fmt.Sprint(qpInfo.Port), fmt.Sprint(qpn),
		remoteQPN, remoteGID, remoteLID,
		opcodeString(opcode),
	}

	cm.txBytes.WithLabelValues(labels...).Add(float64(bytes))
	cm.txOps.WithLabelValues(labels...).Add(1)

	tk := txSeriesKey{ConnKey: key, Opcode: opcodeString(opcode)}
	cm.mu.Lock()
	cm.seenTx[tk] = labels
	cm.mu.Unlock()
}

// UpdateQPIndex bulk-updates the QP metadata index, then performs an
// incremental diff-sync on activePair to backfill local labels without Reset.
func (cm *CommMetrics) UpdateQPIndex(qps []model.RDMAQP) {
	cm.mu.Lock()
	cm.qpIndex = make(map[ConnKey]model.QPInfo, len(qps))
	for _, qp := range qps {
		if qp.Type == "GSI" {
			continue
		}
		key := ConnKey{PID: uint32(qp.PID), QPN: uint32(qp.LQPN)}
		cm.qpIndex[key] = model.QPInfo{
			Device: qp.IfName,
			Port:   qp.Port,
			Comm:   qp.Comm,
		}
	}

	connSnap := make(map[ConnKey]model.ConnectionInfo, len(cm.connMap))
	for k, v := range cm.connMap {
		connSnap[k] = v
	}
	cm.mu.Unlock()

	cm.syncActivePairs(connSnap)
}

// upsertActivePair sets the active pair gauge for one key, deleting the
// previous label set if labels changed (e.g. local info got backfilled).
func (cm *CommMetrics) upsertActivePair(key ConnKey, conn model.ConnectionInfo) {
	qpInfo := cm.lookupQPInfo(key)
	newLabels := cm.buildPairLabels(key, conn, qpInfo)

	cm.mu.Lock()
	old, hadOld := cm.activePairs[key]
	cm.activePairs[key] = newLabels
	cm.mu.Unlock()

	if hadOld && old != newLabels {
		cm.activePair.DeleteLabelValues(old.values()...)
	}
	cm.activePair.WithLabelValues(newLabels.values()...).Set(1)
}

// syncActivePairs performs an incremental diff between the desired set
// (connSnap) and the tracked activePairs, upserting new/changed entries
// and deleting stale ones.
func (cm *CommMetrics) syncActivePairs(connSnap map[ConnKey]model.ConnectionInfo) {
	desired := make(map[ConnKey]pairLabels, len(connSnap))
	for key, conn := range connSnap {
		qpInfo := cm.lookupQPInfo(key)
		desired[key] = cm.buildPairLabels(key, conn, qpInfo)
	}

	cm.mu.Lock()
	// Delete stale pairs
	for key, old := range cm.activePairs {
		if _, ok := desired[key]; !ok {
			cm.activePair.DeleteLabelValues(old.values()...)
			delete(cm.activePairs, key)
		}
	}
	// Upsert new or changed pairs
	for key, want := range desired {
		old, hadOld := cm.activePairs[key]
		if hadOld && old == want {
			continue
		}
		if hadOld && old != want {
			cm.activePair.DeleteLabelValues(old.values()...)
		}
		cm.activePair.WithLabelValues(want.values()...).Set(1)
		cm.activePairs[key] = want
	}
	cm.mu.Unlock()
}

func (cm *CommMetrics) buildPairLabels(key ConnKey, conn model.ConnectionInfo, qpInfo model.QPInfo) pairLabels {
	return pairLabels{
		Node:        cm.nodeName,
		PID:         fmt.Sprint(key.PID),
		Comm:        qpInfo.Comm,
		LocalDevice: qpInfo.Device,
		LocalPort:   fmt.Sprint(qpInfo.Port),
		LocalQPN:    fmt.Sprint(key.QPN),
		RemoteQPN:   fmt.Sprint(conn.RemoteQPN),
		RemoteGID:   conn.RemoteGID,
		RemoteLID:   fmt.Sprint(conn.RemoteLID),
	}
}

// CleanInactiveCounters removes tx gauge series for ConnKeys not in the active set.
func (cm *CommMetrics) CleanInactiveCounters(active map[ConnKey]struct{}) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for tk, labels := range cm.seenTx {
		if _, ok := active[tk.ConnKey]; !ok {
			cm.txBytes.DeleteLabelValues(labels...)
			cm.txOps.DeleteLabelValues(labels...)
			delete(cm.seenTx, tk)
		}
	}
}

// ActivePairsSnapshot returns a copy of tracked active pairs for testing.
func (cm *CommMetrics) ActivePairsSnapshot() map[ConnKey]pairLabels {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make(map[ConnKey]pairLabels, len(cm.activePairs))
	for k, v := range cm.activePairs {
		result[k] = v
	}
	return result
}

func (cm *CommMetrics) lookupQPInfo(key ConnKey) model.QPInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if info, ok := cm.qpIndex[key]; ok {
		return info
	}
	for k, info := range cm.qpIndex {
		if k.QPN == key.QPN {
			return info
		}
	}
	return model.QPInfo{}
}

func opcodeString(op uint32) string {
	switch op {
	case 0:
		return "RDMA_WRITE"
	case 1:
		return "RDMA_WRITE_WITH_IMM"
	case 2:
		return "SEND"
	case 3:
		return "SEND_WITH_IMM"
	case 4:
		return "RDMA_READ"
	case 5:
		return "ATOMIC_CMP_AND_SWP"
	case 6:
		return "ATOMIC_FETCH_AND_ADD"
	default:
		return fmt.Sprintf("OP_%d", op)
	}
}
