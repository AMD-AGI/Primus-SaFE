package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// NewTimer ...
// new一个timer且注册到普罗米修斯，请注意，metricName在一个进程内不可以重复，否则panic
// metricName是指标名字，确保一个进程内唯一性
// help是描述指标用途
// labels 是维度

// NewTimer
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

// Timer 返回一个函数，并且开始计时，结束计时则调用返回的函数
// 请参考timer_test.go 的demo
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

// Observe ：传入duration和labels，
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
