// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/model"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/util"
)

func (h *Handler) doFlushTcpFlow(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.flushTmpCache()
		}
	}
}

func (h *Handler) flushTmpCache() {
	// Swap the cache under lock so in-flight eBPF writers can't mutate the map
	// we are about to iterate. After the swap `cache` is exclusively ours, so
	// the range below needs no lock.
	h.tcpTmpMu.Lock()
	cache := h.tcpTmpCache
	h.tcpTmpCache = newTcpTmpCache()
	h.tcpTmpMu.Unlock()

	slog.Debug("flush tmp tcp flow cache", "count", len(cache.tcpFlowCache))
	for key, value := range cache.tcpFlowCache {
		h.consumeForSingleFlow(key, value.FlowData)
		// RTT data is captured in tcpFlowCache (not tcpConnCache)
		h.consumeForSingleRtt(key, *value)
	}
	slog.Debug("flush tmp tcp conn cache", "count", len(cache.tcpConnCache))
}

func (h *Handler) consumeForSingleRtt(key model.TcpFlowCacheKey, value model.TcpFlowDataValue) {
	_, remoteAddr, _, _, t, direction := h.getDirection(key.SAddr, key.Daddr, uint16(key.Sport), uint16(key.Dport))
	if t == -1 {
		return
	}

	// Get the type of remote address
	typ, _, err := h.ranger.match(remoteAddr)
	if err != nil {
		slog.Error("match remote addr failed", "addr", remoteAddr, "error", err)
		typ = IPSourceError
	}

	if util.In(typ, []string{IPSourceK8sPod, IPSourceK8sSvc, IPSourceDNS, IPSourceLocalhost, IPSourceDocker}) {
		return
	}

	if value.PktCount > 0 {
		h.metrics.tcpRtt.WithLabelValues(nodeName, remoteAddr, direction).Observe(float64(value.RttTotal) / float64(value.PktCount))
	}
}

func (h *Handler) consumeForSingleFlow(key model.TcpFlowCacheKey, dataLen uint64) {
	_, remoteAddr, localPort, remotePort, connType, direction := h.getDirection(key.SAddr, key.Daddr, uint16(key.Sport), uint16(key.Dport))
	if connType == -1 {
		return
	}

	// Get the type of remote address
	typ, _, err := h.ranger.match(remoteAddr)
	if err != nil {
		slog.Error("match remote addr failed", "addr", remoteAddr, "error", err)
		typ = IPSourceError
	}

	if typ == IPSourceDNS {
		h.metrics.dnsFlow.Add(float64(dataLen))
		return
	}

	if util.In(typ, []string{IPSourceK8sPod, IPSourceK8sSvc}) {
		h.metrics.k8sTCPFlow.WithLabelValues(nodeName, typ).Add(float64(dataLen))
		return
	}

	switch connType {
	case model.FlowTypeIngress:
		ingressKey := model.TcpIngressMetricValue{
			Lport:     localPort,
			Raddr:     remoteAddr,
			Direction: direction,
			Type:      typ,
			Value:     float64(dataLen),
		}
		h.metricsCache.tcpIngressFlow.Upsert(ingressKey.String(), func(old util.Item[model.TcpIngressMetricValue]) util.Item[model.TcpIngressMetricValue] {
			if old.Value.Raddr == "" {
				old.Value = ingressKey
			} else {
				old.Value.Value += float64(dataLen)
			}
			return old
		})
	case model.FlowTypeEgress:
		egressKey := model.TcpEgressMetricValue{
			Raddr:     remoteAddr,
			Rport:     remotePort,
			Direction: direction,
			Type:      typ,
			Value:     float64(dataLen),
		}
		h.metricsCache.tcpEgressFlow.Upsert(egressKey.String(), func(old util.Item[model.TcpEgressMetricValue]) util.Item[model.TcpEgressMetricValue] {
			if old.Value.Raddr == "" {
				old.Value = egressKey
			} else {
				old.Value.Value += float64(dataLen)
			}
			return old
		})
	}
}
