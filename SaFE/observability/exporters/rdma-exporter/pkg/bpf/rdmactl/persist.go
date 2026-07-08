// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rdmactl

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
)

// PersistedConnection is the JSON-safe representation of a connection.
type PersistedConnection struct {
	QPN       uint32    `json:"qpn"`
	RemoteQPN uint32    `json:"remote_qpn"`
	RemoteGID string    `json:"remote_gid"`
	RemoteLID uint16    `json:"remote_lid"`
	PortNum   uint8     `json:"port_num"`
	PID       uint32    `json:"pid"`
	LastSeen  time.Time `json:"last_seen"`
}

// PersistedState is the on-disk state file format.
type PersistedState struct {
	Connections map[string]PersistedConnection `json:"connections"`
	SavedAt     time.Time                      `json:"saved_at"`
}

func encodeConnKey(k ConnKey) string {
	return fmt.Sprintf("%d-%d", k.PID, k.QPN)
}

func decodeConnKey(s string) (ConnKey, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return ConnKey{}, fmt.Errorf("invalid key %q", s)
	}
	pid, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return ConnKey{}, err
	}
	qpn, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return ConnKey{}, err
	}
	return ConnKey{PID: uint32(pid), QPN: uint32(qpn)}, nil
}

func toPersistedConn(info model.ConnectionInfo) PersistedConnection {
	return PersistedConnection{
		QPN: info.QPN, RemoteQPN: info.RemoteQPN,
		RemoteGID: info.RemoteGID, RemoteLID: info.RemoteLID,
		PortNum: info.PortNum, PID: info.PID, LastSeen: info.LastSeen,
	}
}

func fromPersistedConn(pc PersistedConnection) model.ConnectionInfo {
	return model.ConnectionInfo{
		QPN: pc.QPN, RemoteQPN: pc.RemoteQPN,
		RemoteGID: pc.RemoteGID, RemoteLID: pc.RemoteLID,
		PortNum: pc.PortNum, PID: pc.PID, LastSeen: pc.LastSeen,
	}
}

// LoadState reads the persisted state file and populates connMap.
func (h *Handler) LoadState(path string) error {
	h.persistPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("no persisted state file", "path", path)
			return nil
		}
		return fmt.Errorf("read state: %w", err)
	}

	var state PersistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parse state: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for keyStr, pc := range state.Connections {
		key, err := decodeConnKey(keyStr)
		if err != nil {
			slog.Warn("skip bad key in state", "key", keyStr, "error", err)
			continue
		}
		info := fromPersistedConn(pc)
		if info.LastSeen.IsZero() {
			info.LastSeen = state.SavedAt
		}
		h.connMap[key] = info
	}

	slog.Info("loaded persisted connections", "count", len(state.Connections), "saved_at", state.SavedAt)
	return nil
}

// SaveState writes the current connMap to disk atomically.
func (h *Handler) SaveState() error {
	if h.persistPath == "" {
		return nil
	}

	h.mu.RLock()
	conns := make(map[string]PersistedConnection, len(h.connMap))
	for k, v := range h.connMap {
		conns[encodeConnKey(k)] = toPersistedConn(v)
	}
	h.mu.RUnlock()

	state := PersistedState{
		Connections: conns,
		SavedAt:     time.Now().UTC(),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	dir := filepath.Dir(h.persistPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	tmp := h.persistPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, h.persistPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename state: %w", err)
	}

	slog.Debug("state saved", "connections", len(conns))
	return nil
}

// ReplayConnections triggers onConnection for every entry in connMap,
// bringing the unified metrics layer up to date after load or scan.
func (h *Handler) ReplayConnections() {
	if h.onConnection == nil {
		return
	}
	h.mu.RLock()
	snapshot := make(map[ConnKey]model.ConnectionInfo, len(h.connMap))
	for k, v := range h.connMap {
		snapshot[k] = v
	}
	h.mu.RUnlock()

	for _, info := range snapshot {
		h.onConnection(info.PID, info.QPN, info)
	}
	slog.Info("replayed connections to metrics layer", "count", len(snapshot))
}

// CleanStale removes connections not seen within maxAge.
func (h *Handler) CleanStale(maxAge time.Duration) {
	now := time.Now()
	h.mu.Lock()
	var removed int
	for k, info := range h.connMap {
		if !info.LastSeen.IsZero() && now.Sub(info.LastSeen) > maxAge {
			delete(h.connMap, k)
			removed++
		}
	}
	h.mu.Unlock()
	if removed > 0 {
		slog.Info("cleaned stale metric entries", "removed", removed)
	}
}

// ScanExistingQPs discovers active QPs via "rdma res show qp" and creates
// connection entries for QPNs not yet in connMap (partial info, no remote GID).
func (h *Handler) ScanExistingQPs() {
	if h.executor == nil {
		return
	}
	out, err := h.executor.Execute("rdma", "res", "show", "qp", "-j")
	if err != nil {
		return
	}
	var qps []struct {
		IfName string `json:"ifname"`
		Port   int    `json:"port"`
		LQPN   int    `json:"lqpn"`
		RQPN   int    `json:"rqpn"`
		PID    int    `json:"pid"`
		Type   string `json:"type"`
	}
	if json.Unmarshal(out, &qps) != nil {
		return
	}

	now := time.Now().UTC()
	h.mu.Lock()
	var added int
	for _, qp := range qps {
		if qp.Type == "GSI" || qp.PID == 0 {
			continue
		}
		key := ConnKey{PID: uint32(qp.PID), QPN: uint32(qp.LQPN)}
		if _, exists := h.connMap[key]; exists {
			continue
		}
		h.connMap[key] = model.ConnectionInfo{
			QPN:       uint32(qp.LQPN),
			RemoteQPN: uint32(qp.RQPN),
			PortNum:   uint8(qp.Port),
			PID:       uint32(qp.PID),
			LastSeen:  now,
		}
		added++
	}
	h.mu.Unlock()

	if added > 0 {
		slog.Info("scan discovered existing QPs", "added", added)
	}
}
