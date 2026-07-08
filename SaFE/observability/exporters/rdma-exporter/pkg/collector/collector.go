// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
	"github.com/prometheus/client_golang/prometheus"
)

// sysfsInfinibandPath is where RDMA devices expose their PCIe `device` symlink.
const sysfsInfinibandPath = "/sys/class/infiniband"

// deviceAddress resolves an RDMA device's PCIe BDF (address) via sysfs:
// /sys/class/infiniband/<dev>/device -> .../0000:c1:00.0. Returns "" if unknown.
func deviceAddress(ifname string) string {
	link, err := os.Readlink(filepath.Join(sysfsInfinibandPath, ifname, "device"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

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

var (
	rdmaDeviceInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rdma_device_info",
			Help: "RDMA device information (value is always 1)",
		},
		[]string{"node", "device", "address", "node_type", "fw"},
	)
)

func init() {
	prometheus.MustRegister(rdmaDeviceInfo)
}

// Collector manages RDMA metrics collection
type Collector struct {
	interval       int
	executor       *CommandExecutor
	mu             sync.RWMutex
	devices        []model.RDMADevice
	deviceAddr     map[string]string // ifname -> PCIe BDF (address)
	metrics        map[string]*prometheus.GaugeVec
	metricsLock    sync.Mutex
	sysfsCollector *SysfsCollector
	qpCollector    *QPCollector
	enableSysfs    bool
	enableQP       bool
}

// New creates a new Collector
func New(interval int) *Collector {
	return NewWithOptions(interval, true, true)
}

// NewWithOptions creates a new Collector with feature toggles.
func NewWithOptions(interval int, enableSysfs, enableQP bool) *Collector {
	executor := NewCommandExecutor()
	c := &Collector{
		interval:    interval,
		executor:    executor,
		deviceAddr:  make(map[string]string),
		metrics:     make(map[string]*prometheus.GaugeVec),
		enableSysfs: enableSysfs,
		enableQP:    enableQP,
	}
	if enableSysfs {
		c.sysfsCollector = NewSysfsCollector(nodeName)
	}
	if enableQP {
		c.qpCollector = NewQPCollector(executor, nodeName)
	}
	return c
}

// GetExecutor returns the command executor
func (c *Collector) GetExecutor() *CommandExecutor {
	return c.executor
}

// Start begins the metrics collection loop
func (c *Collector) Start(ctx context.Context) {
	c.refreshDevices()
	c.collect()

	ticker := time.NewTicker(time.Duration(c.interval) * time.Second)
	defer ticker.Stop()

	deviceTicker := time.NewTicker(60 * time.Second)
	defer deviceTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Collector stopping")
			return
		case <-ticker.C:
			c.collect()
		case <-deviceTicker.C:
			c.refreshDevices()
		}
	}
}

// NodeName returns the node name used for metric labels.
func NodeName() string { return nodeName }

func (c *Collector) refreshDevices() {
	devices, err := c.getRDMADevices()
	if err != nil {
		slog.Error("Failed to get RDMA devices", "error", err)
		return
	}

	addr := make(map[string]string, len(devices))
	for _, dev := range devices {
		addr[dev.IfName] = deviceAddress(dev.IfName)
	}

	c.mu.Lock()
	c.devices = devices
	c.deviceAddr = addr
	c.mu.Unlock()

	// Update device info metric
	for _, dev := range devices {
		rdmaDeviceInfo.WithLabelValues(nodeName, dev.IfName, addr[dev.IfName], dev.NodeType, dev.FW).Set(1)
	}

	slog.Debug("RDMA devices refreshed", "count", len(devices))
}

// addrFor returns the cached PCIe BDF for an RDMA device ("" when unknown).
func (c *Collector) addrFor(ifname string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deviceAddr[ifname]
}

func (c *Collector) getRDMADevices() ([]model.RDMADevice, error) {
	output, err := c.executor.Execute("rdma", "dev", "show", "-j")
	if err != nil {
		return nil, fmt.Errorf("failed to run rdma command: %w", err)
	}

	var devices []model.RDMADevice
	if err := json.Unmarshal(output, &devices); err != nil {
		return nil, fmt.Errorf("failed to parse rdma output: %w", err)
	}

	return devices, nil
}

