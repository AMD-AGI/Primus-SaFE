// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
	dto "github.com/prometheus/client_model/go"
)

func TestSysfsCollectorCollect(t *testing.T) {
	root := t.TempDir()
	dev := "bnxt_re0"
	port := "1"
	hw := filepath.Join(root, dev, "ports", port, "hw_counters")
	if err := os.MkdirAll(hw, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, val := range map[string]string{
		"rx_bytes":     "12345",
		"tx_bytes":     "67890",
		"rx_pkts":      "100",
		"tx_pkts":      "200",
		"active_qps":   "4",
		"some_unknown": "999",
	} {
		if err := os.WriteFile(filepath.Join(hw, name), []byte(val+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	c := NewSysfsCollector("test-node")
	c.basePath = root
	c.Collect([]model.RDMADevice{{IfName: dev}}, map[string]string{dev: "0000:c1:00.0"})

	if len(c.metrics) == 0 {
		t.Fatal("expected metrics in c.metrics")
	}

	wantVals := map[string]float64{
		"rdma_hw_rx_bytes":   12345,
		"rdma_hw_tx_bytes":   67890,
		"rdma_hw_rx_pkts":    100,
		"rdma_hw_tx_pkts":    200,
		"rdma_hw_active_qps": 4,
	}
	for name, want := range wantVals {
		gv, ok := c.metrics[name]
		if !ok {
			t.Errorf("missing metric %q", name)
			continue
		}
		var m dto.Metric
		if err := gv.WithLabelValues("test-node", dev, "0000:c1:00.0", port).Write(&m); err != nil {
			t.Errorf("%s Write: %v", name, err)
			continue
		}
		if got := m.GetGauge().GetValue(); got != want {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
	if _, bad := c.metrics["rdma_hw_some_unknown"]; bad {
		t.Error("unexpected metric for skipped counter some_unknown")
	}
}
