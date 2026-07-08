// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rdmactl

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64 -cc clang -cflags "-I../../../../include -D__TARGET_ARCH_x86" bpf ./bpf/rdmactl.bpf.c

var removeMemlock sync.Once

// ConnKey uniquely identifies a QP connection across processes.
type ConnKey struct {
	PID uint32
	QPN uint32
}

// OnConnectionFunc is called when a new QP connection is observed.
type OnConnectionFunc func(pid, qpn uint32, info model.ConnectionInfo)

// Handler manages the kprobe on ib_modify_qp and maintains a connection map.
type Handler struct {
	objs         *bpfObjects
	kpLink       link.Link
	reader       *ringbuf.Reader
	eventChan    chan RdmaCtrlEvent
	connMap      map[ConnKey]model.ConnectionInfo
	mu           sync.RWMutex
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	connInfo     *prometheus.GaugeVec
	onConnection OnConnectionFunc
	deviceCache  map[uint32]string
	dcMu         sync.RWMutex
	executor     interface{ Execute(string, ...string) ([]byte, error) }
	persistPath  string
}

func New(nodeName string) (*Handler, error) {
	var err error
	removeMemlock.Do(func() { err = rlimit.RemoveMemlock() })
	if err != nil {
		return nil, fmt.Errorf("remove memlock: %w", err)
	}

	spec, err := loadBpf()
	if err != nil {
		return nil, fmt.Errorf("load bpf spec: %w", err)
	}

	objs := &bpfObjects{}
	if err := spec.LoadAndAssign(objs, nil); err != nil {
		return nil, fmt.Errorf("load bpf objects: %w", err)
	}

	kpLink, err := link.Kprobe("_ib_modify_qp", objs.TraceIbModifyQp, nil)
	if err != nil {
		return nil, fmt.Errorf("attach kprobe: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		kpLink.Close()
		return nil, fmt.Errorf("create ringbuf reader: %w", err)
	}

	connInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rdma_connection_info",
		Help: "RDMA QP connection remote endpoint (value is always 1)",
	}, []string{"node", "device", "lqpn", "rqpn", "remote_gid", "remote_lid", "port_num"})
	prometheus.MustRegister(connInfo)

	h := &Handler{
		objs:        objs,
		kpLink:      kpLink,
		reader:      reader,
		eventChan:   make(chan RdmaCtrlEvent, 4096),
		connMap:     make(map[ConnKey]model.ConnectionInfo),
		connInfo:    connInfo,
		deviceCache: make(map[uint32]string),
	}
	return h, nil
}

// SetOnConnection registers a callback for connection events.
func (h *Handler) SetOnConnection(fn OnConnectionFunc) {
	h.onConnection = fn
}

// SetExecutor injects the command executor for device cache refresh.
func (h *Handler) SetExecutor(exec interface{ Execute(string, ...string) ([]byte, error) }) {
	h.executor = exec
}

func (h *Handler) refreshDeviceCache() {
	if h.executor == nil {
		return
	}
	out, err := h.executor.Execute("rdma", "res", "show", "qp", "-j")
	if err != nil {
		return
	}
	var qps []struct {
		IfName string `json:"ifname"`
		LQPN   int    `json:"lqpn"`
		PID    int    `json:"pid"`
		Type   string `json:"type"`
	}
	if json.Unmarshal(out, &qps) != nil {
		return
	}

	activeKeys := make(map[ConnKey]struct{})
	h.dcMu.Lock()
	for _, qp := range qps {
		h.deviceCache[uint32(qp.LQPN)] = qp.IfName
		if qp.Type != "GSI" && qp.PID != 0 {
			activeKeys[ConnKey{PID: uint32(qp.PID), QPN: uint32(qp.LQPN)}] = struct{}{}
		}
	}
	h.dcMu.Unlock()

	h.markInactiveConns(activeKeys)
}

// markInactiveConns checks connMap entries against the active QP set.
// Entries not in the active set have their LastSeen left as-is (they will
// be cleaned by CleanStale based on the age threshold).
// Entries still active get their LastSeen refreshed to now.
func (h *Handler) markInactiveConns(activeKeys map[ConnKey]struct{}) {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, info := range h.connMap {
		if _, active := activeKeys[key]; active {
			info.LastSeen = now
			h.connMap[key] = info
		}
	}
}

