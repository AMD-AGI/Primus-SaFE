package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type HistogramVec struct {
	histogram *prometheus.HistogramVec
}

func NewHistogramVec(metricsName, help string, labels []string, opts ...OptsFunc) *HistogramVec {
	opt := &mOpts{
		name: metricsName,
		help: help,
	}
	for _, optsFunc := range opts {
		optsFunc(opt)
	}
	if len(opt.buckets) == 0 {
		opt.buckets = []float64{.0001, .0005, .001, .005, .01, .025, .05, .1, .5, 1, 2.5, 5, 10, 60, 600, 3600}
	}
	hisOpts := opt.GetHistogramOpts()

	histogram := prometheus.NewHistogramVec(hisOpts, labels)

	prometheus.MustRegister(histogram)

	return &HistogramVec{
		histogram: histogram,
	}
}

func (h *HistogramVec) Observe(value float64, labels ...string) {
	h.histogram.WithLabelValues(labels...).Observe(value)
}

func (h *HistogramVec) Delete(labels ...string) {
	h.histogram.DeleteLabelValues(labels...)
}
