// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package example

import (
	"bytes"
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/metrics"
	"net/http"
)

var (
	sampleCounter   *metrics.CounterVec
	sampleGauge     *metrics.GaugeVec
	sampleHistogram *metrics.HistogramVec
	sampleTimer     *metrics.Timer // Timer contains a Summary and a Histogram
)

func Init() {
	// Initializing Timer without metrics.WithBuckets and metrics.WithQuantile will use default values {.0001, .0005, .001, .005, .01, .025, .05, .1, .5, 1, 2.5, 5, 10, 60, 600, 3600} and {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	sampleTimer = metrics.NewTimer("test_timer", "test timer", []string{"http_code"}, metrics.WithBuckets([]float64{0.1, 0.5, 1, 2, 5, 10}), metrics.WithQuantile(map[float64]float64{
		0.99: 0.01,
		0.9:  0.1,
		0.5:  0.5,
	}))
	// Counter is a monotonically increasing counter
	sampleCounter = metrics.NewCounterVec("test_counter", "test counter", []string{"http_code"})
	// Gauge is a counter that can increase or decrease
	sampleGauge = metrics.NewGaugeVec("test_gauge", "test gauge", []string{"http_code"})
	// Histogram is actually contained in Timer, standalone Histogram can be used to measure non-time units such as request size
	sampleHistogram = metrics.NewHistogramVec("test_histogram", "test histogram", []string{"http_code"}, metrics.WithBuckets([]float64{0.1, 0.5, 1, 2, 5, 10}))
}

func Record() {
	req, err := http.NewRequestWithContext(context.Background(), "GET", "http://www.baidu.com", nil)
	if err != nil {
		panic(err)
	}
	t := sampleTimer.Timer() // Returns a function, calling it again directly measures the duration
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t("err")
		sampleHistogram.Observe(0, "err")
		sampleCounter.Inc("err")
		sampleGauge.Add(1, "err")
		return
	}
	defer resp.Body.Close()
	bodyBuffer := &bytes.Buffer{}
	size, err := bodyBuffer.ReadFrom(resp.Body)
	if err != nil {
		t("err")
		sampleHistogram.Observe(0, "err")
		sampleCounter.Inc("err")
		sampleGauge.Add(1, "err")
		return
	}
	t(resp.Status)
	sampleHistogram.Observe(float64(size), resp.Status) // Only observe Histogram type
	sampleCounter.Inc(resp.Status)
	sampleGauge.Add(1, resp.Status)
}
