// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.
// Build trigger: dual registry support

package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/bpf/rdmactl"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/bpf/rdmaflow"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/collector"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/discovery"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	port := flag.String("port", getEnvOrDefault("RDMA_EXPORTER_PORT", "9401"), "metrics port")
	interval := flag.Int("interval", getEnvOrDefaultInt("RDMA_EXPORTER_INTERVAL", 5), "collection interval in seconds")
	logLevel := flag.String("log-level", getEnvOrDefault("RDMA_EXPORTER_LOG_LEVEL", "info"), "log level")
	enableEBPF := flag.Bool("enable-ebpf", getEnvOrDefaultBool("RDMA_EXPORTER_ENABLE_EBPF", false), "enable eBPF probes (kprobe + uprobe)")
	providerLib := flag.String("provider-lib", getEnvOrDefault("RDMA_EXPORTER_PROVIDER_LIB", "libbnxt_re"), "RDMA provider library name prefix to search in /proc/*/maps")
	providerFunc := flag.String("provider-func", getEnvOrDefault("RDMA_EXPORTER_PROVIDER_FUNC", "bnxt_re_post_send"), "uprobe target function name in provider library")
	statePath := flag.String("state-path", getEnvOrDefault("RDMA_EXPORTER_STATE_PATH", "/var/lib/rdma-exporter/state.json"), "path for connection state persistence")
	flag.Parse()

	setupLogging(*logLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeName := collector.NodeName()

	c := collector.NewWithOptions(*interval, true, true)

	slog.Info("RDMA Exporter starting",
		"port", *port,
		"interval", *interval,
		"enable_ebpf", *enableEBPF,
		"use_nsenter", c.GetExecutor().IsNsenterEnabled(),
	)

	if err := collector.RunPreflightChecks(c.GetExecutor()); err != nil {
		slog.Warn("Preflight checks had warnings", "error", err)
	}

	go c.Start(ctx)

	var ctlHandler *rdmactl.Handler
	var flowHandler *rdmaflow.Handler

	var commMetrics *collector.CommMetrics

	if *enableEBPF {
		commMetrics = collector.NewCommMetrics(nodeName)

		var err error
		ctlHandler, err = rdmactl.New(nodeName)
		if err != nil {
			slog.Error("Failed to initialize kprobe handler", "error", err)
		} else {
			ctlHandler.SetOnConnection(func(pid, qpn uint32, info model.ConnectionInfo) {
				commMetrics.UpdateConnection(pid, qpn, info)
			})
			ctlHandler.SetExecutor(c.GetExecutor())

			// Load persisted state and replay to metrics layer
			if err := ctlHandler.LoadState(*statePath); err != nil {
				slog.Error("load persisted state", "error", err)
			}
			ctlHandler.ReplayConnections()

			// Start kprobe + timers
			ctlHandler.Start(ctx, nodeName)
			slog.Info("kprobe handler started (ib_modify_qp)")

			// Scan existing QPs and replay any new entries
			ctlHandler.ScanExistingQPs()
			ctlHandler.ReplayConnections()
		}

		flowHandler, err = rdmaflow.New(nodeName)
		if err != nil {
			slog.Error("Failed to initialize uprobe handler", "error", err)
		} else {
			flowHandler.SetOnSendEvent(func(pid, qpn, opcode uint32, bytes uint64) {
				commMetrics.OnSendEvent(pid, qpn, opcode, bytes)
			})
			flowHandler.Start(ctx)
			slog.Info("uprobe handler started (bnxt_re_post_send)")

			providerLibs := parseProviderList(*providerLib)
			providerFuncs := parseProviderList(*providerFunc)
			disc := discovery.NewMultiDiscoverer("", providerLibs, providerFuncs)
			go runUprobeDiscoveryLoop(ctx, c, flowHandler, disc, *interval)
		}

		go runQPIndexLoop(ctx, c, commMetrics, flowHandler, *interval)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler)

	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("Received shutdown signal", "signal", sig)
		cancel()

		if ctlHandler != nil {
			if err := ctlHandler.SaveState(); err != nil {
				slog.Error("save state on shutdown", "error", err)
			}
			ctlHandler.Close()
		}
		if flowHandler != nil {
			flowHandler.Close()
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("Server shutdown error", "error", err)
		}
	}()

	slog.Info("Starting HTTP server", "addr", server.Addr)
	ln, listenErr := listenWithRetry(ctx, server.Addr, 30, 2*time.Second)
	if listenErr != nil {
		if ctx.Err() != nil {
			slog.Info("port retry interrupted by shutdown")
			return
		}
		slog.Error("Failed to bind port after retries", "error", listenErr)
		os.Exit(1)
	}
	if err := server.Serve(ln); err != http.ErrServerClosed {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}

	slog.Info("RDMA Exporter stopped")
}

