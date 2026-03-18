// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type DriDevice struct {
	PCIAddr string `json:"pciAddr"`
	Card    string `json:"card"`
	CardId  int    `json:"cardId"`
	Render  string `json:"render"`
}

type GPUInfo struct {
	GPU              int           `json:"gpu"`
	Asic             AsicInfo      `json:"asic"`
	Bus              BusInfo       `json:"bus"`
	VBIOS            VBIOSInfo     `json:"vbios"`
	Limit            LimitInfo     `json:"limit"`
	Driver           DriverInfo    `json:"driver"`
	Board            BoardInfo     `json:"board"`
	RAS              RASInfo       `json:"ras"`
	Partition        PartitionInfo `json:"partition"`
	SocPState        SocPStateInfo `json:"soc_pstate"`
	XGMIPLPD         XGMIPLPDInfo  `json:"xgmi_plpd"`
	ProcessIsolation string        `json:"process_isolation"`
	NUMA             NUMAInfo      `json:"numa"`
	VRAM             VRAMInfo      `json:"vram"`
	CacheInfo        []CacheInfo   `json:"cache_info"`
	DriDevice        DriDevice     `json:"dri_device"`
}

type AsicInfo struct {
	MarketName            string      `json:"market_name"`
	VendorID              string      `json:"vendor_id"`
	VendorName            string      `json:"vendor_name"`
	SubvendorID           string      `json:"subvendor_id"`
	DeviceID              string      `json:"device_id"`
	SubsystemID           string      `json:"subsystem_id"`
	RevID                 string      `json:"rev_id"`
	AsicSerial            string      `json:"asic_serial"`
	OAMID                 interface{} `json:"oam_id"`
	NumComputeUnits       int         `json:"num_compute_units"`
	TargetGraphicsVersion string      `json:"target_graphics_version"`
}

type BusInfo struct {
	BDF                  string        `json:"bdf"`
	MaxPCIeWidth         interface{}   `json:"max_pcie_width"`
	MaxPCIeSpeed         ValueWithUnit `json:"max_pcie_speed"`
	PCIeInterfaceVersion string        `json:"pcie_interface_version"`
	SlotType             string        `json:"slot_type"`
}

func (b *BusInfo) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but without custom unmarshaling
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

type NAOrInt struct {
	Value int  `json:"value"`
	NA    bool `json:"na"`
}

func (v *NAOrInt) UnmarshalJSON(data []byte) error {
	type alias NAOrInt
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err == nil {
		*v = NAOrInt(tmp)
		return nil
	}

	var strVal string
	if err := json.Unmarshal(data, &strVal); err != nil {
		return fmt.Errorf("invalid string input: %w", err)
	}

	if strVal == "N/A" {
		*v = NAOrInt{NA: true}
		return nil
	}

	if value, err := strconv.Atoi(strVal); err == nil {
		*v = NAOrInt{Value: value}
		return nil
	}

	if err := json.Unmarshal([]byte(strVal), &tmp); err == nil {
		*v = NAOrInt(tmp)
		return nil
	}

	return fmt.Errorf("invalid NAOrInt format: %s", strVal)
}

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

func (v ValueWithUnit) ToBytes() (int64, error) {
	unit := strings.ToUpper(strings.TrimSpace(v.Unit))
	multiplier := float64(1)

	switch unit {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1 << 10 // 1024
	case "MB":
		multiplier = 1 << 20 // 1024^2
	case "GB":
		multiplier = 1 << 30 // 1024^3
	case "TB":
		multiplier = 1 << 40 // 1024^4
	default:
		return 0, fmt.Errorf("unknown unit: %s", v.Unit)
	}

	bytes := v.Value * multiplier
	if bytes > float64(int64(^uint64(0)>>1)) {
		return 0, fmt.Errorf("value exceeds int32 max: %f bytes", bytes)
	}
	return int64(bytes), nil
}

type VBIOSInfo struct {
	Name       string `json:"name"`
	BuildDate  string `json:"build_date"`
	PartNumber string `json:"part_number"`
	Version    string `json:"version"`
}

