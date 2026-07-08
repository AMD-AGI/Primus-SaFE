// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

package model

import (
	"testing"
)

func TestRDMADeviceStruct(t *testing.T) {
	dev := RDMADevice{
		IfIndex:      1,
		IfName:       "mlx5_0",
		NodeType:     "IB",
		FW:           "22.36.1010",
		NodeGUID:     "b8ce:f604:001c:dae8",
		SysImageGUID: "b8ce:f604:001c:dae8",
	}

	if dev.IfIndex != 1 {
		t.Errorf("IfIndex: got %d, want 1", dev.IfIndex)
	}
	if dev.IfName != "mlx5_0" {
		t.Errorf("IfName: got %q, want mlx5_0", dev.IfName)
	}
	if dev.NodeType != "IB" {
		t.Errorf("NodeType: got %q, want IB", dev.NodeType)
	}
	if dev.FW != "22.36.1010" {
		t.Errorf("FW: got %q, want 22.36.1010", dev.FW)
	}
}

func TestRDMAStatStruct(t *testing.T) {
	stat := RDMAStat{
		Device: "mlx5_0",
		Port:   "1",
		Stats:  map[string]int64{"rx_write_requests": 100, "tx_write_requests": 200},
	}

	if stat.Device != "mlx5_0" {
		t.Errorf("Device: got %q, want mlx5_0", stat.Device)
	}
	if stat.Port != "1" {
		t.Errorf("Port: got %q, want 1", stat.Port)
	}
	if stat.Stats["rx_write_requests"] != 100 {
		t.Errorf("rx_write_requests: got %d, want 100", stat.Stats["rx_write_requests"])
	}
	if stat.Stats["tx_write_requests"] != 200 {
		t.Errorf("tx_write_requests: got %d, want 200", stat.Stats["tx_write_requests"])
	}
}

func TestRDMAStatEmptyStats(t *testing.T) {
	stat := RDMAStat{
		Device: "mlx5_0",
		Port:   "1",
		Stats:  make(map[string]int64),
	}

	if len(stat.Stats) != 0 {
		t.Errorf("expected empty stats, got %d entries", len(stat.Stats))
	}
}