// runUprobeDiscoveryLoop periodically discovers RDMA processes and attaches/detaches uprobes.
func runUprobeDiscoveryLoop(ctx context.Context, c *collector.Collector, flow uprobeManager, disc processDiscoverer, interval int) {
	discoveryInterval := 30
	if interval > discoveryInterval {
		discoveryInterval = interval
	}
	ticker := time.NewTicker(time.Duration(discoveryInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			discoverAndAttach(&collectorQPLister{c}, flow, disc)
		}
	}
}

func runQPIndexLoop(ctx context.Context, c *collector.Collector, cm *collector.CommMetrics, flow *rdmaflow.Handler, interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			out, err := c.GetExecutor().Execute("rdma", "res", "show", "qp", "-j")
			if err != nil {
				continue
			}
			var qps []model.RDMAQP
			if err := json.Unmarshal(out, &qps); err != nil {
				continue
			}
			cm.UpdateQPIndex(qps)

			activeFlowKeys := make(map[rdmaflow.FlowKey]struct{})
			activeConnKeys := make(map[collector.ConnKey]struct{})
			for _, qp := range qps {
				if qp.Type == "GSI" || qp.PID == 0 {
					continue
				}
				activeFlowKeys[rdmaflow.FlowKey{PID: uint32(qp.PID), QPN: uint32(qp.LQPN)}] = struct{}{}
				activeConnKeys[collector.ConnKey{PID: uint32(qp.PID), QPN: uint32(qp.LQPN)}] = struct{}{}
			}
			if flow != nil {
				flow.CleanInactiveQPs(activeFlowKeys)
			}
			cm.CleanInactiveCounters(activeConnKeys)
		}
	}
}

func parseProviderList(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// qpLister executes rdma res show qp and returns raw JSON.
type qpLister interface {
	ListQPs() ([]model.RDMAQP, error)
}

// uprobeManager manages uprobe attach/detach lifecycle.
type uprobeManager interface {
	AttachTo(pid int, libPath string, symbolName string) error
	AttachedPIDs() map[int]struct{}
	DetachPID(pid int)
}

// processDiscoverer finds RDMA processes for given PIDs.
type processDiscoverer interface {
	FindRDMAProcesses(pids []int) []discovery.RDMAProcess
}

// collectorQPLister adapts *collector.Collector to qpLister.
type collectorQPLister struct {
	c *collector.Collector
}

func (l *collectorQPLister) ListQPs() ([]model.RDMAQP, error) {
	out, err := l.c.GetExecutor().Execute("rdma", "res", "show", "qp", "-j")
	if err != nil {
		return nil, err
	}
	var qps []model.RDMAQP
	if err := json.Unmarshal(out, &qps); err != nil {
		return nil, err
	}
	return qps, nil
}

func discoverAndAttach(lister qpLister, flow uprobeManager, disc processDiscoverer) {
	qps, err := lister.ListQPs()
	if err != nil {
		slog.Debug("uprobe discovery: list qps failed", "error", err)
		return
	}

	activePIDs := make(map[int]struct{})
	for _, qp := range qps {
		if qp.Type == "GSI" || qp.PID == 0 {
			continue
		}
		activePIDs[qp.PID] = struct{}{}
	}

	if len(activePIDs) > 0 {
		pids := make([]int, 0, len(activePIDs))
		for pid := range activePIDs {
			pids = append(pids, pid)
		}
		procs := disc.FindRDMAProcesses(pids)
		for _, p := range procs {
			if err := flow.AttachTo(p.PID, p.LibPath, p.PostSendSym); err != nil {
				slog.Warn("uprobe attach failed", "pid", p.PID, "error", err)
			}
		}
	}

	attached := flow.AttachedPIDs()
	for pid := range attached {
		if _, ok := activePIDs[pid]; !ok {
			flow.DetachPID(pid)
		}
	}
}

func listenWithRetry(ctx context.Context, addr string, maxRetries int, retryInterval time.Duration) (net.Listener, error) {
	var ln net.Listener
	var err error
	for i := 0; i < maxRetries; i++ {
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		slog.Warn("port not available, retrying...", "addr", addr, "attempt", i+1, "error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryInterval):
		}
	}
	return nil, err
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

func setupLogging(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(handler))
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		v, err := strconv.Atoi(value)
		if err == nil {
			return v
		}
	}
	return defaultValue
}

func getEnvOrDefaultBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		switch value {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		}
	}
	return defaultValue
}
