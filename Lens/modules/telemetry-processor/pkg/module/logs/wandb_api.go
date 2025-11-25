package logs

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// WandBHandler WandB 数据上报 API 处理器
type WandBHandler struct {
	detector     *WandBFrameworkDetector
	logProcessor *WandBLogProcessor
}

// wandbHandler 全局 handler 实例
var wandbHandler *WandBHandler

// InitWandBHandler 初始化 WandB Handler
func InitWandBHandler(
	detector *WandBFrameworkDetector,
	logProcessor *WandBLogProcessor,
) {
	wandbHandler = &WandBHandler{
		detector:     detector,
		logProcessor: logProcessor,
	}
}

// WandBBatchRequest 批量上报请求
type WandBBatchRequest struct {
	Detection *WandBDetectionRequest `json:"detection,omitempty"`
	Metrics   *WandBMetricsRequest   `json:"metrics,omitempty"`
	Logs      *WandBLogsRequest      `json:"logs,omitempty"`
}

// ReceiveWandBDetection 处理框架检测上报
// POST /wandb/detection
func ReceiveWandBDetection(ctx *gin.Context) {
	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), 
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBDetectionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse WandB detection request: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	if req.WorkloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	if err := wandbHandler.detector.ProcessWandBDetection(ctx.Request.Context(), &req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to process WandB detection: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process detection", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "detection reported successfully",
	}))
}

// ReceiveWandBMetrics 处理指标上报
// POST /wandb/metrics
func ReceiveWandBMetrics(ctx *gin.Context) {
	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBMetricsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse WandB metrics request: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	if req.WorkloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	if err := wandbHandler.logProcessor.ProcessMetrics(ctx.Request.Context(), &req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to process WandB metrics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process metrics", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "metrics reported successfully",
		"count":   len(req.Metrics),
	}))
}

// ReceiveWandBLogs 处理日志上报
// POST /wandb/logs
func ReceiveWandBLogs(ctx *gin.Context) {
	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBLogsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse WandB logs request: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	if req.WorkloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	if err := wandbHandler.logProcessor.ProcessLogs(ctx.Request.Context(), &req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to process WandB logs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process logs", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "logs reported successfully",
		"count":   len(req.Logs),
	}))
}

// ReceiveWandBBatch 批量上报
// POST /wandb/batch
func ReceiveWandBBatch(ctx *gin.Context) {
	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBBatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse WandB batch request: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	result := gin.H{
		"success": true,
		"results": gin.H{},
	}

	// 处理框架检测
	if req.Detection != nil {
		if err := wandbHandler.detector.ProcessWandBDetection(ctx.Request.Context(), req.Detection); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to process detection in batch: %v", err)
			result["results"].(gin.H)["detection"] = gin.H{
				"success": false,
				"error":   err.Error(),
			}
		} else {
			result["results"].(gin.H)["detection"] = gin.H{
				"success": true,
			}
		}
	}

	// 处理指标
	if req.Metrics != nil {
		if err := wandbHandler.logProcessor.ProcessMetrics(ctx.Request.Context(), req.Metrics); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to process metrics in batch: %v", err)
			result["results"].(gin.H)["metrics"] = gin.H{
				"success": false,
				"error":   err.Error(),
			}
		} else {
			result["results"].(gin.H)["metrics"] = gin.H{
				"success": true,
				"count":   len(req.Metrics.Metrics),
			}
		}
	}

	// 处理日志
	if req.Logs != nil {
		if err := wandbHandler.logProcessor.ProcessLogs(ctx.Request.Context(), req.Logs); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to process logs in batch: %v", err)
			result["results"].(gin.H)["logs"] = gin.H{
				"success": false,
				"error":   err.Error(),
			}
		} else {
			result["results"].(gin.H)["logs"] = gin.H{
				"success": true,
				"count":   len(req.Logs.Logs),
			}
		}
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), result))
}

