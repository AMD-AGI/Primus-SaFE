// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rdmaflow

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var testLabels = []string{"node", "qpn", "pid", "device", "source"}

func newTestHandler() *Handler {
	return &Handler{
		seenSeries: make(map[seriesKey]struct{}),
		txBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_rdma_qp_tx_bytes",
			Help: "test",
		}, testLabels),
		txOps: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_rdma_qp_tx_ops",
			Help: "test",
		}, testLabels),
		nodeName: "test-node",
	}
}

func gaugeCount(g *prometheus.GaugeVec) int {
	ch := make(chan prometheus.Metric, 100)
	g.Collect(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	return count
}

func gaugeValue(g *prometheus.GaugeVec, labels ...string) float64 {
	m := g.WithLabelValues(labels...)
	var metric dto.Metric
	m.(prometheus.Metric).Write(&metric)
	return metric.GetGauge().GetValue()
}

func TestCleanInactiveQPs_SameQPNDifferentPID(t *testing.T) {
	h := newTestHandler()

	h.seenSeries[seriesKey{qpn: "10", pid: "100", device: "", source: "uprobe_provider"}] = struct{}{}
	h.seenSeries[seriesKey{qpn: "10", pid: "200", device: "", source: "uprobe_provider"}] = struct{}{}
	h.txOps.WithLabelValues("test-node", "10", "100", "", "uprobe_provider").Set(5)
	h.txOps.WithLabelValues("test-node", "10", "200", "", "uprobe_provider").Set(3)

	active := map[FlowKey]struct{}{
		{PID: 200, QPN: 10}: {},
	}
	h.CleanInactiveQPs(active)

	if len(h.seenSeries) != 1 {
		t.Fatalf("expected 1 surviving series, got %d", len(h.seenSeries))
	}
	if gaugeCount(h.txOps) != 1 {
		t.Fatalf("expected 1 gauge series, got %d", gaugeCount(h.txOps))
	}
}

func TestCleanInactiveQPs_EmptyActiveDeletesAll(t *testing.T) {
	h := newTestHandler()

	h.seenSeries[seriesKey{qpn: "10", pid: "100", device: "", source: "uprobe_provider"}] = struct{}{}
	h.seenSeries[seriesKey{qpn: "20", pid: "200", device: "", source: "uprobe_provider"}] = struct{}{}
	h.txOps.WithLabelValues("test-node", "10", "100", "", "uprobe_provider").Set(10)
	h.txOps.WithLabelValues("test-node", "20", "200", "", "uprobe_provider").Set(20)

	h.CleanInactiveQPs(map[FlowKey]struct{}{})

	if len(h.seenSeries) != 0 {
		t.Fatalf("expected 0 series, got %d", len(h.seenSeries))
	}
	if gaugeCount(h.txOps) != 0 {
		t.Fatalf("expected 0 gauge series, got %d", gaugeCount(h.txOps))
	}
}

func TestCleanInactiveQPs_FullActiveDeletesNone(t *testing.T) {
	h := newTestHandler()

	h.seenSeries[seriesKey{qpn: "10", pid: "100", device: "", source: "uprobe_provider"}] = struct{}{}
	h.seenSeries[seriesKey{qpn: "20", pid: "200", device: "", source: "uprobe_provider"}] = struct{}{}
	h.txOps.WithLabelValues("test-node", "10", "100", "", "uprobe_provider").Set(10)
	h.txOps.WithLabelValues("test-node", "20", "200", "", "uprobe_provider").Set(20)

	active := map[FlowKey]struct{}{
		{PID: 100, QPN: 10}: {},
		{PID: 200, QPN: 20}: {},
	}
	h.CleanInactiveQPs(active)

	if len(h.seenSeries) != 2 {
		t.Fatalf("expected 2 series preserved, got %d", len(h.seenSeries))
	}
	if gaugeCount(h.txOps) != 2 {
		t.Fatalf("expected 2 gauge series preserved, got %d", gaugeCount(h.txOps))
	}
}
