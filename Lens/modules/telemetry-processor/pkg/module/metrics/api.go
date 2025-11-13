package metrics

import (
	"io"
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/pods"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/snappy"
)

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

// SetDebugConfig 设置调试配置
// POST /api/v1/metrics/debug/config
// Body: {"enabled": true, "metric_pattern": "gpu_.*", "label_selectors": {"pod": "test-pod-*"}, "max_records": 1000}
func SetDebugConfigHandler(ctx *gin.Context) {
	var config DebugConfig
	if err := ctx.ShouldBindJSON(&config); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	SetDebugConfig(&config)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, gin.H{
		"message": "Debug config updated successfully",
		"config":  config,
	}))
}

// GetDebugConfigHandler 获取当前调试配置
// GET /api/v1/metrics/debug/config
func GetDebugConfigHandler(ctx *gin.Context) {
	config := GetDebugConfig()
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, config))
}

// GetDebugRecordsHandler 获取调试记录
// GET /api/v1/metrics/debug/records
func GetDebugRecordsHandler(ctx *gin.Context) {
	records, stats := GetDebugRecords()
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, gin.H{
		"records": records,
		"stats":   stats,
	}))
}

// ClearDebugRecordsHandler 清空调试记录
// DELETE /api/v1/metrics/debug/records
func ClearDebugRecordsHandler(ctx *gin.Context) {
	ClearDebugRecords()
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, gin.H{
		"message": "Debug records cleared successfully",
	}))
}

// DisableDebugHandler 快速关闭调试
// POST /api/v1/metrics/debug/disable
func DisableDebugHandler(ctx *gin.Context) {
	config := GetDebugConfig()
	config.Enabled = false
	SetDebugConfig(config)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, gin.H{
		"message": "Debug disabled successfully",
	}))
}
