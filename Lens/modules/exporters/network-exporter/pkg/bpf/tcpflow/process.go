package tcpflow

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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 -cc clang -cflags $CFLAGS bpf ./bpf/tcpflow.bpf.c

const (
	AF_INET  = 2  // IPv4
	AF_INET6 = 10 // IPv6
	AF_UNIX  = 1  // Unix Domain Socket
)

var (
	onceInit sync.Once
)

type TcpFlow struct {
	Saddr   [28]byte `ebpf:"saddr"`  // offset: 0
	Daddr   [28]byte `ebpf:"daddr"`  // offset: 32
	SPort   uint16   `ebpf:"sport"`  // offset: 64
	DPort   uint16   `ebpf:"dport"`  // offset: 66
	Family  uint16   `ebpf:"family"` // offset: 68
	Reason  uint16   `ebpf:"reason"`
	DataLen uint32   `ebpf:"data_len"` // offset: 76
	Srtt    uint32   `ebpf:"srtt"`
	Pid     uint32   `ebpf:"pid"`
}

func (c TcpFlow) String() string {
	return fmt.Sprintf("Family %d Saddr %s Daddr %s SPort %d DPort %d DataLen %d Reason %d Srtt %d Pid %d", c.Family, c.GetSaddr(), c.GetDaddr(), c.SPort, c.DPort, c.DataLen, c.Reason, c.Srtt, c.Pid)
}

func (c TcpFlow) GetSaddr() string {
	switch c.Family {
	case AF_INET:
		return net.IP(c.Saddr[4:8]).String()
	case AF_INET6:
		return net.IP(c.Saddr[8:24]).String()
	}
	return string(c.Saddr[:])
}

func (c TcpFlow) GetDaddr() string {
	switch c.Family {
	case AF_INET:
		return net.IP(c.Daddr[4:8]).String()
	case AF_INET6:
		return net.IP(c.Daddr[8:24]).String()
	}
	return string(c.Daddr[:])
}

type bpfObject struct {
	Probe *ebpf.Program `ebpf:"trace_tcp_probe"`
	Map   *ebpf.Map     `ebpf:"events"`
}

type BpfTcpFlow struct {
	obj        *bpfObject
	probe      link.Link
	cancelCtx  context.Context
	cancelFunc context.CancelFunc
	eventChan  chan TcpFlow
	reader     *ringbuf.Reader
}

func NewBpfTcpFlow() (*BpfTcpFlow, error) {
	bpfTcpFlow := &BpfTcpFlow{}
	if err := bpfTcpFlow.init(); err != nil {
		return nil, err
	}
	return bpfTcpFlow, nil
}

func (b *BpfTcpFlow) InitChan(size int) {
	b.eventChan = make(chan TcpFlow, size)
}

func (b *BpfTcpFlow) Read() <-chan TcpFlow {
	return b.eventChan
}

func (b *BpfTcpFlow) Start() {
	go b.Sync()
}

func (b *BpfTcpFlow) Sync() error {
	b.doSyncEvent()
	return nil
}

func (b *BpfTcpFlow) doSyncEvent() {
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
				log.Errorf("failed to read from ringbuf: %v", err)
				continue
			}
			var event TcpFlow
			err = binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
			if err != nil {
				log.Errorf("parsing ringbuf record: %v", err)
				continue
			}
			log.Tracef("Received event -  Event %v ", event)
			metrics.BpfEventRecv.Inc("tcpflow")
			if b.eventChan != nil {
				select {
				case b.eventChan <- event:
				default:
					metrics.BpfEventChanDrop.Inc("tcpflow")
				}
			} else {
				log.Debugf("Received event -  Event %v ", event)
			}
		}
	}
}

func (b *BpfTcpFlow) init() error {
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
	if err := b.initProbeLink(objs); err != nil {
		log.Errorf("failed to init probe link: %v", err)
		return err
	}
	return nil
}

func (b *BpfTcpFlow) initProbeLink(objs *bpfObject) error {
	linkProbe, err := link.Tracepoint("tcp", "tcp_probe", objs.Probe, nil)
	if err != nil {
		return err
	}
	b.probe = linkProbe
	return nil
}
