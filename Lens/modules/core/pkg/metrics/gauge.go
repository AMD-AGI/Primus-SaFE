// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import "github.com/prometheus/client_golang/prometheus"

type GaugeVec struct {
	gauges *prometheus.GaugeVec
}

func (self *GaugeVec) Describe(descs chan<- *prometheus.Desc) {
	self.gauges.Describe(descs)
}

func (self *GaugeVec) Collect(metrics chan<- prometheus.Metric) {
	self.gauges.Collect(metrics)
}

func NewGaugeVec(metricsName, help string, labels []string, opts ...OptsFunc) *GaugeVec {
	opt := &mOpts{
		name: metricsName,
		help: help,
	}
	for _, optsFunc := range opts {
		optsFunc(opt)
	}
	gaugeOpt := opt.GetGaugeOpts()
	cc := prometheus.NewGaugeVec(gaugeOpt, labels)

	prometheus.MustRegister(cc)

	return &GaugeVec{
		gauges: cc,
	}
}

func (self *GaugeVec) Inc(labels ...string) {
	self.gauges.WithLabelValues(labels...).Inc()
}

func (self *GaugeVec) Add(v float64, labels ...string) {
	self.gauges.WithLabelValues(labels...).Add(v)
}

func (self *GaugeVec) Dec(labels ...string) {
	self.gauges.WithLabelValues(labels...).Dec()
}

func (self *GaugeVec) Sub(v float64, labels ...string) {
	self.gauges.WithLabelValues(labels...).Sub(v)
}

func (self *GaugeVec) Set(v float64, labels ...string) {
	self.gauges.WithLabelValues(labels...).Set(v)
}

func (self *GaugeVec) Delete(labels ...string) {
	self.gauges.DeleteLabelValues(labels...)
}
