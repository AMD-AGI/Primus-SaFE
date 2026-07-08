// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rdmaflow

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/prometheus/client_golang/prometheus"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64 -cc clang -cflags "-I../../../../include -D__TARGET_ARCH_x86" bpf ./bpf/rdmaflow.bpf.c

var removeMemlock sync.Once

// OnSendEventFunc is called for each uprobe send event.
type OnSendEventFunc func(pid, qpn, opcode uint32, bytes uint64)

// FlowKey identifies an active QP by pid+qpn for cleanup.
type FlowKey struct {
	PID uint32
	QPN uint32
}

type seriesKey struct {
	qpn    string
	pid    string
	device string
	source string
}

// Handler manages uprobes on provider post_send and aggregates per-QP byte counters.
type Handler struct {
	objs      *bpfObjects
	reader    *ringbuf.Reader
	eventChan chan RdmaSendEvent
	links     map[int]link.Link // pid -> uprobe link
	linksMu   sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	seenSeries  map[seriesKey]struct{}
	seenMu      sync.Mutex
	txBytes     *prometheus.GaugeVec
	txOps       *prometheus.GaugeVec
	nodeName    string
	onSendEvent OnSendEventFunc
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

	reader, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		return nil, fmt.Errorf("create ringbuf reader: %w", err)
	}

	txBytes := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rdma_qp_tx_bytes",
		Help: "Cumulative bytes sent per QP",
	}, []string{"node", "qpn", "pid", "device", "source"})

	txOps := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rdma_qp_tx_ops",
		Help: "Cumulative send operations per QP",
	}, []string{"node", "qpn", "pid", "device", "source"})

	prometheus.MustRegister(txBytes, txOps)

	return &Handler{
		objs:       objs,
		reader:     reader,
		eventChan:  make(chan RdmaSendEvent, 65536),
		links:      make(map[int]link.Link),
		seenSeries: make(map[seriesKey]struct{}),
		txBytes:    txBytes,
		txOps:      txOps,
		nodeName:   nodeName,
	}, nil
}

// SetOnSendEvent registers a callback for send events.
func (h *Handler) SetOnSendEvent(fn OnSendEventFunc) {
	h.onSendEvent = fn
}

// AttachTo attaches the uprobe to a specific PID's provider library.
func (h *Handler) AttachTo(pid int, libPath string, symbolName string) error {
	h.linksMu.Lock()
	defer h.linksMu.Unlock()

	if _, exists := h.links[pid]; exists {
		return nil
	}

	ex, err := link.OpenExecutable(libPath)
	if err != nil {
		return fmt.Errorf("open executable %s: %w", libPath, err)
	}

	up, err := ex.Uprobe(symbolName, h.objs.TracePostSend, &link.UprobeOptions{PID: pid})
	if err != nil {
		return fmt.Errorf("attach uprobe to pid %d: %w", pid, err)
	}

	h.links[pid] = up
	slog.Info("uprobe attached", "pid", pid, "lib", libPath, "symbol", symbolName)
	return nil
}

// DetachPID removes the uprobe for a specific PID.
func (h *Handler) DetachPID(pid int) {
	h.linksMu.Lock()
	defer h.linksMu.Unlock()

	if l, ok := h.links[pid]; ok {
		l.Close()
		delete(h.links, pid)
		slog.Info("uprobe detached", "pid", pid)
	}
}

// AttachedPIDs returns the set of currently attached PIDs.
func (h *Handler) AttachedPIDs() map[int]struct{} {
	h.linksMu.Lock()
	defer h.linksMu.Unlock()
	result := make(map[int]struct{}, len(h.links))
	for pid := range h.links {
		result[pid] = struct{}{}
	}
	return result
}

// CleanInactiveQPs removes gauge series for QPs not in the active set (pid+qpn).
func (h *Handler) CleanInactiveQPs(active map[FlowKey]struct{}) {
	var stale []seriesKey

	h.seenMu.Lock()
	for sk := range h.seenSeries {
		pid, _ := strconv.ParseUint(sk.pid, 10, 32)
		qpn, _ := strconv.ParseUint(sk.qpn, 10, 32)
		if _, ok := active[FlowKey{PID: uint32(pid), QPN: uint32(qpn)}]; !ok {
			stale = append(stale, sk)
			delete(h.seenSeries, sk)
		}
	}
	h.seenMu.Unlock()

	for _, sk := range stale {
		h.txBytes.DeleteLabelValues(h.nodeName, sk.qpn, sk.pid, sk.device, sk.source)
		h.txOps.DeleteLabelValues(h.nodeName, sk.qpn, sk.pid, sk.device, sk.source)
	}
}

func (h *Handler) Start(ctx context.Context) {
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
		h.processLoop(ctx2)
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
			slog.Error("rdmaflow ringbuf read", "error", err)
			continue
		}
		var ev RdmaSendEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &ev); err != nil {
			slog.Error("rdmaflow parse event", "error", err)
			continue
		}
		select {
		case h.eventChan <- ev:
		default:
		}
	}
}

func (h *Handler) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-h.eventChan:
			if !ok {
				return
			}
			qpn := strconv.FormatUint(uint64(ev.QPN), 10)
			pid := strconv.FormatUint(uint64(ev.PID), 10)
			source := "uprobe_provider"
			h.txBytes.WithLabelValues(h.nodeName, qpn, pid, "", source).Add(float64(ev.Bytes))
			h.txOps.WithLabelValues(h.nodeName, qpn, pid, "", source).Add(1)
			h.seenMu.Lock()
			h.seenSeries[seriesKey{qpn: qpn, pid: pid, device: "", source: source}] = struct{}{}
			h.seenMu.Unlock()
			if h.onSendEvent != nil {
				h.onSendEvent(ev.PID, ev.QPN, ev.Opcode, ev.Bytes)
			}
		}
	}
}

func (h *Handler) Close() {
	if h.cancel != nil {
		h.cancel()
	}
	if h.reader != nil {
		h.reader.Close()
	}
	h.wg.Wait()

	h.linksMu.Lock()
	for pid, l := range h.links {
		l.Close()
		delete(h.links, pid)
	}
	h.linksMu.Unlock()
}
