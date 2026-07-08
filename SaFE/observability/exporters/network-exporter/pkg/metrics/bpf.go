// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	BpfEventChanDrop = NewCounterVec("network_exporter_bpf_event_chan_drop", "BPF event channel drop", []string{"type"})
	BpfEventRecv     = NewCounterVec("network_exporter_bpf_event_recv", "BPF event received", []string{"type"})
)

type CounterVecWrapper struct {
	counter *prometheus.CounterVec
}

func NewCounterVec(name, help string, labels []string) *CounterVecWrapper {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: help,
		},
		labels,
	)
	prometheus.MustRegister(counter)
	return &CounterVecWrapper{counter: counter}
}

func (c *CounterVecWrapper) Inc(labelValue string) {
	c.counter.WithLabelValues(labelValue).Inc()
}

func (c *CounterVecWrapper) Add(labelValue string, value float64) {
	c.counter.WithLabelValues(labelValue).Add(value)
}
