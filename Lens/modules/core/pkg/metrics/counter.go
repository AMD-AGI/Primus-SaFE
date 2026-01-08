// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type CounterVec struct {
	counters *prometheus.CounterVec
}

func NewCounterVec(metricsName, help string, labels []string, opts ...OptsFunc) *CounterVec {
	opt := &mOpts{
		name: metricsName,
		help: help,
	}
	for _, optsFunc := range opts {
		optsFunc(opt)
	}
	counterOpt := opt.GetCounterOpts()
	cc := prometheus.NewCounterVec(counterOpt, labels)
	prometheus.MustRegister(cc)

	return &CounterVec{
		counters: cc,
	}
}

func (self *CounterVec) Inc(labels ...string) {
	self.counters.WithLabelValues(labels...).Inc()
}

func (self *CounterVec) Add(count float64, labels ...string) {
	self.counters.WithLabelValues(labels...).Add(count)
}

func (self *CounterVec) Delete(labels ...string) {
	self.counters.DeleteLabelValues(labels...)
}