type LimitInfo struct {
	MaxPower                   ValueWithUnit `json:"max_power"`
	MinPower                   ValueWithUnit `json:"min_power"`
	SocketPower                ValueWithUnit `json:"socket_power"`
	SlowdownEdgeTemperature    interface{}   `json:"slowdown_edge_temperature"` // string "N/A"
	SlowdownHotspotTemperature ValueWithUnit `json:"slowdown_hotspot_temperature"`
	SlowdownVRAMTemperature    ValueWithUnit `json:"slowdown_vram_temperature"`
	ShutdownEdgeTemperature    interface{}   `json:"shutdown_edge_temperature"` // string "N/A"
	ShutdownHotspotTemperature ValueWithUnit `json:"shutdown_hotspot_temperature"`
	ShutdownVRAMTemperature    ValueWithUnit `json:"shutdown_vram_temperature"`
}

type DriverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type BoardInfo struct {
	ModelNumber      string `json:"model_number"`
	ProductSerial    string `json:"product_serial"`
	FRUID            string `json:"fru_id"`
	ProductName      string `json:"product_name"`
	ManufacturerName string `json:"manufacturer_name"`
}

type RASInfo struct {
	EEPROMVersion   string      `json:"eeprom_version"`
	ParitySchema    string      `json:"parity_schema"`
	SingleBitSchema string      `json:"single_bit_schema"`
	DoubleBitSchema string      `json:"double_bit_schema"`
	PoisonSchema    string      `json:"poison_schema"`
	ECCBlockState   interface{} `json:"ecc_block_state"`
}

type PartitionInfo struct {
	ComputePartition string `json:"compute_partition"`
	MemoryPartition  string `json:"memory_partition"`
	PartitionID      int    `json:"partition_id"`
}

type SocPStateInfo struct {
	NumSupported int          `json:"num_supported"`
	CurrentID    int          `json:"current_id"`
	Policies     []PolicyInfo `json:"policies"`
}

func (s *SocPStateInfo) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = SocPStateInfo{}
		return nil
	}

	type Alias SocPStateInfo
	var tmp Alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*s = SocPStateInfo(tmp)
	return nil
}

type PolicyInfo struct {
	PolicyID          int    `json:"policy_id"`
	PolicyDescription string `json:"policy_description"`
}

type XGMIPLPDInfo struct {
	NumSupported int          `json:"num_supported"`
	CurrentID    int          `json:"current_id"`
	PLPDs        []PolicyInfo `json:"plpds"`
}

func (x *XGMIPLPDInfo) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*x = XGMIPLPDInfo{}
		return nil
	}

	type Alias XGMIPLPDInfo
	var tmp Alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*x = XGMIPLPDInfo(tmp)
	return nil
}

type NUMAInfo struct {
	Node     int `json:"node"`
	Affinity int `json:"affinity"`
}

type VRAMInfo struct {
	Type     string        `json:"type"`
	Vendor   string        `json:"vendor"`
	Size     ValueWithUnit `json:"size"`
	BitWidth int           `json:"bit_width"`
}

func (v VRAMInfo) GetVramSizeMegaBytes() int32 {
	bytes, _ := v.Size.ToBytes()
	result := int32(bytes / 1024 / 1024)
	return result
}

type CacheInfo struct {
	Cache            int           `json:"cache"`
	CacheProperties  []string      `json:"cache_properties"`
	CacheSize        ValueWithUnit `json:"cache_size"`
	CacheLevel       int           `json:"cache_level"`
	MaxNumCUShared   int           `json:"max_num_cu_shared"`
	NumCacheInstance int           `json:"num_cache_instance"`
}

type RocmSmiDriverVersion struct {
	System struct {
		DriverVersion string `json:"Driver version"`
	} `json:"system"`
}

type CardMetrics struct {
	Gpu                        int     `json:"gpu"`
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

// GPUPowerInfo represents power information for a single GPU
type GPUPowerInfo struct {
	GPU   int       `json:"gpu"`
	Power PowerInfo `json:"power"`
}

// GPUMetricsInfo represents metrics information for a single GPU (including power and PCIE)
type GPUMetricsInfo struct {
	GPU   int       `json:"gpu"`
	Power PowerInfo `json:"power"`
	PCIE  PCIEInfo  `json:"pcie"`
}

// GPUPCIEInfo represents PCIE information for a single GPU
type GPUPCIEInfo struct {
	GPU  int      `json:"gpu"`
	PCIE PCIEInfo `json:"pcie"`
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
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "N/A" || str == "" {
			v.IsNA = true
			v.Value = 0
			return nil
		}
		if f, err := strconv.ParseFloat(str, 64); err == nil {
			v.Value = f
			v.IsNA = false
			return nil
		}
		v.IsNA = true
		v.Value = 0
		return nil
	}

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
