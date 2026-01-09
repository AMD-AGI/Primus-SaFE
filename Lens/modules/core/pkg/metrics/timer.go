// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// NewTimer creates a new timer and registers it to Prometheus.
// Note: metricName must be unique within a process, otherwise it will panic
// metricName is the metric name, must be unique within a process
// help describes the purpose of the metric
// labels are the dimensions
func NewTimer(metricName, help string, labels []string, opts ...OptsFunc) *Timer {
	opt := &mOpts{
		name:     metricName,
		help:     help,
		quantile: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		buckets:  []float64{.0001, .0005, .001, .005, .01, .025, .05, .1, .5, 1, 2.5, 5, 10, 60, 600, 3600},
	}

	for _, optFunc := range opts {
		optFunc(opt)
	}

	var summary *prometheus.SummaryVec
	// summary
	summary = prometheus.NewSummaryVec(
		opt.GetSummaryOpts(),
		labels)

	prometheus.MustRegister(summary)

	// histogram

	histogram := prometheus.NewHistogramVec(opt.GetHistogramOpts(), labels)

	prometheus.MustRegister(histogram)
	return &Timer{
		name:      metricName,
		summary:   summary,
		histogram: histogram,
	}
}

type Timer struct {
	name      string
	summary   *prometheus.SummaryVec
	histogram *prometheus.HistogramVec
}

// Timer returns a function and starts timing. To end timing, call the returned function.
// Please refer to the demo in timer_test.go
func (t *Timer) Timer() func(values ...string) {
	if t == nil {
		return func(values ...string) {}
	}

	now := time.Now()

	return func(values ...string) {
		seconds := float64(time.Since(now)) / float64(time.Second)
		if t.summary != nil {
			t.summary.WithLabelValues(values...).Observe(seconds)
		}
		t.histogram.WithLabelValues(values...).Observe(seconds)
	}
}

// Observe takes duration and labels as parameters
func (t *Timer) Observe(duration time.Duration, label ...string) {
	if t == nil {
		return
	}

	seconds := float64(duration) / float64(time.Second)
	if t.summary != nil {
		t.summary.WithLabelValues(label...).Observe(seconds)
	}
	t.histogram.WithLabelValues(label...).Observe(seconds)
}
