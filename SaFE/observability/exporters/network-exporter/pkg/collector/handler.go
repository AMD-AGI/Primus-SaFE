// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/bpf/tcpconn"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/bpf/tcpflow"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/model"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/process"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/reporter"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/util"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type Handler struct {
	nodeName       string
	tcpConn        *tcpconn.BpfTcpConn
	tcpFlow        *tcpflow.BpfTcpFlow
	localIpAddress map[string]struct{}
	localListen    map[int]struct{}
	ranger         *policyRanger
	tcpTmpCache    *tcpTmpCache
	tcpTmpMu       sync.Mutex // guards tcpTmpCache: eBPF writers vs periodic flush swap+iterate
	metricsCache   *metricsCache
	metrics        *networkMetricsSet
	interval       time.Duration

	// Report pipeline (optional, toggled by config)
	reportCache  *reportCache
	reporter     *reporter.Reporter
	processCache *process.Cache
}

// reportCache aggregates per-PID flow data for periodic reporting to fault-manager.
type reportCache struct {
	mu    sync.Mutex
	flows map[model.ReportFlowKey]model.ReportFlowValue
}

func newTcpTmpCache() *tcpTmpCache {
	return &tcpTmpCache{
		tcpFlowCache: make(map[model.TcpFlowCacheKey]*model.TcpFlowDataValue),
		tcpConnCache: make(map[model.TcpFlowCacheKey]*model.TcpFlowDataValue),
	}
}

type tcpTmpCache struct {
	tcpFlowCache map[model.TcpFlowCacheKey]*model.TcpFlowDataValue
	tcpConnCache map[model.TcpFlowCacheKey]*model.TcpFlowDataValue
}

func newMetricsCache() *metricsCache {
	return &metricsCache{
		tcpEgressFlow:  util.NewCache[model.TcpEgressMetricValue](nil, 10*time.Minute),
		tcpIngressFlow: util.NewCache[model.TcpIngressMetricValue](nil, 10*time.Minute),
		k8sFlow:        util.NewCache[float64](nil, 0),
		dnsFlow:        0,
	}
}

type metricsCache struct {
	tcpEgressFlow  *util.Cache[model.TcpEgressMetricValue]
	tcpIngressFlow *util.Cache[model.TcpIngressMetricValue]
	k8sFlow        *util.Cache[float64]
	dnsFlow        float64
}

func NewHandler(interval int, reporterCfg *reporter.Config) (*Handler, error) {
	h := &Handler{
		localIpAddress: make(map[string]struct{}),
		localListen:    map[int]struct{}{},
		tcpTmpCache:    newTcpTmpCache(),
		metricsCache:   newMetricsCache(),
		metrics:        newNetworkMetricsSet(),
		interval:       time.Duration(interval) * time.Second,
	}

	// Load default policy
	policy := LoadDefaultPolicy()
	h.setPolicy(policy)

	// Initialize report pipeline if configured
	if reporterCfg != nil && reporterCfg.Enabled {
		h.reporter = reporter.New(*reporterCfg)
		if h.reporter != nil {
			h.reportCache = &reportCache{
				flows: make(map[model.ReportFlowKey]model.ReportFlowValue),
			}
			h.processCache = process.NewCache()
			slog.Info("report pipeline enabled",
				"endpoint", reporterCfg.Endpoint,
				"interval", reporterCfg.Interval,
			)
		}
	}

	return h, nil
}

func (h *Handler) setPolicy(policy NetworkPolicy) {
	if h.ranger == nil {
		h.ranger = &policyRanger{}
	}
	h.ranger.loadFromConfig(policy)
}

func (h *Handler) Init(ctx context.Context) error {
	var err error

	// Load local IP addresses
	if err = h.loadLocalIpAddress(); err != nil {
		return err
	}

	// Initialize BPF programs
	h.tcpConn, err = tcpconn.NewBpfTcpConn()
	if err != nil {
		return err
	}
	h.tcpConn.InitChan(409600)

	h.tcpFlow, err = tcpflow.NewBpfTcpFlow()
	if err != nil {
		return err
	}
	h.tcpFlow.InitChan(409600)

	// Start BPF event processing
	h.tcpConn.Start()
	h.tcpFlow.Start()

	// Load listening ports
	if err = h.loadListeningPort(); err != nil {
		slog.Warn("Failed to load listening ports", "error", err)
	}

	return nil
}

func (h *Handler) Start(ctx context.Context) {
	// Start background goroutines
	go h.runLoadListenPortProcess(ctx, 30*time.Second)
	go h.syncTcpConn(ctx)
	go h.syncTcpFlow(ctx)
	go h.doFlushTcpFlow(ctx)
	go h.flushNetworkMetrics()

	// Start report pipeline if enabled
	if h.reporter != nil {
		go h.runReportPipeline(ctx)
	}
}

func (h *Handler) Close() {
	if h.tcpConn != nil {
		h.tcpConn.Close()
	}
	if h.tcpFlow != nil {
		h.tcpFlow.Close()
	}
}

func (h *Handler) runLoadListenPortProcess(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.loadListeningPort(); err != nil {
				slog.Error("load listen port failed", "error", err)
			}
		}
	}
}

func (h *Handler) loadListeningPort() error {
	ports, err := h.getAllListingPort()
	if err != nil {
		return err
	}
	slog.Debug("load listen ports", "count", len(ports))
	listenPort := map[int]struct{}{}
	for _, port := range ports {
		listenPort[port] = struct{}{}
	}
	h.localListen = listenPort
	return nil
}

