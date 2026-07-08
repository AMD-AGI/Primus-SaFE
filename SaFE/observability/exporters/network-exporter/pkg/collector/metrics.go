// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
)

var nodeName = getNodeName()

func getNodeName() string {
	if name := os.Getenv("NODE_NAME"); name != "" {
		return name
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func newNetworkMetricsSet() *networkMetricsSet {
	return &networkMetricsSet{
		tcpFlowEgress: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "network_exporter",
			Name:      "tcp_flow_egress",
			Help:      "TCP flow outbound bytes",
		}, []string{"node", "raddr", "rport", "direction", "type"}),
		tcpFlowIngress: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "network_exporter",
			Name:      "tcp_flow_ingress",
			Help:      "TCP flow inbound bytes",
		}, []string{"node", "raddr", "lport", "direction", "type"}),
		dnsFlow: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "network_exporter",
			Name:        "flow_dns",
			Help:        "DNS flow bytes",
			ConstLabels: prometheus.Labels{"node": nodeName},
		}),
		k8sTCPFlow: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "network_exporter",
			Name:      "k8s_tcp_flow",
			Help:      "K8s TCP flow bytes",
		}, []string{"node", "type"}),
		tcpRtt: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "network_exporter",
				Name:      "tcp_flow_rtt",
				Help:      "TCP flow RTT in microseconds",
				Buckets: []float64{
					50, 100, 200, 300, 500, 700, 1000,
					1300, 1600, 2000, 2500, 3000, 4000,
					5000, 7000, 10000,
				},
			},
			[]string{"node", "raddr", "direction"},
		),
		lastEgressValues:  make(map[string]float64),
		lastIngressValues: make(map[string]float64),
	}
}

type networkMetricsSet struct {
	tcpFlowIngress    *prometheus.CounterVec
	tcpFlowEgress     *prometheus.CounterVec
	dnsFlow           prometheus.Counter
	k8sTCPFlow        *prometheus.CounterVec
	tcpRtt            *prometheus.HistogramVec
	lastEgressValues  map[string]float64
	lastIngressValues map[string]float64
}

func (h *Handler) registerNetworkMetrics(networkMetrics *networkMetricsSet, registry *prometheus.Registry) {
	registry.MustRegister(
		networkMetrics.tcpFlowIngress,
		networkMetrics.tcpFlowEgress,
		networkMetrics.dnsFlow,
		networkMetrics.k8sTCPFlow,
		networkMetrics.tcpRtt,
	)
}
