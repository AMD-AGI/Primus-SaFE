// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tcpconn

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/metrics"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 -cc clang -cflags $CFLAGS bpf ./bpf/tcpconn.bpf.c

var (
	onceInit sync.Once
)

const (
	AF_INET  = 2  // IPv4
	AF_INET6 = 10 // IPv6
	AF_UNIX  = 1  // Unix Domain Socket
)

const (
	EventTypeProbeClose   = "close"
	EventTypeProbeConnect = "connect"
)

type ConnEvent struct {
	Pid     uint32   `ebpf:"pid"`
	SPort   uint16   `ebpf:"sport"`
	DPort   uint16   `ebpf:"dport"`
	Family  uint16   `ebpf:"family"`
	Saddr   [4]byte  `ebpf:"saddr"`
	Daddr   [4]byte  `ebpf:"daddr"`
	SaddrV6 [16]byte `ebpf:"saddr_v6"`
	DaddrV6 [16]byte `ebpf:"daddr_v6"`
	Typ     [16]byte `ebpf:"typ"`
}

func (c ConnEvent) GetType() string {
	offset := 0
	for i, b := range c.Typ {
		if b == 0 {
			offset = i
			break
		}
	}
	return string(c.Typ[:offset])
}

func (c ConnEvent) String() string {
	switch c.Family {
	case AF_INET:
		return fmt.Sprintf("ConnEvent: Pid %d Family %d Sip %s Sp %d Dip %s Dp %d Type %s",
			c.Pid, c.Family, net.IP(c.Saddr[:]).String(), c.SPort,
			net.IP(c.Daddr[:]).String(), c.DPort, c.GetType())
	case AF_INET6:
		return fmt.Sprintf("ConnEvent: Pid %d Family %d Sip %s Sp %d Dip %s Dp %d Type %s",
			c.Pid, c.Family, net.IP(c.SaddrV6[:]).String(), c.SPort,
			net.IP(c.DaddrV6[:]).String(), c.DPort, c.GetType())
	}
	return ""
}

func (c ConnEvent) GetSip() string {
	switch c.Family {
	case AF_INET:
		return net.IP(c.Saddr[:]).String()
	case AF_INET6:
		return net.IP(c.SaddrV6[:]).String()
	}
	return ""
}

func (c ConnEvent) GetDip() string {
	switch c.Family {
	case AF_INET:
		return net.IP(c.Daddr[:]).String()
	case AF_INET6:
		return net.IP(c.DaddrV6[:]).String()
	}
	return ""
}

type bpfObject struct {
	Close        *ebpf.Program `ebpf:"probe_tcp_close"`
	ProbeConnect *ebpf.Program `ebpf:"probe_tcp_connect"`
	Map          *ebpf.Map     `ebpf:"events"`
}

func NewBpfTcpConn() (*BpfTcpConn, error) {
	b := &BpfTcpConn{}
	if err := b.init(); err != nil {
		return nil, err
	}
	return b, nil
}

type BpfTcpConn struct {
	obj         *bpfObject
	connectLink link.Link
	closeLink   link.Link
	cancelCtx   context.Context
	cancelFunc  context.CancelFunc
	eventChan   chan ConnEvent
	reader      *ringbuf.Reader
}

func (b *BpfTcpConn) Close() {
	b.cancelFunc()
	if b.connectLink != nil {
		b.connectLink.Close()
	}
	if b.closeLink != nil {
		b.closeLink.Close()
	}
	if b.reader != nil {
		b.reader.Close()
	}
}

func (b *BpfTcpConn) Read() <-chan ConnEvent {
	return b.eventChan
}

func (b *BpfTcpConn) Sync() error {
	b.doSyncEvent()
	return nil
}

func (b *BpfTcpConn) Start() {
	go b.Sync()
}

func (b *BpfTcpConn) doSyncEvent() {
	var event ConnEvent
	for {
		select {
		case <-b.cancelCtx.Done():
			return
		default:
			if b.reader == nil {
				time.Sleep(1 * time.Second)
				continue
			}
			record, err := b.reader.Read()
			if err != nil {
				if err == ringbuf.ErrClosed {
					return
				}
				slog.Error("reading from ringbuf", "error", err)
				continue
			}
			err = binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
			if err != nil {
				slog.Error("parsing ringbuf record", "error", err)
				continue
			}
			slog.Debug("Received tcpconn event", "type", event.GetType(), "event", event.String())
			metrics.BpfEventRecv.Inc("tcpconn")
			if b.eventChan != nil {
				select {
				case b.eventChan <- event:
				default:
					metrics.BpfEventChanDrop.Inc("tcpconn")
				}
			}
		}
	}
}

func (b *BpfTcpConn) InitChan(size int) {
	b.eventChan = make(chan ConnEvent, size)
}

func (b *BpfTcpConn) init() error {
	var err error
	b.cancelCtx, b.cancelFunc = context.WithCancel(context.Background())
	onceInit.Do(func() {
		err = rlimit.RemoveMemlock()
	})
	if err != nil {
		return err
	}
	spec, err := loadBpf()
	if err != nil {
		return err
	}
	objs := &bpfObject{}
	if err := spec.LoadAndAssign(objs, nil); err != nil {
		slog.Error("failed to load bpf objects", "error", err)
		return err
	}
	b.obj = objs
	b.reader, err = ringbuf.NewReader(objs.Map)
	if err != nil {
		slog.Error("failed to create ringbuf reader", "error", err)
		return err
	}
	if err := b.initClosedLink(objs); err != nil {
		slog.Error("failed to init close link", "error", err)
		return err
	}
	if err := b.initProbeConnectLink(objs); err != nil {
		slog.Error("failed to init probe connect link", "error", err)
		return err
	}
	return nil
}

func (b *BpfTcpConn) initClosedLink(objs *bpfObject) error {
	linkClose, err := link.Kprobe("tcp_close", objs.Close, nil)
	if err != nil {
		return err
	}
	b.closeLink = linkClose
	return nil
}

func (b *BpfTcpConn) initProbeConnectLink(objs *bpfObject) error {
	linkProbeConnect, err := link.Kprobe("tcp_connect", objs.ProbeConnect, nil)
	if err != nil {
		return err
	}
	b.connectLink = linkProbeConnect
	return nil
}
