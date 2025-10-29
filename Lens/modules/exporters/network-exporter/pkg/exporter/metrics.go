package exporter

import (
	"strconv"
	"time"

	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/model"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func (h *Handler) flushNetworkMetrics() {
	for {
		h.refreshNetworkMetrics()
		time.Sleep(15 * time.Second)
	}
}

func (h *Handler) refreshNetworkMetrics() {
	h.metrics.Reset()
	h.metricsCache.tcpIngressFlow.Range(func(key string, value model.TcpIngressMetricValue) bool {
		h.metrics.tcpFlowIngress.WithLabelValues(value.Raddr, strconv.Itoa(value.Lport), value.Direction, value.Type).Add(value.Value)
		return true
	})
	h.metricsCache.tcpEgressFlow.Range(func(key string, value model.TcpEgressMetricValue) bool {
		h.metrics.tcpFlowEgress.WithLabelValues(value.Raddr, strconv.Itoa(value.Rport), value.Direction, value.Type).Add(value.Value)
		return true
	})
}

func newNetworkMetricsSet() *networkMetricsSet {
	return &networkMetricsSet{
		tcpFlowEgress: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "network",
			Name:      "tcp_flow_egress",
			Help:      "tcp flow outbound",
		}, []string{"raddr", "rport", "direction", "type"}),
		tcpFlowIngress: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "network",
			Name:      "tcp_flow_ingress",
			Help:      "tcp flow inbound",
		}, []string{"lport", "raddr", "direction", "type"}),
		dnsFlow: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "network",
			Name:      "flow_dns",
			Help:      "flow dns",
		}),
		k8sTCPFlow: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "network",
			Name:      "k8s_tcp_flow",
			Help:      "k8s tcp flow",
		}, []string{"type"}),
		tcpRtt: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "primus_lens",
				Subsystem: "network",
				Name:      "tcp_flow_rtt",
				Help:      "tcp flow rtt",
				Buckets: []float64{
					50,
					100,
					200,
					300,
					500,
					700,
					1000,
					1300,
					1600,
					2000,
					2500,
					3000,
					4000,
					5000,
					7000,
					10000,
				},
			},
			[]string{"raddr", "direction"},
		),
	}
}

type networkMetricsSet struct {
	tcpFlowIngress *prometheus.CounterVec
	tcpFlowEgress  *prometheus.CounterVec
	dnsFlow        prometheus.Counter
	k8sTCPFlow     *prometheus.CounterVec
	tcpRtt         *prometheus.HistogramVec
}

func (n *networkMetricsSet) Reset() {
	n.tcpFlowEgress.Reset()
	n.tcpFlowIngress.Reset()
}

func (h *Handler) Gather() ([]*dto.MetricFamily, error) {
	result := []*dto.MetricFamily{}
	defaultGather := prometheus.DefaultGatherer
	metrics, err := defaultGather.Gather()
	if err != nil {
		return nil, err
	}
	result = append(result, metrics...)
	r := prometheus.NewRegistry()
	h.registerNetworkMetrics(h.metrics, r)
	networkMetrics, err := r.Gather()
	if err != nil {
		return nil, err
	}
	result = append(result, networkMetrics...)
	return result, nil

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