func (h *Handler) rebuildConnMetrics(nodeName string) {
	h.mu.RLock()
	conns := make(map[ConnKey]model.ConnectionInfo, len(h.connMap))
	for k, v := range h.connMap {
		conns[k] = v
	}
	h.mu.RUnlock()

	h.dcMu.RLock()
	defer h.dcMu.RUnlock()

	h.connInfo.Reset()
	for _, info := range conns {
		device := h.deviceCache[info.QPN]
		h.connInfo.WithLabelValues(
			nodeName, device,
			fmt.Sprint(info.QPN), fmt.Sprint(info.RemoteQPN),
			info.RemoteGID, fmt.Sprint(info.RemoteLID), fmt.Sprint(info.PortNum),
		).Set(1)
	}
}

func (h *Handler) Start(ctx context.Context, nodeName string) {
	ctx2, cancel := context.WithCancel(ctx)
	h.cancel = cancel

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.readLoop(ctx2)
	}()

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.processLoop(ctx2, nodeName)
	}()

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		refreshTicker := time.NewTicker(5 * time.Second)
		persistTicker := time.NewTicker(30 * time.Second)
		staleTicker := time.NewTicker(1 * time.Minute)
		defer refreshTicker.Stop()
		defer persistTicker.Stop()
		defer staleTicker.Stop()
		for {
			h.refreshDeviceCache()
			h.rebuildConnMetrics(nodeName)
			select {
			case <-ctx2.Done():
				return
			case <-refreshTicker.C:
			case <-persistTicker.C:
				if err := h.SaveState(); err != nil {
					slog.Error("persist state", "error", err)
				}
			case <-staleTicker.C:
				h.CleanStale(5 * time.Minute)
			}
		}
	}()
}

func (h *Handler) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		record, err := h.reader.Read()
		if err != nil {
			if err == ringbuf.ErrClosed {
				return
			}
			slog.Error("rdmactl ringbuf read", "error", err)
			continue
		}
		var ev RdmaCtrlEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &ev); err != nil {
			slog.Error("rdmactl parse event", "error", err)
			continue
		}
		select {
		case h.eventChan <- ev:
		default:
			slog.Warn("rdmactl event channel full, dropping event")
		}
	}
}

func (h *Handler) processLoop(ctx context.Context, nodeName string) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-h.eventChan:
			if !ok {
				return
			}
			gid := formatGID(ev.RemoteGID)
			info := model.ConnectionInfo{
				QPN:       ev.QPN,
				RemoteQPN: ev.RemoteQPN,
				RemoteGID: gid,
				RemoteLID: ev.RemoteLID,
				PortNum:   ev.PortNum,
				PID:       ev.PID,
				LastSeen:  time.Now().UTC(),
			}
			key := ConnKey{PID: ev.PID, QPN: ev.QPN}
			h.mu.Lock()
			h.connMap[key] = info
			h.mu.Unlock()

			if h.onConnection != nil {
				h.onConnection(ev.PID, ev.QPN, info)
			}

			slog.Debug("rdma connection",
				"qpn", ev.QPN, "remote_qpn", ev.RemoteQPN,
				"remote_gid", gid, "pid", ev.PID)
		}
	}
}

// GetConnectionMap returns a snapshot of the current (pid,qpn)->ConnectionInfo map.
func (h *Handler) GetConnectionMap() map[ConnKey]model.ConnectionInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make(map[ConnKey]model.ConnectionInfo, len(h.connMap))
	for k, v := range h.connMap {
		result[k] = v
	}
	return result
}

func (h *Handler) Close() {
	if h.cancel != nil {
		h.cancel()
	}
	if h.reader != nil {
		h.reader.Close()
	}
	h.wg.Wait()
	if h.kpLink != nil {
		h.kpLink.Close()
	}
}

func formatGID(gid [16]byte) string {
	return fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
		gid[0], gid[1], gid[2], gid[3],
		gid[4], gid[5], gid[6], gid[7],
		gid[8], gid[9], gid[10], gid[11],
		gid[12], gid[13], gid[14], gid[15])
}
