// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/gpu-exporter/pkg/model"
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

// Per-GPU metrics carry an `address` (PCIe BDF) label in addition to `gpu_id`.
// `gpu_id` is retained during the compat window; `address` is the stable
// physical anchor used by the telemetry-gateway join (ADR 0003 D3).
var (
	gpuUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_utilization",
			Help: "GPU utilization percentage",
		},
		[]string{"node", "gpu_id", "address"},
	)
	gpuSocketPower = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_socket_power_watts",
			Help: "GPU socket power in watts",
		},
		[]string{"node", "gpu_id", "address"},
	)
	gpuPCIEBandwidth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_pcie_bandwidth_mbs",
			Help: "GPU PCIe bandwidth in MB/s",
		},
		[]string{"node", "gpu_id", "address"},
	)
	gpuTempJunction = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_temperature_junction_celsius",
			Help: "GPU junction temperature in Celsius",
		},
		[]string{"node", "gpu_id", "address"},
	)
	gpuTempMemory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_temperature_memory_celsius",
			Help: "GPU memory temperature in Celsius",
		},
		[]string{"node", "gpu_id", "address"},
	)
	gpuMemoryUsed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_memory_used_percent",
			Help: "GPU memory usage percentage",
		},
		[]string{"node", "gpu_id", "address"},
	)
	gpuDriverVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_driver_info",
			Help: "GPU driver information (value is always 1)",
		},
		[]string{"node", "driver_version"},
	)
	// gpuDeviceInfo surfaces stable per-GPU identity (address/serial/model)
	// as an info series (value always 1).
	gpuDeviceInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_device_info",
			Help: "GPU device identity (value is always 1)",
		},
		[]string{"node", "gpu_id", "address", "serial", "model"},
	)
	gpuXGMIReadKBs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_xgmi_read_kbs",
			Help: "GPU XGMI link read bandwidth in KB/s",
		},
		[]string{"node", "gpu_id", "address", "link"},
	)
	gpuXGMIWriteKBs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gpu_xgmi_write_kbs",
			Help: "GPU XGMI link write bandwidth in KB/s",
		},
		[]string{"node", "gpu_id", "address", "link"},
	)
)

func init() {
	prometheus.MustRegister(gpuUtilization)
	prometheus.MustRegister(gpuSocketPower)
	prometheus.MustRegister(gpuPCIEBandwidth)
	prometheus.MustRegister(gpuTempJunction)
	prometheus.MustRegister(gpuTempMemory)
	prometheus.MustRegister(gpuMemoryUsed)
	prometheus.MustRegister(gpuDriverVersion)
	prometheus.MustRegister(gpuDeviceInfo)
	prometheus.MustRegister(gpuXGMIReadKBs)
	prometheus.MustRegister(gpuXGMIWriteKBs)
}

// Collector manages GPU metrics collection
type Collector struct {
	interval      int
	executor      *CommandExecutor
	mu            sync.RWMutex
	cardMetrics   []model.CardMetrics
	gpuMetrics    []model.GPUMetricsInfo
	driverVersion string
	// gpuAddr maps an amd-smi GPU index to its PCIe BDF (address), refreshed
	// from `amd-smi static` on a slow cadence.
	gpuAddr map[int]string
}

// New creates a new Collector
func New(interval int) *Collector {
	return &Collector{
		interval: interval,
		executor: NewCommandExecutor(),
		gpuAddr:  make(map[int]string),
	}
}

// GetExecutor returns the command executor
func (c *Collector) GetExecutor() *CommandExecutor {
	return c.executor
}

// Start begins the metrics collection loop
func (c *Collector) Start(ctx context.Context) {
	// Initial collection
	c.collectStaticInfo()
	c.collect()

	// Start periodic collection
	ticker := time.NewTicker(time.Duration(c.interval) * time.Second)
	defer ticker.Stop()

	// Static identity / driver info refresh (less frequent)
	staticTicker := time.NewTicker(60 * time.Second)
	defer staticTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Collector stopping")
			return
		case <-ticker.C:
			c.collect()
		case <-staticTicker.C:
			c.collectStaticInfo()
		}
	}
}

