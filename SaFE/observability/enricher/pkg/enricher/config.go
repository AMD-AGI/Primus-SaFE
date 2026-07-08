/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package enricher implements the SaFE-native metrics enricher: a scraper that
// reads per-GPU metrics from the AMD device-metrics-exporter, maps each GPU
// series to the owning SaFE Workload (workload_uid), and remote-writes
// robust-compatible `workload_gpu_*` series to VictoriaMetrics.
//
// This reproduces the output contract of primus-robust's telemetry-gateway
// (same metric names + labels) so the existing per-workload Grafana dashboards
// work unchanged, and so switching back to primus-robust later is a clean
// removal of this component.
package enricher

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime settings, all overridable via environment variables so
// the Helm chart can wire them without code changes.
type Config struct {
	// Interval between scrape+enrich passes.
	Interval time.Duration
	// ExporterServiceName / Namespace / Port locate the AMD
	// device-metrics-exporter. We scrape every backing endpoint so per-node
	// series are all covered (a Service ClusterIP would only hit one pod).
	ExporterServiceName string
	ExporterNamespace   string
	ExporterPort        int
	ExporterScheme      string
	ExporterPath        string
	// VMImportURL is the VictoriaMetrics Prometheus-text import endpoint on
	// vminsert, e.g.
	// http://vminsert-...:8480/insert/0/prometheus/api/v1/import/prometheus
	VMImportURL string
	// WorkloadPodLabel is the pod label whose value is the owning Workload
	// name (resolved to the Workload CR UID).
	WorkloadPodLabel string
	// ClusterName is stamped as the `cluster` label on emitted series.
	ClusterName string
	// HTTPTimeout bounds each scrape / write request.
	HTTPTimeout time.Duration
}

func LoadConfig() Config {
	return Config{
		Interval:            envDuration("ENRICHER_INTERVAL", 30*time.Second),
		ExporterServiceName: envString("EXPORTER_SERVICE", "default-metrics-exporter"),
		ExporterNamespace:   envString("EXPORTER_NAMESPACE", "kube-amd-gpu"),
		ExporterPort:        envInt("EXPORTER_PORT", 5000),
		ExporterScheme:      envString("EXPORTER_SCHEME", "http"),
		ExporterPath:        envString("EXPORTER_PATH", "/metrics"),
		VMImportURL: envString("VM_IMPORT_URL",
			"http://vminsert-primus-safe-vmcluster.primus-safe-observability.svc:8480/insert/0/prometheus/api/v1/import/prometheus"),
		WorkloadPodLabel: envString("WORKLOAD_POD_LABEL", "primus-safe.workload.id"),
		ClusterName:      envString("CLUSTER_NAME", "default"),
		HTTPTimeout:      envDuration("ENRICHER_HTTP_TIMEOUT", 20*time.Second),
	}
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
