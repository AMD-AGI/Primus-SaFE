/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package enricher

// outputMetric describes one robust-compatible `workload_gpu_*` series and how
// to derive it from whichever GPU exporter is present.
//
// The enricher is deliberately source-agnostic so it works with BOTH:
//   - primus-robust's gpu-exporter (metric names already match the robust
//     contract: gpu_utilization, gpu_pcie_bandwidth_mbs, ... in 0-100 percent),
//   - the AMD amd-gpu-operator device-metrics-exporter (gpu_gfx_activity, etc.).
//
// Derivation kind:
//   - "direct": first present Source wins, multiplied by Scale.
//   - "ratio":  Numerator/Denominator * 100.
//   - "auto":   try Sources (direct) first; if none present, fall back to the
//     Numerator/Denominator ratio. Used for memory-used-percent, which robust's
//     exporter provides directly but the AMD exporter only exposes as used/total.
//
// Source names are candidate lists (robust names listed first) so the first
// present one wins regardless of which exporter is deployed.
//
// Unit convention: all "percent" metrics are normalized to 0-100 so the
// dashboards render them directly (no *100). Robust's exporter already reports
// 0-100; the AMD exporter's activity metrics are also 0-100, so Scale is 1.
type outputMetric struct {
	// Name is the emitted metric name (robust contract), before the prefix.
	Name string
	// Kind is "direct", "ratio", or "auto".
	Kind string
	// Sources are candidate source metric names for a direct value.
	Sources []string
	// Scale multiplies a direct source value.
	Scale float64
	// Numerator / Denominator are candidate source lists for a ratio value.
	Numerator   []string
	Denominator []string
}

// outputMetrics is the enrichment table. Names + units match the primus-robust
// gpu-exporter contract (workload_gpu_utilization, workload_gpu_pcie_bandwidth_mbs,
// ...), so the same dashboards work with either exporter and switching to
// primus-robust later is a drop-in.
var outputMetrics = []outputMetric{
	{
		// 0-100 percent. robust gpu_utilization (percent) or AMD gpu_gfx_activity.
		Name:    "gpu_utilization",
		Kind:    "direct",
		Sources: []string{"gpu_utilization", "gpu_gfx_activity", "gpu_gfx_activity_percent"},
		Scale:   1,
	},
	{
		Name:    "gpu_socket_power_watts",
		Kind:    "direct",
		Sources: []string{"gpu_socket_power_watts", "gpu_socket_power", "gpu_package_power", "gpu_average_package_power", "gpu_power_usage"},
		Scale:   1,
	},
	{
		// 0-100 percent. robust exposes it directly; AMD needs used/total VRAM.
		Name:        "gpu_memory_used_percent",
		Kind:        "auto",
		Sources:     []string{"gpu_memory_used_percent"},
		Scale:       1,
		Numerator:   []string{"gpu_used_vram", "gpu_vram_used", "gpu_memory_used"},
		Denominator: []string{"gpu_total_vram", "gpu_vram_total", "gpu_memory_total"},
	},
	{
		// Memory-controller (UMC) engine activity, 0-100 percent (AMD only).
		Name:    "gpu_memory_utilization",
		Kind:    "direct",
		Sources: []string{"gpu_umc_activity", "gpu_memory_activity"},
		Scale:   1,
	},
	{
		// Absolute VRAM used, normalized to bytes (AMD exporter reports MB).
		Name:    "gpu_memory_used_bytes",
		Kind:    "direct",
		Sources: []string{"gpu_used_vram", "gpu_vram_used", "gpu_memory_used"},
		Scale:   1000000,
	},
	{
		// robust-only: PCIe bandwidth (MB/s). AMD exporter does not expose it.
		Name:    "gpu_pcie_bandwidth_mbs",
		Kind:    "direct",
		Sources: []string{"gpu_pcie_bandwidth_mbs", "gpu_pcie_bandwidth"},
		Scale:   1,
	},
	{
		Name:    "gpu_temperature_junction_celsius",
		Kind:    "direct",
		Sources: []string{"gpu_temperature_junction_celsius", "gpu_junction_temperature", "gpu_temperature_hotspot", "gpu_edge_temperature"},
		Scale:   1,
	},
	{
		Name:    "gpu_temperature_memory_celsius",
		Kind:    "direct",
		Sources: []string{"gpu_temperature_memory_celsius", "gpu_memory_temperature", "gpu_hbm_temperature"},
		Scale:   1,
	},
}

// relabelPrefix is prepended to every emitted metric name, matching robust's
// telemetry-gateway default (workload_gpu_utilization, etc.).
const relabelPrefix = "workload_"
