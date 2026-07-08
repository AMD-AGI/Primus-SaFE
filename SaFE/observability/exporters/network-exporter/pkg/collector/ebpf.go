// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/bpf/tcpconn"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/bpf/tcpflow"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/model"
)

func (h *Handler) syncTcpConn(ctx context.Context) {
	for {
		select {
		case conn := <-h.tcpConn.Read():
			h.consumeTcpConn(conn)
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler) syncTcpFlow(ctx context.Context) {
	for {
		select {
		case flow := <-h.tcpFlow.Read():
			h.consumeTcpFlow(flow)
		case <-ctx.Done():
			return
		}
	}
}

// consumeTcpFlow processes TCP flow events from eBPF
func (h *Handler) consumeTcpFlow(e tcpflow.TcpFlow) {
	key := model.TcpFlowCacheKey{
		SAddr:  e.GetSaddr(),
		Daddr:  e.GetDaddr(),
		Sport:  int(e.SPort),
		Dport:  int(e.DPort),
		Family: int(e.Family),
	}
	h.tcpTmpMu.Lock()
	cacheMap := h.tcpTmpCache.tcpFlowCache
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
	h.tcpTmpMu.Unlock()

	// Populate report cache (if enabled)
	if h.reportCache != nil {
		_, remoteAddr, _, remotePort, connType, _ := h.getDirection(key.SAddr, key.Daddr, e.SPort, e.DPort)
		if connType == -1 {
			return
		}

		reportKey := model.ReportFlowKey{
			Pid:       e.Pid,
			Raddr:     remoteAddr,
			Rport:     remotePort,
			Direction: model.GetDirectionName(connType),
		}

		h.reportCache.mu.Lock()
		if v, ok := h.reportCache.flows[reportKey]; ok {
			if connType == model.FlowTypeEgress {
				v.EgressBytes += uint64(e.DataLen)
			} else {
				v.IngressBytes += uint64(e.DataLen)
			}
			h.reportCache.flows[reportKey] = v
		} else {
			val := model.ReportFlowValue{}
			if connType == model.FlowTypeEgress {
				val.EgressBytes = uint64(e.DataLen)
			} else {
				val.IngressBytes = uint64(e.DataLen)
			}
			h.reportCache.flows[reportKey] = val
		}
		h.reportCache.mu.Unlock()
	}
}

func (h *Handler) consumeTcpConn(e tcpconn.ConnEvent) {
	key := model.TcpFlowCacheKey{
		SAddr:  e.GetSip(),
		Daddr:  e.GetDip(),
		Sport:  int(e.SPort),
		Dport:  int(e.DPort),
		Family: int(e.Family),
	}
	addValue := 0
	switch e.GetType() {
	case tcpconn.EventTypeProbeConnect:
		addValue = 1
	case tcpconn.EventTypeProbeClose:
		addValue = -1
	}

	h.tcpTmpMu.Lock()
	cacheMap := h.tcpTmpCache.tcpConnCache
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
	h.tcpTmpMu.Unlock()
}
