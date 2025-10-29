package example

import (
	"bytes"
	"context"
	"github.com/AMD-AGI/primus-lens/core/pkg/metrics"
	"net/http"
)

var (
	sampleCounter   *metrics.CounterVec
	sampleGauge     *metrics.GaugeVec
	sampleHistogram *metrics.HistogramVec
	sampleTimer     *metrics.Timer // Timer里包含了一个Summary和一个Histogram
)

func Init() {
	// 初始化Timer可以不使用 metrics.WithBuckets和metrics.WithQuantile，会有默认值{.0001, .0005, .001, .005, .01, .025, .05, .1, .5, 1, 2.5, 5, 10, 60, 600, 3600}和0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	sampleTimer = metrics.NewTimer("test_timer", "test timer", []string{"http_code"}, metrics.WithBuckets([]float64{0.1, 0.5, 1, 2, 5, 10}), metrics.WithQuantile(map[float64]float64{
		0.99: 0.01,
		0.9:  0.1,
		0.5:  0.5,
	}))
	// Counter是一个单调递增的计数器
	sampleCounter = metrics.NewCounterVec("test_counter", "test counter", []string{"http_code"})
	// Gauge是一个可增可减的计数器
	sampleGauge = metrics.NewGaugeVec("test_gauge", "test gauge", []string{"http_code"})
	// Histogram实际上包含在Timer中，单独的Histogram可以用来计量一些非时间单位，比如请求大小等
	sampleHistogram = metrics.NewHistogramVec("test_histogram", "test histogram", []string{"http_code"}, metrics.WithBuckets([]float64{0.1, 0.5, 1, 2, 5, 10}))
}

func Record() {
	req, err := http.NewRequestWithContext(context.Background(), "GET", "http://www.baidu.com", nil)
	if err != nil {
		panic(err)
	}
	t := sampleTimer.Timer() // 返回一个函数，再次调用可以直接计量使用时长
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
	sampleHistogram.Observe(float64(size), resp.Status) // 仅Observe Histogram类型
	sampleCounter.Inc(resp.Status)
	sampleGauge.Add(1, resp.Status)
}
