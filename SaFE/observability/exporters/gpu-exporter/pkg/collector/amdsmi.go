// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"encoding/json"
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/gpu-exporter/pkg/model"
)

// cardMetricsFromGPUMetrics derives the per-card metrics (utilization,
// temperature, memory %, socket power) from `amd-smi metric` output, replacing
// the retired rocm-smi card-metrics path (ADR 0003 D3 / step 9).
func cardMetricsFromGPUMetrics(m model.GPUMetricsInfo) model.CardMetrics {
	return model.CardMetrics{
		GPU:                       m.GPU,
		GPUUsePercent:             m.Usage.GFXActivity.Value,
		GFXActivity:               m.Usage.GFXActivity.Value,
		TemperatureJunction:       m.Temperature.Hotspot.Value,
		TemperatureMemory:         m.Temperature.Mem.Value,
		GPUMemoryAllocatedPercent: m.MemUsage.UsedPercent(),
		SocketGraphicsPowerWatts:  m.Power.SocketPower.Value,
	}
}

// GetGPUStaticInfo retrieves static GPU information from amd-smi
func GetGPUStaticInfo(executor *CommandExecutor) ([]model.GPUInfo, error) {
	output, err := executor.Execute("amd-smi", "static", "--json")
	if err != nil {
		return nil, fmt.Errorf("failed to execute amd-smi static: %w", err)
	}

	jsonStr := model.ExtractJSON(string(output))
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON array found in amd-smi output")
	}

	var gpus []model.GPUInfo
	if err := json.Unmarshal([]byte(jsonStr), &gpus); err != nil {
		return nil, fmt.Errorf("failed to parse GPU info JSON: %w", err)
	}

	if gpus == nil {
		gpus = []model.GPUInfo{}
	}

	return gpus, nil
}

// GetGPUMetrics retrieves runtime metrics (usage, temperature, memory, power,
// PCIe, and XGMI) from amd-smi. The full metric set is requested so the card
// metrics that previously came from rocm-smi are now sourced here too.
func GetGPUMetrics(executor *CommandExecutor) ([]model.GPUMetricsInfo, error) {
	output, err := executor.Execute("amd-smi", "metric", "--json")
	if err != nil {
		return nil, fmt.Errorf("failed to execute amd-smi metric: %w", err)
	}

	jsonStr := model.ExtractJSON(string(output))
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON array found in amd-smi metric output")
	}

	var metricsInfos []model.GPUMetricsInfo
	if err := json.Unmarshal([]byte(jsonStr), &metricsInfos); err != nil {
		return nil, fmt.Errorf("failed to parse GPU metrics JSON: %w", err)
	}

	return metricsInfos, nil
}