func (h *Handler) getAllListingPort() ([]int, error) {
	// Try host path first, then fall back to regular path
	tcpPorts, err := util.TcpListenWithCustomPath("/host-proc/net/tcp")
	if err != nil {
		tcpPorts, err = util.TcpListen()
		if err != nil {
			return nil, err
		}
	}

	results := []int{}
	for _, port := range tcpPorts {
		if port.LocalAddr == nil {
			continue
		}
		results = append(results, int(port.LocalAddr.Port))
	}
	return results, nil
}

func (h *Handler) loadLocalIpAddress() error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			switch ip := addr.(type) {
			case *net.IPNet:
				h.localIpAddress[ip.IP.String()] = struct{}{}
			case *net.IPAddr:
				h.localIpAddress[ip.IP.String()] = struct{}{}
			}
		}
	}

	slog.Debug("loaded local IP addresses", "count", len(h.localIpAddress))
	return nil
}

func (h *Handler) getDirection(saddr, daddr string, sport, dport uint16) (localAddr, remoteAddr string, localPort, remotePort int, typ int, direction string) {
	saddrLocal := false
	sPortListen := false
	daddrLocal := false
	dPortListen := false

	if _, ok := h.localIpAddress[saddr]; ok {
		saddrLocal = true
		if _, ok := h.localListen[int(sport)]; ok {
			sPortListen = true
		}
	}
	if _, ok := h.localIpAddress[daddr]; ok {
		daddrLocal = true
		if _, ok := h.localListen[int(dport)]; ok {
			dPortListen = true
		}
	}

	if saddrLocal && daddrLocal && !sPortListen && !dPortListen {
		typ = -1
		return
	}

	if sPortListen || dPortListen {
		typ = model.FlowTypeIngress
		if sPortListen {
			direction = model.DirectionOutbound
			localAddr = saddr
			remoteAddr = daddr
			localPort = int(sport)
			remotePort = int(dport)
		} else {
			direction = model.DirectionInbound
			localAddr = daddr
			remoteAddr = saddr
			localPort = int(dport)
			remotePort = int(sport)
		}
	} else {
		typ = model.FlowTypeEgress
		if saddrLocal {
			direction = model.DirectionOutbound
			localAddr = saddr
			remoteAddr = daddr
			localPort = int(sport)
			remotePort = int(dport)
		} else {
			direction = model.DirectionInbound
			localAddr = daddr
			remoteAddr = saddr
			localPort = int(dport)
			remotePort = int(sport)
		}
	}
	return
}

// Gather implements prometheus.Gatherer
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

func (h *Handler) flushNetworkMetrics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		h.refreshNetworkMetrics()
	}
}

func (h *Handler) refreshNetworkMetrics() {
	h.metricsCache.tcpIngressFlow.Range(func(key string, value model.TcpIngressMetricValue) bool {
		last := h.metrics.lastIngressValues[key]
		delta := value.Value - last
		if delta > 0 {
			h.metrics.tcpFlowIngress.WithLabelValues(nodeName, value.Raddr, strconv.Itoa(value.Lport), value.Direction, value.Type).Add(delta)
			h.metrics.lastIngressValues[key] = value.Value
		}
		return true
	})
	h.metricsCache.tcpEgressFlow.Range(func(key string, value model.TcpEgressMetricValue) bool {
		last := h.metrics.lastEgressValues[key]
		delta := value.Value - last
		if delta > 0 {
			h.metrics.tcpFlowEgress.WithLabelValues(nodeName, value.Raddr, strconv.Itoa(value.Rport), value.Direction, value.Type).Add(delta)
			h.metrics.lastEgressValues[key] = value.Value
		}
		return true
	})
}

// runReportPipeline periodically flushes the report cache and sends data to fault-manager.
func (h *Handler) runReportPipeline(ctx context.Context) {
	ticker := time.NewTicker(h.reporter.Interval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.flushAndReport(ctx)
		}
	}
}

// flushAndReport atomically swaps the report cache, resolves PID info, and sends the report.
func (h *Handler) flushAndReport(ctx context.Context) {
	// Swap out the current report cache
	h.reportCache.mu.Lock()
	snapshot := h.reportCache.flows
	h.reportCache.flows = make(map[model.ReportFlowKey]model.ReportFlowValue)
	h.reportCache.mu.Unlock()

	if len(snapshot) == 0 {
		return
	}

	// Build report entries
	entries := make([]model.ReportFlowEntry, 0, len(snapshot))
	for key, val := range snapshot {
		info := h.processCache.Lookup(key.Pid)
		entries = append(entries, model.ReportFlowEntry{
			ProcessName:  info.Comm,
			Pid:          info.Pid,
			NsPid:        info.NsPid,
			RemoteAddr:   key.Raddr,
			RemotePort:   key.Rport,
			EgressBytes:  val.EgressBytes,
			IngressBytes: val.IngressBytes,
		})
	}

	payload := &model.ReportPayload{
		NodeName:  nodeName,
		Timestamp: time.Now().Unix(),
		Flows:     entries,
	}

	if err := h.reporter.Send(ctx, payload); err != nil {
		slog.Error("failed to report flows to fault-manager", "error", err)
	}

	// Periodically clean up stale PID cache entries
	h.processCache.Cleanup()
}
