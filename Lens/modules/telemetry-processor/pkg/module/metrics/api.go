package metrics

import (
	"io"
	"net/http"

	"github.com/AMD-AGI/primus-lens/core/pkg/constant"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/AMD-AGI/primus-lens/telemetry-processor/pkg/module/pods"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/snappy"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	testMetrics = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gpu_utilization",
	}, []string{constant.PrimusLensNodeLabelName, "gpu_id", "job", "app"})
)

func init() {
	prometheus.MustRegister(testMetrics)
	testMetrics.WithLabelValues("smc300x-ccs-aus-a17-40", "0", "primus-lens-telemetry-p1", "primus-lens-node-exporter").Set(1.0)
}

func InsertHandler(c *gin.Context) {
	compressed, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(http.StatusBadRequest, "read body failed: %v", err)
		return
	}

	data, err := snappy.Decode(nil, compressed)
	if err != nil {
		c.String(http.StatusBadRequest, "snappy decode failed: %v", err)
		return
	}

	var req prompb.WriteRequest
	if err := req.UnmarshalProtobuf(data); err != nil {
		c.String(http.StatusBadRequest, "protobuf unmarshal failed: %v", err)
		return
	}

	// Call pluggable processing logic
	if err := processTimeSeries(req.Timeseries); err != nil {
		c.String(http.StatusInternalServerError, "processing failed: %v", err)
		return
	}

	c.String(http.StatusNoContent, "ok")
}

func GetPodCache(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, pods.GetNodeDevicePodCache()))
}

func GetPodWorkloadCache(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, pods.GetPodWorkloadCache()))
}
