// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsProbeInterval = 30 * time.Second
	metricsProbeWait     = 15 * time.Second // wait after process start before first probe
)

var (
	componentHealthyGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "primus_lens",
			Subsystem: "component",
			Name:      "healthy",
			Help:      "1 if component is healthy (ready >= 1 or ready == desired for kube-system), 0 otherwise",
		},
		[]string{"category", "cluster", "component", "platform", "app_name", "namespace"},
	)
)

func init() {
	prometheus.MustRegister(componentHealthyGauge)
	go runComponentProbeMetricsLoop()
}

func runComponentProbeMetricsLoop() {
	time.Sleep(metricsProbeWait)
	ticker := time.NewTicker(metricsProbeInterval)
	defer ticker.Stop()
	for range ticker.C {
		updateComponentProbeMetrics(context.Background())
	}
}

func updateComponentProbeMetrics(ctx context.Context) {
	cm := clientsets.GetClusterManager()
	if cm == nil {
		return
	}
	clients, err := cm.GetClusterClientsOrDefault("")
	if err != nil || clients == nil || clients.K8SClientSet == nil || clients.K8SClientSet.ControllerRuntimeClient == nil {
		return
	}
	c := clients.K8SClientSet.ControllerRuntimeClient
	clusterName := clients.ClusterName

	// Reset gauges for this cluster so stale labels disappear (optional: only set current cluster labels)
	// Here we set only the labels we'll update this round; old labels may remain until next full sync.
	// For simplicity we don't reset the whole vector; we only Set() below.

	// 1) Kube-system: CoreDNS
	if item, err := probeCoreDNS(ctx, c, clusterName); err == nil {
		v := 0.0
		if item.Healthy {
			v = 1.0
		}
		componentHealthyGauge.WithLabelValues("kube_system", clusterName, "coredns", "", "", "").Set(v)
	}

	// 2) Kube-system: NodeLocal DNS
	if item, err := probeNodeLocalDNS(ctx, c, clusterName); err == nil {
		v := 0.0
		if item.Healthy {
			v = 1.0
		}
		componentHealthyGauge.WithLabelValues("kube_system", clusterName, "node_local_dns", "", "", "").Set(v)
	}

	// 3) Platform: Primus-SaFE
	safeList, _ := listComponentsByLabel(ctx, c, labelPrimusSafeAppName)
	for _, comp := range safeList {
		v := 0.0
		if comp.Healthy {
			v = 1.0
		}
		componentHealthyGauge.WithLabelValues("platform", clusterName, "", "primus_safe", comp.AppName, comp.Namespace).Set(v)
	}

	// 4) Platform: Primus-Lens
	lensList, _ := listComponentsByLabel(ctx, c, labelPrimusLensAppName)
	for _, comp := range lensList {
		v := 0.0
		if comp.Healthy {
			v = 1.0
		}
		componentHealthyGauge.WithLabelValues("platform", clusterName, "", "primus_lens", comp.AppName, comp.Namespace).Set(v)
	}
}