func (c *Collector) collect() {
	stats, err := c.getRDMAStatistics()
	if err != nil {
		slog.Error("Failed to get RDMA statistics", "error", err)
		return
	}

	c.metricsLock.Lock()
	defer c.metricsLock.Unlock()

	for _, stat := range stats {
		addr := c.addrFor(stat.Device)
		for key, value := range stat.Stats {
			metricName := "rdma_stat_" + sanitizeMetricName(key)

			gauge, exists := c.metrics[metricName]
			if !exists {
				gauge = prometheus.NewGaugeVec(
					prometheus.GaugeOpts{
						Name: metricName,
						Help: fmt.Sprintf("RDMA statistic: %s", key),
					},
					[]string{"node", "device", "address", "port"},
				)
				if err := prometheus.Register(gauge); err != nil {
					// Metric might already be registered
					if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
						gauge = are.ExistingCollector.(*prometheus.GaugeVec)
					} else {
						slog.Warn("Failed to register metric", "name", metricName, "error", err)
						continue
					}
				}
				c.metrics[metricName] = gauge
			}

			gauge.WithLabelValues(nodeName, stat.Device, addr, stat.Port).Set(float64(value))
		}
	}

	slog.Debug("RDMA metrics collected", "stats_count", len(stats))

	if c.sysfsCollector != nil {
		devices := c.GetDevicesSnapshot()
		c.sysfsCollector.Collect(devices, c.addrSnapshot())
	}
	if c.qpCollector != nil {
		c.qpCollector.Collect()
	}
}

func (c *Collector) getRDMAStatistics() ([]model.RDMAStat, error) {
	output, err := c.executor.Execute("rdma", "statistic", "show")
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}

	return parseRDMAStatistics(string(output))
}

func parseRDMAStatistics(output string) ([]model.RDMAStat, error) {
	var results []model.RDMAStat

	// Split by "link " to get each device/port section
	sections := strings.Split(output, "link ")
	for _, section := range sections[1:] { // Skip first empty section
		lines := strings.Split(strings.TrimSpace(section), "\n")
		if len(lines) == 0 {
			continue
		}

		// Parse device/port from first line
		parts := strings.Fields(lines[0])
		if len(parts) < 1 {
			continue
		}

		devicePort := parts[0]
		var device, port string
		if strings.Contains(devicePort, "/") {
			dp := strings.SplitN(devicePort, "/", 2)
			device = dp[0]
			port = dp[1]
		} else {
			device = devicePort
			port = "unknown"
		}

		stat := model.RDMAStat{
			Device: device,
			Port:   port,
			Stats:  make(map[string]int64),
		}

		// Parse stats from all lines (including first line after device/port)
		for _, line := range lines {
			fields := strings.Fields(line)
			// Skip device/port identifier
			startIdx := 0
			if strings.Contains(fields[0], "/") || fields[0] == device {
				startIdx = 1
			}

			for i := startIdx; i+1 < len(fields); i += 2 {
				key := fields[i]
				valStr := fields[i+1]
				val, err := strconv.ParseInt(valStr, 10, 64)
				if err == nil {
					if val < 0 {
						val = int64(uint32(val))
					}
					stat.Stats[key] = val
				}
			}
		}

		if len(stat.Stats) > 0 {
			results = append(results, stat)
		}
	}

	return results, nil
}

func sanitizeMetricName(name string) string {
	// Replace non-alphanumeric characters with underscores
	result := strings.ToLower(name)
	result = strings.ReplaceAll(result, "-", "_")
	result = strings.ReplaceAll(result, ".", "_")
	result = strings.ReplaceAll(result, " ", "_")
	return result
}

// GetDevicesSnapshot returns a copy of the current devices
func (c *Collector) GetDevicesSnapshot() []model.RDMADevice {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]model.RDMADevice, len(c.devices))
	copy(result, c.devices)
	return result
}

// addrSnapshot returns a copy of the ifname -> address map.
func (c *Collector) addrSnapshot() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]string, len(c.deviceAddr))
	for k, v := range c.deviceAddr {
		result[k] = v
	}
	return result
}
