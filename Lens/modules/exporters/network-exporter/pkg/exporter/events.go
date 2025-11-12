package exporter

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/stringUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/network-exporter/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/network-exporter/pkg/util"
)

func (n *Handler) doFlushTcpFlow(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.flushTmpCache()
		}
	}
}

func (n *Handler) flushTmpCache() {
	cache := n.tcpTmpCache
	n.tcpTmpCache = newTcpTmpCache()

	n.lg.Debugf("flush tmp tcp flow cache %d", len(cache.tcpFlowCache))
	for key, value := range cache.tcpFlowCache {
		n.consumeForSingleFlow(key, value.FlowData)
	}
	n.lg.Debugf("flush tmp tcp conn cache %d", len(cache.tcpConnCache))
	for key, value := range cache.tcpConnCache {
		n.consumeForSingleRtt(key, *value)
	}
}

func (n *Handler) consumeForSingleRtt(key model.TcpFlowCacheKey, value model.TcpFlowDataValue) {
	_, remoteAddr, _, _, t, direction := n.getDirection(key.SAddr, key.Daddr, uint16(key.Sport), uint16(key.Dport))
	if t == -1 {
		return
	}
	// Get the type of remote address
	typ, _, err := n.ranger.match(remoteAddr)
	if err != nil {
		n.lg.Errorf("match remote addr %s failed %v", remoteAddr, err)
		typ = IPSourceError
	}
	if stringUtil.In(typ, []string{IPSourceK8sPod, IPSourceK8sSvc, IPSourceDNS, IPSourceLocalhost, IPSourceDocker}) {
		return
	}
	n.metrics.tcpRtt.WithLabelValues(remoteAddr, direction).Observe(float64(value.RttTotal) / float64(value.PktCount))
}

func (n *Handler) consumeForSingleFlow(key model.TcpFlowCacheKey, dataLen uint64) {
	// metrics
	_, remoteAddr, localPort, remotePort, connType, direction := n.getDirection(key.SAddr, key.Daddr, uint16(key.Sport), uint16(key.Dport))
	if connType == -1 {
		return
	}
	// Get the type of remote address
	typ, _, err := n.ranger.match(remoteAddr)
	if err != nil {
		n.lg.Errorf("match remote addr %s failed %v", remoteAddr, err)
		typ = IPSourceError
	}
	if typ == IPSourceDNS {
		n.metrics.dnsFlow.Add(float64(dataLen))
		return
	}
	if stringUtil.In(typ, []string{IPSourceK8sPod, IPSourceK8sSvc}) {
		n.metrics.k8sTCPFlow.WithLabelValues(typ).Add(float64(dataLen))
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
		n.metricsCache.tcpIngressFlow.Upsert(ingressKey.String(), func(old util.Item[model.TcpIngressMetricValue]) util.Item[model.TcpIngressMetricValue] {
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
		n.metricsCache.tcpEgressFlow.Upsert(egressKey.String(), func(old util.Item[model.TcpEgressMetricValue]) util.Item[model.TcpEgressMetricValue] {
			if old.Value.Raddr == "" {
				old.Value = egressKey
			} else {
				old.Value.Value += float64(dataLen)
			}
			return old
		})
	}
}
