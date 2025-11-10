package exporter

import (
	"context"

	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/bpf/tcpconn"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/bpf/tcpflow"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/model"
)

func (n *Handler) syncTcpConn(ctx context.Context) {
	for {
		select {
		case conn := <-n.tcpConn.Read():
			n.consumeTcpConn(conn)
		case <-ctx.Done():
			return
		}
	}
}

func (n *Handler) syncTcpFlow(ctx context.Context) {
	for {
		select {
		case flow := <-n.tcpFlow.Read():
			n.consumeTcpFlow(flow)
		case <-ctx.Done():
			return
		}
	}
}

// Network traffic is exported as metrics, not recorded at packet granularity for now
func (n *Handler) consumeTcpFlow(e tcpflow.TcpFlow) {
	key := model.TcpFlowCacheKey{
		SAddr:  e.GetSaddr(),
		Daddr:  e.GetDaddr(),
		Sport:  int(e.SPort),
		Dport:  int(e.DPort),
		Family: int(e.Family),
	}
	cacheMap := n.tcpTmpCache.tcpFlowCache

	if _, ok := cacheMap[key]; ok {
		cacheMap[key].FlowData += uint64(e.DataLen)
		cacheMap[key].RttTotal += uint64(e.Srtt)
		cacheMap[key].PktCount += 1
	} else {
		cacheMap[key] = &model.TcpFlowDataValue{
			FlowData: uint64(e.DataLen),
			RttTotal: uint64(e.Srtt),
			PktCount: 1,
		}
	}
}

func (n *Handler) consumeTcpConn(e tcpconn.ConnEvent) {
	key := model.TcpFlowCacheKey{
		SAddr:  e.GetSip(),
		Daddr:  e.GetDip(),
		Sport:  int(e.SPort),
		Dport:  int(e.DPort),
		Family: int(e.Family),
	}
	cacheMap := n.tcpTmpCache.tcpConnCache
	addValue := 0
	switch e.GetType() {
	case tcpconn.EventTypeProbeConnect:
		addValue = 1
	case tcpconn.EventTypeProbeClose:
		addValue = -1
	}

	if _, ok := cacheMap[key]; ok {
		cacheMap[key].ConnCount += uint64(addValue)
	} else {
		if addValue < 0 {
			addValue = 0
		}
		cacheMap[key] = &model.TcpFlowDataValue{
			ConnCount: uint64(addValue),
		}
	}
}
