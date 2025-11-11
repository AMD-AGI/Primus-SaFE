package tcpconn

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/network-exporter/pkg/metrics"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"net"
	"sync"
	"time"
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
		return fmt.Sprintf("Disconnect event: Pid %d Family %d Sip %s Sp %d Dip %s Dp %d", c.Pid, c.Family, net.IP(c.Saddr[:]).String(), c.SPort, net.IP(c.Daddr[:]).String(), c.DPort)
	case AF_INET6:
		return fmt.Sprintf("Disconnect event: Pid %d Family %d Sip %s Sp %d Dip %s Dp %d", c.Pid, c.Family, net.IP(c.SaddrV6[:]).String(), c.SPort, net.IP(c.DaddrV6[:]).String(), c.DPort)
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
	b.connectLink.Close()
	b.closeLink.Close()
	b.reader.Close()
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
				log.Errorf("reading from ringbuf: %v", err)
				continue
			}
			err = binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
			if err != nil {
				log.Errorf("parsing ringbuf record: %v", err)
				continue
			}
			log.Tracef("Received event - Type %s Event %v ", event.GetType(), event)
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
		log.Errorf("failed to load bpf objects: %v", err)
		return err
	}
	b.obj = objs
	b.reader, err = ringbuf.NewReader(objs.Map)
	if err != nil {
		log.Errorf("failed to create ringbuf reader: %v", err)
		return err
	}
	if err := b.initClosedLink(objs); err != nil {
		log.Errorf("failed to init close link: %v", err)
		return err
	}
	if err := b.initProbeConnectLink(objs); err != nil {
		log.Errorf("failed to init probe connect link: %v", err)
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