// collectStaticInfo refreshes the gpu_id -> address (BDF) map from
// `amd-smi static`, publishes the gpu_device_info identity series, and the
// driver version (sourced from amd-smi, replacing the retired rocm-smi path).
func (c *Collector) collectStaticInfo() {
	infos, err := GetGPUStaticInfo(c.executor)
	if err != nil {
		slog.Debug("amd-smi static not available, address label will be empty", "error", err)
		return
	}

	addr := make(map[int]string, len(infos))
	driverVersion := ""
	for _, g := range infos {
		addr[g.GPU] = g.Bus.BDF
		if driverVersion == "" {
			driverVersion = g.Driver.Version
		}
		gpuDeviceInfo.WithLabelValues(
			nodeName,
			fmt.Sprintf("%d", g.GPU),
			g.Bus.BDF,
			g.Asic.AsicSerial,
			g.Asic.MarketName,
		).Set(1)
	}

	c.mu.Lock()
	c.gpuAddr = addr
	c.mu.Unlock()

	c.publishDriverVersion(driverVersion)
	slog.Debug("GPU static info refreshed", "count", len(infos))
}

// addrFor returns the cached PCIe BDF for a GPU index ("" when unknown).
func (c *Collector) addrFor(gpuID int) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.gpuAddr[gpuID]
}

// publishDriverVersion updates the gpu_driver_info series, clearing the old
// label value when the version changes.
func (c *Collector) publishDriverVersion(version string) {
	if version == "" {
		return
	}
	c.mu.Lock()
	if c.driverVersion != version {
		if c.driverVersion != "" {
			gpuDriverVersion.DeleteLabelValues(nodeName, c.driverVersion)
		}
		c.driverVersion = version
	}
	c.mu.Unlock()

	gpuDriverVersion.WithLabelValues(nodeName, version).Set(1)
	slog.Debug("Driver version updated", "version", version)
}

func (c *Collector) collect() {
	// All runtime metrics now come from a single `amd-smi metric` call: card
	// metrics (utilization, temperature, memory, power) plus PCIe and XGMI.
	gpuMetrics, err := GetGPUMetrics(c.executor)
	if err != nil {
		slog.Error("Failed to get GPU metrics", "error", err)
		return
	}

	cardMetrics := make([]model.CardMetrics, 0, len(gpuMetrics))
	for _, p := range gpuMetrics {
		gpuID := fmt.Sprintf("%d", p.GPU)
		addr := c.addrFor(p.GPU)

		cm := cardMetricsFromGPUMetrics(p)
		cardMetrics = append(cardMetrics, cm)

		gpuUtilization.WithLabelValues(nodeName, gpuID, addr).Set(cm.GPUUsePercent)
		gpuTempJunction.WithLabelValues(nodeName, gpuID, addr).Set(cm.TemperatureJunction)
		gpuTempMemory.WithLabelValues(nodeName, gpuID, addr).Set(cm.TemperatureMemory)
		gpuMemoryUsed.WithLabelValues(nodeName, gpuID, addr).Set(cm.GPUMemoryAllocatedPercent)
		gpuSocketPower.WithLabelValues(nodeName, gpuID, addr).Set(cm.SocketGraphicsPowerWatts)

		gpuPCIEBandwidth.WithLabelValues(nodeName, gpuID, addr).Set(p.PCIE.Bandwidth.Value)
		for _, link := range p.XGMILink {
			gpuXGMIReadKBs.WithLabelValues(nodeName, gpuID, addr, link.Link).Set(link.Read.Value)
			gpuXGMIWriteKBs.WithLabelValues(nodeName, gpuID, addr, link.Link).Set(link.Write.Value)
		}
	}

	c.mu.Lock()
	c.cardMetrics = cardMetrics
	c.gpuMetrics = gpuMetrics
	c.mu.Unlock()

	slog.Debug("Metrics collection completed", "gpu_metrics_count", len(gpuMetrics))
}

// GetCardMetricsSnapshot returns a copy of the current card metrics
func (c *Collector) GetCardMetricsSnapshot() []model.CardMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]model.CardMetrics, len(c.cardMetrics))
	copy(result, c.cardMetrics)
	return result
}

// GetGPUMetricsSnapshot returns a copy of the current GPU metrics
func (c *Collector) GetGPUMetricsSnapshot() []model.GPUMetricsInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]model.GPUMetricsInfo, len(c.gpuMetrics))
	copy(result, c.gpuMetrics)
	return result
}
