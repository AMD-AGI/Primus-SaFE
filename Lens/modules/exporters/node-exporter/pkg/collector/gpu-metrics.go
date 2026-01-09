// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/kubelet"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	nodeK8SGpuAllocationRate = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "node_k8s_gpu_allocation_rate",
		Help:        "node k8s gpu allocation rate",
		ConstLabels: nil,
	})
	gpuUtilization = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gpu_utilization",
		Help: "gpu utilization",
	}, []string{"gpu_id"})
	gpuSocketPower = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gpu_socket_power_watts",
		Help: "gpu socket power in watts",
	}, []string{"gpu_id"})
	gpuPCIEBandwidth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gpu_pcie_bandwidth_mbs",
		Help: "gpu pcie bandwidth in Mb/s",
	}, []string{"gpu_id"})
)

func init() {
	prometheus.MustRegister(nodeK8SGpuAllocationRate)
	prometheus.MustRegister(gpuUtilization)
	prometheus.MustRegister(gpuSocketPower)
	prometheus.MustRegister(gpuPCIEBandwidth)
}

func runLoadGpuMetrics(ctx context.Context) {
	for {
		err := loadGpuUtilization(ctx)
		if err != nil {
			log.Errorf("Failed to load gpu utilization: %v", err)
		}
		err = loadGpuAllocationRate(ctx)
		if err != nil {
			log.Errorf("Failed to load gpu allocation rate: %v", err)

		}
		err = loadGpuPower(ctx)
		if err != nil {
			log.Errorf("Failed to load gpu power: %v", err)
		}
		err = loadGpuPCIE(ctx)
		if err != nil {
			log.Errorf("Failed to load gpu pcie: %v", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func loadGpuAllocationRate(ctx context.Context) error {
	gpuCount := len(GetGpuDeviceInfo())
	nodeName := os.Getenv("NODE_NAME")
	nodeIp := os.Getenv("NODE_IP")
	// Use empty string for current cluster default authentication in node-exporter
	pods, err := kubelet.GetGpuPodsByKubeletAddress(ctx, nodeName, fmt.Sprintf("https://%s:%d", nodeIp, 10250), "", metadata.GpuVendorAMD)
	if err != nil {
		return err
	}
	allocated := gpu.GetAllocatedGpuResourceFromPods(ctx, pods, metadata.GetResourceName(metadata.GpuVendorAMD))
	rate := 0.0
	if gpuCount > 0 {
		rate = float64(allocated) / float64(gpuCount) * 100
	}
	nodeK8SGpuAllocationRate.Set(rate)
	return nil
}

func loadGpuUtilization(ctx context.Context) error {
	for _, metrics := range GetCardMetrics() {
		gpuUtilization.WithLabelValues(fmt.Sprintf("%d", metrics.Gpu)).Set(metrics.GPUUsePercent)
	}
	return nil
}

func loadGpuPower(ctx context.Context) error {
	for _, powerInfo := range GetGPUPowerInfo() {
		gpuSocketPower.WithLabelValues(fmt.Sprintf("%d", powerInfo.GPU)).Set(powerInfo.Power.SocketPower.Value)
	}
	return nil
}

func loadGpuPCIE(ctx context.Context) error {
	pcieInfos := GetPCIEGPUMetricsInfo()
	for _, pcieInfo := range pcieInfos {
		gpuID := fmt.Sprintf("%d", pcieInfo.GPU)
		gpuPCIEBandwidth.WithLabelValues(gpuID).Set(pcieInfo.PCIE.Bandwidth.Value)
	}
	return nil
}
