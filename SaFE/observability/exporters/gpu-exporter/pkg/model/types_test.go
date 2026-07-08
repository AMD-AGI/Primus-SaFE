// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

package model

import (
	"encoding/json"
	"testing"
)

func TestValueWithUnitUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ValueWithUnit
	}{
		{
			"normal value",
			`{"value": 100.5, "unit": "W"}`,
			ValueWithUnit{Value: 100.5, Unit: "W"},
		},
		{
			"zero value",
			`{"value": 0, "unit": "MHz"}`,
			ValueWithUnit{Value: 0, Unit: "MHz"},
		},
		{
			"N/A string",
			`"N/A"`,
			ValueWithUnit{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v ValueWithUnit
			err := json.Unmarshal([]byte(tt.input), &v)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.Value != tt.expected.Value {
				t.Errorf("value: got %f, want %f", v.Value, tt.expected.Value)
			}
			if v.Unit != tt.expected.Unit {
				t.Errorf("unit: got %q, want %q", v.Unit, tt.expected.Unit)
			}
		})
	}
}

func TestCardMetricsUnmarshalNewFormat(t *testing.T) {
	input := `{
		"temperature_junction": 65.0,
		"temperature_memory": 50.0,
		"socket_graphics_power_watts": 200.5,
		"gpu_use_percent": 85.0,
		"gpu_memory_allocated_percent": 70.0,
		"gfx_activity": 90.0,
		"gpu_memory_rw_activity_percent": 60.0,
		"memory_activity": 55.0,
		"avg_memory_bandwidth": 45.0
	}`

	var cm CardMetrics
	err := json.Unmarshal([]byte(input), &cm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cm.TemperatureJunction != 65.0 {
		t.Errorf("TemperatureJunction: got %f, want 65.0", cm.TemperatureJunction)
	}
	if cm.TemperatureMemory != 50.0 {
		t.Errorf("TemperatureMemory: got %f, want 50.0", cm.TemperatureMemory)
	}
	if cm.SocketGraphicsPowerWatts != 200.5 {
		t.Errorf("SocketGraphicsPowerWatts: got %f, want 200.5", cm.SocketGraphicsPowerWatts)
	}
	if cm.GPUUsePercent != 85.0 {
		t.Errorf("GPUUsePercent: got %f, want 85.0", cm.GPUUsePercent)
	}
	if cm.GPUMemoryAllocatedPercent != 70.0 {
		t.Errorf("GPUMemoryAllocatedPercent: got %f, want 70.0", cm.GPUMemoryAllocatedPercent)
	}
	if cm.GFXActivity != 90.0 {
		t.Errorf("GFXActivity: got %f, want 90.0", cm.GFXActivity)
	}
}

func TestCardMetricsUnmarshalLegacyFormat(t *testing.T) {
	input := `{
		"Temperature (Sensor junction) (C)": "72",
		"Temperature (Sensor memory) (C)": "55",
		"Current Socket Graphics Package Power (W)": "150.3",
		"GPU use (%)": "40",
		"GPU Memory Allocated (VRAM%)": "30",
		"GFX Activity": "50",
		"GPU Memory Read/Write Activity (%)": "25",
		"Memory Activity": "20",
		"Avg. Memory Bandwidth": "15"
	}`

	var cm CardMetrics
	err := json.Unmarshal([]byte(input), &cm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cm.TemperatureJunction != 72.0 {
		t.Errorf("TemperatureJunction: got %f, want 72.0", cm.TemperatureJunction)
	}
	if cm.SocketGraphicsPowerWatts != 150.3 {
		t.Errorf("SocketGraphicsPowerWatts: got %f, want 150.3", cm.SocketGraphicsPowerWatts)
	}
	if cm.GPUUsePercent != 40.0 {
		t.Errorf("GPUUsePercent: got %f, want 40.0", cm.GPUUsePercent)
	}
}

func TestCardMetricsUnmarshalStringValues(t *testing.T) {
	input := `{
		"temperature_junction": "65.5",
		"temperature_memory": "50.0",
		"socket_graphics_power_watts": "200",
		"gpu_use_percent": "85",
		"gpu_memory_allocated_percent": "70",
		"gfx_activity": "90",
		"gpu_memory_rw_activity_percent": "60",
		"memory_activity": "55",
		"avg_memory_bandwidth": "45"
	}`

	var cm CardMetrics
	err := json.Unmarshal([]byte(input), &cm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cm.TemperatureJunction != 65.5 {
		t.Errorf("TemperatureJunction: got %f, want 65.5", cm.TemperatureJunction)
	}
}

func TestStringOrNAUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected StringOrNA
	}{
		{"string value", `"16"`, StringOrNA{Value: "16", IsNA: false}},
		{"N/A", `"N/A"`, StringOrNA{Value: "", IsNA: true}},
		{"numeric value", `42`, StringOrNA{Value: "42", IsNA: false}},
		{"float value", `3.14`, StringOrNA{Value: "3.14", IsNA: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s StringOrNA
			err := json.Unmarshal([]byte(tt.input), &s)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.IsNA != tt.expected.IsNA {
				t.Errorf("IsNA: got %v, want %v", s.IsNA, tt.expected.IsNA)
			}
			if s.Value != tt.expected.Value {
				t.Errorf("Value: got %q, want %q", s.Value, tt.expected.Value)
			}
		})
	}
}

func TestStringOrNAUnmarshalValueWithUnit(t *testing.T) {
	input := `{"value": 8.0, "unit": "GT/s"}`
	var s StringOrNA
	err := json.Unmarshal([]byte(input), &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.IsNA {
		t.Error("should not be N/A")
	}
	if s.Value != "8" {
		t.Errorf("expected Value 8, got %q", s.Value)
	}
}

func TestVoltageValueOrNAUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expValue   float64
		expUnit    string
		expIsNA    bool
		shouldFail bool
	}{
		{"numeric value", `{"value": 1.2, "unit": "V"}`, 1.2, "V", false, false},
		{"N/A value", `{"value": "N/A", "unit": "mV"}`, 0, "mV", true, false},
		{"string numeric", `{"value": "0.85", "unit": "V"}`, 0.85, "V", false, false},
		{"zero value", `{"value": 0, "unit": "V"}`, 0, "V", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v VoltageValueOrNA
			err := json.Unmarshal([]byte(tt.input), &v)
			if tt.shouldFail {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.Value != tt.expValue {
				t.Errorf("Value: got %f, want %f", v.Value, tt.expValue)
			}
			if v.Unit != tt.expUnit {
				t.Errorf("Unit: got %q, want %q", v.Unit, tt.expUnit)
			}
			if v.IsNA != tt.expIsNA {
				t.Errorf("IsNA: got %v, want %v", v.IsNA, tt.expIsNA)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal JSON array", `[{"gpu": 0}]`, `[{"gpu": 0}]`},
		{"with prefix text", `some output\n[{"gpu": 0}]`, `[{"gpu": 0}]`},
		{"with suffix text", `[{"gpu": 0}]\nsome trailing`, `[{"gpu": 0}]`},
		{"with both prefix and suffix", `prefix [1,2,3] suffix`, `[1,2,3]`},
		{"no JSON array", `no array here`, ""},
		{"empty string", "", ""},
		{"only brackets", `[]`, `[]`},
		{"nested arrays", `[[1,2],[3,4]]`, `[[1,2],[3,4]]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractJSON(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractJSON(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseFloatField(t *testing.T) {
	m := map[string]any{
		"float_val":  float64(42.5),
		"string_val": "100.5",
		"bad_string": "not_a_number",
		"nil_val":    nil,
	}

	if v := parseFloatField(m, "float_val"); v != 42.5 {
		t.Errorf("float_val: got %f, want 42.5", v)
	}
	if v := parseFloatField(m, "string_val"); v != 100.5 {
		t.Errorf("string_val: got %f, want 100.5", v)
	}
	if v := parseFloatField(m, "bad_string"); v != 0 {
		t.Errorf("bad_string: got %f, want 0", v)
	}
	if v := parseFloatField(m, "missing"); v != 0 {
		t.Errorf("missing: got %f, want 0", v)
	}
}

func TestBusInfoUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			"numeric width",
			`{"bdf": "0000:03:00.0", "max_pcie_width": 16, "max_pcie_speed": {"value": 16.0, "unit": "GT/s"}, "pcie_interface_version": "Gen 4", "slot_type": "PCIE"}`,
		},
		{
			"string width",
			`{"bdf": "0000:03:00.0", "max_pcie_width": "16", "max_pcie_speed": {"value": 16.0, "unit": "GT/s"}, "pcie_interface_version": "Gen 4", "slot_type": "PCIE"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b BusInfo
			err := json.Unmarshal([]byte(tt.input), &b)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b.BDF != "0000:03:00.0" {
				t.Errorf("BDF: got %q, want 0000:03:00.0", b.BDF)
			}
		})
	}
}

func TestGPUInfoUnmarshal(t *testing.T) {
	input := `{
		"gpu": 0,
		"asic": {
			"market_name": "MI300X",
			"vendor_id": "0x1002",
			"device_id": "0x7400",
			"asic_serial": "ABC123",
			"subsystem_id": "0x1234"
		},
		"bus": {
			"bdf": "0000:03:00.0",
			"max_pcie_width": 16,
			"max_pcie_speed": {"value": 16.0, "unit": "GT/s"},
			"pcie_interface_version": "Gen 4",
			"slot_type": "PCIE"
		},
		"driver": {
			"name": "amdgpu",
			"version": "6.7.0"
		}
	}`

	var gpu GPUInfo
	err := json.Unmarshal([]byte(input), &gpu)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gpu.GPU != 0 {
		t.Errorf("GPU: got %d, want 0", gpu.GPU)
	}
	if gpu.Asic.MarketName != "MI300X" {
		t.Errorf("MarketName: got %q, want MI300X", gpu.Asic.MarketName)
	}
	if gpu.Driver.Name != "amdgpu" {
		t.Errorf("DriverName: got %q, want amdgpu", gpu.Driver.Name)
	}
}

func TestRocmSmiDriverVersionUnmarshal(t *testing.T) {
	input := `{"system": {"Driver version": "6.7.0"}}`

	var dv RocmSmiDriverVersion
	err := json.Unmarshal([]byte(input), &dv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dv.System.DriverVersion != "6.7.0" {
		t.Errorf("DriverVersion: got %q, want 6.7.0", dv.System.DriverVersion)
	}
}

func TestGPUMetricsInfoUnmarshal(t *testing.T) {
	input := `{
		"gpu": 0,
		"power": {
			"socket_power": {"value": 200.0, "unit": "W"},
			"gfx_voltage": {"value": 1.2, "unit": "V"},
			"soc_voltage": {"value": 0.85, "unit": "V"},
			"mem_voltage": {"value": "N/A", "unit": "V"},
			"throttle_status": "UNTHROTTLED",
			"power_management": "ENABLED"
		},
		"pcie": {
			"width": "16",
			"speed": {"value": 16.0, "unit": "GT/s"},
			"bandwidth": {"value": 31508.0, "unit": "Mb/s"},
			"replay_count": "0",
			"l0_to_recovery_count": "N/A",
			"replay_roll_over_count": "N/A",
			"nak_sent_count": "N/A",
			"nak_received_count": "N/A",
			"current_bandwidth_sent": "N/A",
			"current_bandwidth_received": "N/A",
			"max_packet_size": "N/A",
			"lc_perf_other_end_recovery": "N/A"
		}
	}`

	var m GPUMetricsInfo
	err := json.Unmarshal([]byte(input), &m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.GPU != 0 {
		t.Errorf("GPU: got %d, want 0", m.GPU)
	}
	if m.Power.SocketPower.Value != 200.0 {
		t.Errorf("SocketPower: got %f, want 200.0", m.Power.SocketPower.Value)
	}
	if m.PCIE.Bandwidth.Value != 31508.0 {
		t.Errorf("Bandwidth: got %f, want 31508.0", m.PCIE.Bandwidth.Value)
	}
	if len(m.XGMILink) != 0 {
		t.Errorf("XGMILink: got %d links, want 0 (not present)", len(m.XGMILink))
	}
}

func TestGPUMetricsInfoWithXGMI(t *testing.T) {
	input := `{
		"gpu": 0,
		"power": {
			"socket_power": {"value": 200.0, "unit": "W"},
			"gfx_voltage": {"value": 1.2, "unit": "V"},
			"soc_voltage": {"value": 0.85, "unit": "V"},
			"mem_voltage": {"value": "N/A", "unit": "V"},
			"throttle_status": "UNTHROTTLED",
			"power_management": "ENABLED"
		},
		"pcie": {
			"width": "16",
			"speed": {"value": 16.0, "unit": "GT/s"},
			"bandwidth": {"value": 31508.0, "unit": "Mb/s"},
			"replay_count": "0",
			"l0_to_recovery_count": "N/A",
			"replay_roll_over_count": "N/A",
			"nak_sent_count": "N/A",
			"nak_received_count": "N/A",
			"current_bandwidth_sent": "N/A",
			"current_bandwidth_received": "N/A",
			"max_packet_size": "N/A",
			"lc_perf_other_end_recovery": "N/A"
		},
		"xgmi_link": [
			{
				"link": "0",
				"bdf": "0000:0c:00.0",
				"bit_rate": {"value": 36, "unit": "Gb/s"},
				"max_bandwidth": {"value": 36864, "unit": "MB/s"},
				"read": {"value": 512.5, "unit": "KB/s"},
				"write": {"value": 1024.0, "unit": "KB/s"}
			},
			{
				"link": "1",
				"bdf": "0000:43:00.0",
				"bit_rate": {"value": 36, "unit": "Gb/s"},
				"max_bandwidth": {"value": 36864, "unit": "MB/s"},
				"read": {"value": 0, "unit": "KB/s"},
				"write": {"value": 256.0, "unit": "KB/s"}
			}
		]
	}`

	var m GPUMetricsInfo
	err := json.Unmarshal([]byte(input), &m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.GPU != 0 {
		t.Errorf("GPU: got %d, want 0", m.GPU)
	}
	if len(m.XGMILink) != 2 {
		t.Fatalf("XGMILink: got %d links, want 2", len(m.XGMILink))
	}
	if m.XGMILink[0].Link != "0" {
		t.Errorf("Link[0].Link: got %q, want \"0\"", m.XGMILink[0].Link)
	}
	if m.XGMILink[0].Read.Value != 512.5 {
		t.Errorf("Link[0].Read: got %f, want 512.5", m.XGMILink[0].Read.Value)
	}
	if m.XGMILink[0].Write.Value != 1024.0 {
		t.Errorf("Link[0].Write: got %f, want 1024.0", m.XGMILink[0].Write.Value)
	}
	if m.XGMILink[1].Read.Value != 0 {
		t.Errorf("Link[1].Read: got %f, want 0", m.XGMILink[1].Read.Value)
	}
	if m.XGMILink[0].BitRate.Value != 36 {
		t.Errorf("Link[0].BitRate: got %f, want 36", m.XGMILink[0].BitRate.Value)
	}
	if m.XGMILink[0].MaxBandwidth.Value != 36864 {
		t.Errorf("Link[0].MaxBandwidth: got %f, want 36864", m.XGMILink[0].MaxBandwidth.Value)
	}
}

func TestXGMILinkMetricsWithNA(t *testing.T) {
	input := `{
		"link": "2",
		"bdf": "N/A",
		"bit_rate": "N/A",
		"max_bandwidth": "N/A",
		"read": "N/A",
		"write": "N/A"
	}`

	var link XGMILinkMetrics
	err := json.Unmarshal([]byte(input), &link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link.Link != "2" {
		t.Errorf("Link: got %q, want \"2\"", link.Link)
	}
	if link.Read.Value != 0 {
		t.Errorf("Read: got %f, want 0 (N/A)", link.Read.Value)
	}
	if link.Write.Value != 0 {
		t.Errorf("Write: got %f, want 0 (N/A)", link.Write.Value)
	}
}

