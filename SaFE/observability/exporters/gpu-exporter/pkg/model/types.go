// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ValueWithUnit represents a value with its unit (e.g., power, temperature)
type ValueWithUnit struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

func (v *ValueWithUnit) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "N/A" {
			*v = ValueWithUnit{}
			return nil
		}
	}

	type alias ValueWithUnit
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*v = ValueWithUnit(tmp)
	return nil
}

// CardMetrics represents metrics from rocm-smi
type CardMetrics struct {
	GPU                        int     `json:"gpu"`
	TemperatureJunction        float64 `json:"temperature_junction"`
	TemperatureMemory          float64 `json:"temperature_memory"`
	SocketGraphicsPowerWatts   float64 `json:"socket_graphics_power_watts"`
	GPUUsePercent              float64 `json:"gpu_use_percent"`
	GFXActivity                float64 `json:"gfx_activity"`
	GPUMemoryAllocatedPercent  float64 `json:"gpu_memory_allocated_percent"`
	GPUMemoryRWActivityPercent float64 `json:"gpu_memory_rw_activity_percent"`
	MemoryActivity             float64 `json:"memory_activity"`
	AvgMemoryBandwidth         float64 `json:"avg_memory_bandwidth"`
}

func parseFloatField(m map[string]any, key string) float64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		case float64:
			return v
		}
	}
	return 0
}

func (c *CardMetrics) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, ok := raw["temperature_junction"]; ok {
		c.TemperatureJunction = parseFloatField(raw, "temperature_junction")
	} else {
		c.TemperatureJunction = parseFloatField(raw, "Temperature (Sensor junction) (C)")
	}
	if _, ok := raw["temperature_memory"]; ok {
		c.TemperatureMemory = parseFloatField(raw, "temperature_memory")
	} else {
		c.TemperatureMemory = parseFloatField(raw, "Temperature (Sensor memory) (C)")
	}
	if _, ok := raw["socket_graphics_power_watts"]; ok {
		c.SocketGraphicsPowerWatts = parseFloatField(raw, "socket_graphics_power_watts")
	} else {
		c.SocketGraphicsPowerWatts = parseFloatField(raw, "Current Socket Graphics Package Power (W)")
	}
	if _, ok := raw["gpu_use_percent"]; ok {
		c.GPUUsePercent = parseFloatField(raw, "gpu_use_percent")
	} else {
		c.GPUUsePercent = parseFloatField(raw, "GPU use (%)")
	}
	if _, ok := raw["gpu_memory_allocated_percent"]; ok {
		c.GPUMemoryAllocatedPercent = parseFloatField(raw, "gpu_memory_allocated_percent")
	} else {
		c.GPUMemoryAllocatedPercent = parseFloatField(raw, "GPU Memory Allocated (VRAM%)")
	}
	if _, ok := raw["gfx_activity"]; ok {
		c.GFXActivity = parseFloatField(raw, "gfx_activity")
	} else {
		c.GFXActivity = parseFloatField(raw, "GFX Activity")
	}
	if _, ok := raw["gpu_memory_rw_activity_percent"]; ok {
		c.GPUMemoryRWActivityPercent = parseFloatField(raw, "gpu_memory_rw_activity_percent")
	} else {
		c.GPUMemoryRWActivityPercent = parseFloatField(raw, "GPU Memory Read/Write Activity (%)")
	}
	if _, ok := raw["memory_activity"]; ok {
		c.MemoryActivity = parseFloatField(raw, "memory_activity")
	} else {
		c.MemoryActivity = parseFloatField(raw, "Memory Activity")
	}
	if _, ok := raw["avg_memory_bandwidth"]; ok {
		c.AvgMemoryBandwidth = parseFloatField(raw, "avg_memory_bandwidth")
	} else {
		c.AvgMemoryBandwidth = parseFloatField(raw, "Avg. Memory Bandwidth")
	}
	return nil
}

// GPUMetricsInfo represents metrics information for a single GPU from
// `amd-smi metric` (usage, temperature, memory, power, PCIe, and XGMI).
type GPUMetricsInfo struct {
	GPU         int               `json:"gpu"`
	Usage       UsageInfo         `json:"usage"`
	Temperature TemperatureInfo   `json:"temperature"`
	MemUsage    MemUsageInfo      `json:"mem_usage"`
	Power       PowerInfo         `json:"power"`
	PCIE        PCIEInfo          `json:"pcie"`
	XGMILink    []XGMILinkMetrics `json:"xgmi_link"`
}

// UsageInfo represents GPU activity from `amd-smi metric --usage`.
type UsageInfo struct {
	GFXActivity ValueWithUnit `json:"gfx_activity"`
	UMCActivity ValueWithUnit `json:"umc_activity"`
	MMActivity  ValueWithUnit `json:"mm_activity"`
}

// TemperatureInfo represents GPU temperatures from `amd-smi metric --temperature`.
type TemperatureInfo struct {
	Edge    ValueWithUnit `json:"edge"`
	Hotspot ValueWithUnit `json:"hotspot"`
	Mem     ValueWithUnit `json:"mem"`
}

// MemUsageInfo represents VRAM usage from `amd-smi metric --mem-usage`.
type MemUsageInfo struct {
	TotalVRAM ValueWithUnit `json:"total_vram"`
	UsedVRAM  ValueWithUnit `json:"used_vram"`
	FreeVRAM  ValueWithUnit `json:"free_vram"`
}

// UsedPercent returns VRAM used as a percentage of total (0 when total is 0).
func (m MemUsageInfo) UsedPercent() float64 {
	if m.TotalVRAM.Value <= 0 {
		return 0
	}
	return m.UsedVRAM.Value / m.TotalVRAM.Value * 100
}

