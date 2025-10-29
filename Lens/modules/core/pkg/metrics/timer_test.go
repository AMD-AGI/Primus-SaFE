package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prometheus_pb "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/suite"
)

type testSuite struct {
	suite.Suite
	timer  *Timer
	timer2 *Timer
}

func TestSuite(t *testing.T) {
	timer := NewTimer("http_request", "help", []string{"http_code", "url"}, WithBuckets([]float64{0.1, 1, 5, 10, 50, 100}), WithQuantile(map[float64]float64{0.1: 0.01, 0.5: 0.01, 0.7: 0.01, 0.9: 0.001, 0.99: 0.001}))
	timer2 := NewTimer("http_request2", "help", []string{"http_code", "url"})
	ts := &testSuite{
		timer:  timer,
		timer2: timer2,
	}

	suite.Run(t, ts)
}

func (self *testSuite) TestHistogram() {

	self.mockHttpHandleObserve(self.timer, 150*time.Millisecond)

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		self.NoError(err)
	}

	self.NotEmpty(mfs)

	var bbb []*prometheus_pb.Bucket
	for _, mf := range mfs {
		if mf.GetName() == "http_request_h" {
			for _, mm := range mf.GetMetric() {
				histo := mm.GetHistogram()
				if histo != nil {
					bbb = histo.GetBucket()
				}
			}
		}
	}

	self.NotEmpty(bbb)

	for _, bb := range bbb {
		if bb.GetUpperBound() == 0.1 {
			self.Equal(uint64(0), bb.GetCumulativeCount())
		} else {
			self.Equal(uint64(1), bb.GetCumulativeCount())
		}
	}

}

func (self *testSuite) TestSummary() {

	self.mockHttpHandleObserve(self.timer, 200*time.Millisecond)

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		self.NoError(err)
	}

	self.NotEmpty(mfs)

	var quans []*prometheus_pb.Quantile

	for _, mf := range mfs {
		if mf.GetName() == "http_request_s" {
			for _, mm := range mf.GetMetric() {
				summ := mm.GetSummary()
				if summ != nil {
					quans = summ.GetQuantile()
				}
			}
		}
	}

	self.NotEmpty(quans)
	for _, bb := range quans {
		if bb.GetQuantile() < 0.7 {
			if bb.GetValue() > 0.2 {
				self.FailNow("if < P7 then < 0.2")
			}
		} else {
			if bb.GetValue() < 0.2 {
				self.FailNow("if > P7 then > 0.2")
			}
		}
	}
}

func (self *testSuite) TestSummaryDefault() {

	self.mockHttpHandleObserve(self.timer2, 200*time.Millisecond)

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		self.NoError(err)
	}

	self.NotEmpty(mfs)

	var quans []*prometheus_pb.Quantile

	for _, mf := range mfs {
		if mf.GetName() == "http_request2_s" {
			for _, mm := range mf.GetMetric() {
				summ := mm.GetSummary()
				if summ != nil {
					quans = summ.GetQuantile()
				}
			}
		}
	}

	self.NotEmpty(quans)
	for _, bb := range quans {
		fmt.Printf("%v  %v\n", bb.GetQuantile(), bb.GetValue())
		if bb.GetValue() < 0.2 {
			self.FailNow("< 0.2")
		}
	}

}

// 模拟一个http handle 函数
func (self *testSuite) mockHttpHandleObserve(timer *Timer, sleep time.Duration) {
	// 开始计时
	startTime := time.Now()

	// 模拟处理请求的时间
	time.Sleep(sleep)

	// 结束计时， time.Now().Sub(startTime)
	timer.Observe(time.Now().Sub(startTime), "200", "http://baidu.com")
}