// PowerInfo represents detailed power information for a GPU
type PowerInfo struct {
	SocketPower     ValueWithUnit    `json:"socket_power"`
	GfxVoltage      VoltageValueOrNA `json:"gfx_voltage"`
	SocVoltage      VoltageValueOrNA `json:"soc_voltage"`
	MemVoltage      VoltageValueOrNA `json:"mem_voltage"`
	ThrottleStatus  string           `json:"throttle_status"`
	PowerManagement string           `json:"power_management"`
}

// PCIEInfo represents detailed PCIE information for a GPU
type PCIEInfo struct {
	Width                    StringOrNA    `json:"width"`
	Speed                    ValueWithUnit `json:"speed"`
	Bandwidth                ValueWithUnit `json:"bandwidth"`
	ReplayCount              StringOrNA    `json:"replay_count"`
	L0ToRecoveryCount        StringOrNA    `json:"l0_to_recovery_count"`
	ReplayRollOverCount      StringOrNA    `json:"replay_roll_over_count"`
	NAKSentCount             StringOrNA    `json:"nak_sent_count"`
	NAKReceivedCount         StringOrNA    `json:"nak_received_count"`
	CurrentBandwidthSent     StringOrNA    `json:"current_bandwidth_sent"`
	CurrentBandwidthReceived StringOrNA    `json:"current_bandwidth_received"`
	MaxPacketSize            StringOrNA    `json:"max_packet_size"`
	LCPerfOtherEndRecovery   StringOrNA    `json:"lc_perf_other_end_recovery"`
}

// StringOrNA represents a string or numeric value that may be "N/A"
type StringOrNA struct {
	Value string `json:"-"`
	IsNA  bool   `json:"-"`
}

func (s *StringOrNA) UnmarshalJSON(data []byte) error {
	// Try to parse as string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "N/A" {
			s.IsNA = true
			s.Value = ""
		} else {
			s.IsNA = false
			s.Value = str
		}
		return nil
	}

	// Try to parse as number
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		s.IsNA = false
		s.Value = strconv.FormatFloat(num, 'f', -1, 64)
		return nil
	}

	// Try to parse as ValueWithUnit object
	var vwu ValueWithUnit
	if err := json.Unmarshal(data, &vwu); err == nil {
		s.IsNA = false
		s.Value = strconv.FormatFloat(vwu.Value, 'f', -1, 64)
		return nil
	}

	return fmt.Errorf("unable to unmarshal StringOrNA from: %s", string(data))
}

// VoltageValueOrNA represents a voltage value that may be numeric or "N/A"
type VoltageValueOrNA struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	IsNA  bool    `json:"-"`
}

func (v *VoltageValueOrNA) UnmarshalJSON(data []byte) error {
	type Alias struct {
		Value interface{} `json:"value"`
		Unit  string      `json:"unit"`
	}

	var tmp Alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	v.Unit = tmp.Unit

	switch val := tmp.Value.(type) {
	case float64:
		v.Value = val
		v.IsNA = false
	case string:
		if val == "N/A" {
			v.IsNA = true
			v.Value = 0
		} else {
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				v.Value = f
				v.IsNA = false
			} else {
				return fmt.Errorf("invalid voltage value: %s", val)
			}
		}
	default:
		return fmt.Errorf("unexpected type for voltage value: %T", val)
	}

	return nil
}

// XGMILinkMetrics represents metrics for a single XGMI link between GPUs.
// Collected via `amd-smi metric -x --json`.
type XGMILinkMetrics struct {
	Link         string        `json:"link"`
	BDF          string        `json:"bdf"`
	BitRate      ValueWithUnit `json:"bit_rate"`
	MaxBandwidth ValueWithUnit `json:"max_bandwidth"`
	Read         ValueWithUnit `json:"read"`
	Write        ValueWithUnit `json:"write"`
}

// RocmSmiDriverVersion represents driver version from rocm-smi
type RocmSmiDriverVersion struct {
	System struct {
		DriverVersion string `json:"Driver version"`
	} `json:"system"`
}

// GPUInfo represents static GPU information from amd-smi
type GPUInfo struct {
	GPU    int      `json:"gpu"`
	Asic   AsicInfo `json:"asic"`
	Bus    BusInfo  `json:"bus"`
	Driver struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"driver"`
}

// AsicInfo represents ASIC information
type AsicInfo struct {
	MarketName  string `json:"market_name"`
	VendorID    string `json:"vendor_id"`
	DeviceID    string `json:"device_id"`
	AsicSerial  string `json:"asic_serial"`
	SubsystemID string `json:"subsystem_id"`
}

// BusInfo represents PCI bus information
type BusInfo struct {
	BDF                  string        `json:"bdf"`
	MaxPCIeWidth         interface{}   `json:"max_pcie_width"`
	MaxPCIeSpeed         ValueWithUnit `json:"max_pcie_speed"`
	PCIeInterfaceVersion string        `json:"pcie_interface_version"`
	SlotType             string        `json:"slot_type"`
}

func (b *BusInfo) UnmarshalJSON(data []byte) error {
	type Alias BusInfo
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(b),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Convert MaxPCIeWidth from float64 to int if it's a number
	if b.MaxPCIeWidth != nil {
		switch v := b.MaxPCIeWidth.(type) {
		case float64:
			b.MaxPCIeWidth = int(v)
		case int:
			// Already an int, no conversion needed
		case string:
			// Try to parse string as int
			if intVal, err := strconv.Atoi(v); err == nil {
				b.MaxPCIeWidth = intVal
			}
			// Otherwise keep as string (e.g., "N/A")
		}
	}

	return nil
}

// extractJSON extracts JSON array from raw output
func ExtractJSON(raw string) string {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start != -1 && end != -1 && end > start {
		return raw[start : end+1]
	}
	return ""
}
