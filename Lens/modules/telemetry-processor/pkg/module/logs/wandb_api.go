package logs

import (
	"encoding/json"
	"net/http"

	advisorClient "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/client"
	advisorCommon "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// WandBHandler WandB 数据上报 API 处理器
type WandBHandler struct {
	aiAdvisorClient *advisorClient.Client
	logProcessor    *WandBLogProcessor
}

// wandbHandler 全局 handler 实例
var wandbHandler *WandBHandler

// InitWandBHandlerWithClient 初始化 WandB Handler (使用 AI Advisor client)
func InitWandBHandlerWithClient(
	aiAdvisor *advisorClient.Client,
	logProcessor *WandBLogProcessor,
) {
	wandbHandler = &WandBHandler{
		aiAdvisorClient: aiAdvisor,
		logProcessor:    logProcessor,
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
	logrus.Info("====== [WandB Detection API] Received request ======")

	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBDetectionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logrus.Errorf("[WandB Detection API] Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	// 打印请求体详情
	reqJSON, _ := json.MarshalIndent(req, "", "  ")
	logrus.Debugf("[WandB Detection API] Request body:\n%s", string(reqJSON))

	// 支持双层框架的日志输出
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		logrus.Infof("[WandB Detection API] Detection request (双层框架) - WorkloadUID: %s, PodName: %s, RunID: %s, Wrapper: %v, Base: %v",
			req.WorkloadUID, req.PodName, req.Evidence.WandB.ID, req.Hints.WrapperFrameworks, req.Hints.BaseFrameworks)
	} else {
		// 向后兼容：旧格式
		logrus.Infof("[WandB Detection API] Detection request - WorkloadUID: %s, PodName: %s, RunID: %s, PossibleFrameworks: %v",
			req.WorkloadUID, req.PodName, req.Evidence.WandB.ID, req.Hints.PossibleFrameworks)
	}

	// 验证：必须提供 workload_uid 或 pod_name
	if req.WorkloadUID == "" && req.PodName == "" {
		logrus.Warnf("[WandB Detection API] Validation failed: neither workload_uid nor pod_name provided")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "either workload_uid or pod_name is required", nil))
		return
	}

	// Forward detection request to AI Advisor
	logrus.Infof("[WandB Detection API] Forwarding detection request to AI Advisor...")

	// Convert request to AI Advisor format
	advisorReq := convertToAdvisorWandBRequest(&req)

	if err := wandbHandler.aiAdvisorClient.ReportWandBDetection(advisorReq); err != nil {
		logrus.Errorf("[WandB Detection API] Failed to forward detection to AI Advisor: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process detection", nil))
		return
	}

	// 支持双层框架的成功日志
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		logrus.Infof("[WandB Detection API] ✓ Detection processed successfully (双层框架) - Wrapper: %v, Base: %v, WorkloadUID: %s",
			req.Hints.WrapperFrameworks, req.Hints.BaseFrameworks, req.WorkloadUID)
	} else {
		logrus.Infof("[WandB Detection API] ✓ Detection processed successfully - PossibleFrameworks: %v, WorkloadUID: %s",
			req.Hints.PossibleFrameworks, req.WorkloadUID)
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "detection reported successfully",
	}))
}

// ReceiveWandBMetrics 处理指标上报
// POST /wandb/metrics
func ReceiveWandBMetrics(ctx *gin.Context) {
	logrus.Info("====== [WandB Metrics API] Received request ======")

	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBMetricsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logrus.Errorf("[WandB Metrics API] Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	// 打印请求体摘要（避免过大的日志）
	logrus.Infof("[WandB Metrics API] Metrics request - WorkloadUID: %s, PodName: %s, RunID: %s, MetricsCount: %d",
		req.WorkloadUID, req.PodName, req.RunID, len(req.Metrics))

	// 详细日志（仅在 Debug 级别）
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		reqJSON, _ := json.MarshalIndent(req, "", "  ")
		logrus.Debugf("[WandB Metrics API] Request body:\n%s", string(reqJSON))

		// 打印前几个指标示例
		sampleSize := 3
		if len(req.Metrics) < sampleSize {
			sampleSize = len(req.Metrics)
		}
		for i := 0; i < sampleSize; i++ {
			m := req.Metrics[i]
			logrus.Debugf("[WandB Metrics API] Metric[%d]: name=%s, value=%v, step=%d",
				i, m.Name, m.Value, m.Step)
		}
	}

	// 验证：必须提供 workload_uid 或 pod_name
	if req.WorkloadUID == "" && req.PodName == "" {
		logrus.Warnf("[WandB Metrics API] Validation failed: neither workload_uid nor pod_name provided")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "either workload_uid or pod_name is required", nil))
		return
	}

	logrus.Infof("[WandB Metrics API] Starting metrics processing (%d metrics)...", len(req.Metrics))
	if err := wandbHandler.logProcessor.ProcessMetrics(ctx.Request.Context(), &req); err != nil {
		logrus.Errorf("[WandB Metrics API] Failed to process metrics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process metrics", nil))
		return
	}

	logrus.Infof("[WandB Metrics API] ✓ Metrics processed successfully - Count: %d, WorkloadUID: %s",
		len(req.Metrics), req.WorkloadUID)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "metrics reported successfully",
		"count":   len(req.Metrics),
	}))
}

// ReceiveWandBLogs handles training data reporting from wandb.log()
// POST /wandb/logs
// Note: Despite the name, this endpoint receives structured training metrics
// from wandb.log(), not text logs. Data is stored in training_performance table.
func ReceiveWandBLogs(ctx *gin.Context) {
	logrus.Info("====== [WandB Logs/Training API] Received request ======")

	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBLogsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logrus.Errorf("[WandB Logs/Training API] Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	// 打印请求体摘要
	logrus.Infof("[WandB Logs/Training API] Training data request - WorkloadUID: %s, PodName: %s, RunID: %s, LogsCount: %d",
		req.WorkloadUID, req.PodName, req.RunID, len(req.Logs))

	// 详细日志（仅在 Debug 级别）
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		reqJSON, _ := json.MarshalIndent(req, "", "  ")
		logrus.Debugf("[WandB Logs/Training API] Request body:\n%s", string(reqJSON))

		// 打印前几个训练数据条目示例
		sampleSize := 3
		if len(req.Logs) < sampleSize {
			sampleSize = len(req.Logs)
		}
		for i := 0; i < sampleSize; i++ {
			l := req.Logs[i]
			logrus.Debugf("[WandB Logs/Training API] Log[%d]: step=%d, dataKeys=%v",
				i, l.Step, getMapKeys(l.Data))
		}
	}

	// 验证：必须提供 workload_uid 或 pod_name
	if req.WorkloadUID == "" && req.PodName == "" {
		logrus.Warnf("[WandB Logs/Training API] Validation failed: neither workload_uid nor pod_name provided")
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "either workload_uid or pod_name is required", nil))
		return
	}

	logrus.Infof("[WandB Logs/Training API] Starting training data processing (%d entries)...", len(req.Logs))
	if err := wandbHandler.logProcessor.ProcessLogs(ctx.Request.Context(), &req); err != nil {
		logrus.Errorf("[WandB Logs/Training API] Failed to process training data: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "failed to process logs", nil))
		return
	}

	logrus.Infof("[WandB Logs/Training API] ✓ Training data processed successfully - Count: %d, WorkloadUID: %s",
		len(req.Logs), req.WorkloadUID)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "training data reported successfully",
		"count":   len(req.Logs),
	}))
}

// ReceiveWandBBatch 批量上报
// POST /wandb/batch
func ReceiveWandBBatch(ctx *gin.Context) {
	logrus.Info("====== [WandB Batch API] Received request ======")

	if wandbHandler == nil {
		log.GlobalLogger().WithContext(ctx).Errorf("WandB handler not initialized")
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(),
			http.StatusInternalServerError, "WandB handler not initialized", nil))
		return
	}

	var req WandBBatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logrus.Errorf("[WandB Batch API] Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(),
			http.StatusBadRequest, "invalid request format", nil))
		return
	}

	// 打印批量请求摘要
	detectionCount := 0
	if req.Detection != nil {
		detectionCount = 1
	}
	metricsCount := 0
	if req.Metrics != nil {
		metricsCount = len(req.Metrics.Metrics)
	}
	logsCount := 0
	if req.Logs != nil {
		logsCount = len(req.Logs.Logs)
	}

	logrus.Infof("[WandB Batch API] Batch request summary - Detection: %d, Metrics: %d, Logs: %d",
		detectionCount, metricsCount, logsCount)

	// 详细日志（仅在 Debug 级别）
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		reqJSON, _ := json.MarshalIndent(req, "", "  ")
		logrus.Debugf("[WandB Batch API] Request body:\n%s", string(reqJSON))
	}

	result := gin.H{
		"success": true,
		"results": gin.H{},
	}

	// 处理框架检测
	if req.Detection != nil {
		// 支持双层框架的日志输出
		if len(req.Detection.Hints.WrapperFrameworks) > 0 || len(req.Detection.Hints.BaseFrameworks) > 0 {
			logrus.Infof("[WandB Batch API] Processing detection (双层框架) - Wrapper: %v, Base: %v, WorkloadUID: %s",
				req.Detection.Hints.WrapperFrameworks, req.Detection.Hints.BaseFrameworks, req.Detection.WorkloadUID)
		} else {
			logrus.Infof("[WandB Batch API] Processing detection - PossibleFrameworks: %v, WorkloadUID: %s",
				req.Detection.Hints.PossibleFrameworks, req.Detection.WorkloadUID)
		}
		// Forward to AI Advisor
		advisorReq := convertToAdvisorWandBRequest(req.Detection)
		if err := wandbHandler.aiAdvisorClient.ReportWandBDetection(advisorReq); err != nil {
			logrus.Errorf("[WandB Batch API] Failed to forward detection to AI Advisor: %v", err)
			result["results"].(gin.H)["detection"] = gin.H{
				"success": false,
				"error":   err.Error(),
			}
		} else {
			logrus.Infof("[WandB Batch API] ✓ Detection forwarded to AI Advisor successfully")
			result["results"].(gin.H)["detection"] = gin.H{
				"success": true,
			}
		}
	}

	// 处理指标
	if req.Metrics != nil {
		logrus.Infof("[WandB Batch API] Processing metrics - Count: %d, WorkloadUID: %s",
			len(req.Metrics.Metrics), req.Metrics.WorkloadUID)
		if err := wandbHandler.logProcessor.ProcessMetrics(ctx.Request.Context(), req.Metrics); err != nil {
			logrus.Errorf("[WandB Batch API] Failed to process metrics: %v", err)
			result["results"].(gin.H)["metrics"] = gin.H{
				"success": false,
				"error":   err.Error(),
			}
		} else {
			logrus.Infof("[WandB Batch API] ✓ Metrics processed successfully - Count: %d",
				len(req.Metrics.Metrics))
			result["results"].(gin.H)["metrics"] = gin.H{
				"success": true,
				"count":   len(req.Metrics.Metrics),
			}
		}
	}

	// 处理日志
	if req.Logs != nil {
		logrus.Infof("[WandB Batch API] Processing training data - Count: %d, WorkloadUID: %s",
			len(req.Logs.Logs), req.Logs.WorkloadUID)
		if err := wandbHandler.logProcessor.ProcessLogs(ctx.Request.Context(), req.Logs); err != nil {
			logrus.Errorf("[WandB Batch API] Failed to process training data: %v", err)
			result["results"].(gin.H)["logs"] = gin.H{
				"success": false,
				"error":   err.Error(),
			}
		} else {
			logrus.Infof("[WandB Batch API] ✓ Training data processed successfully - Count: %d",
				len(req.Logs.Logs))
			result["results"].(gin.H)["logs"] = gin.H{
				"success": true,
				"count":   len(req.Logs.Logs),
			}
		}
	}

	logrus.Infof("[WandB Batch API] ✓ Batch request completed - Detection: %v, Metrics: %v, Logs: %v",
		req.Detection != nil, req.Metrics != nil, req.Logs != nil)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), result))
}

// getMapKeys 获取 map 的所有键（用于日志输出）
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// convertToAdvisorWandBRequest converts telemetry-processor WandBDetectionRequest
// to ai-advisor WandBDetectionRequest format
// Note: Since structures are slightly different, we use JSON marshaling for safe conversion
func convertToAdvisorWandBRequest(req *WandBDetectionRequest) *advisorCommon.WandBDetectionRequest {
	if req == nil {
		return nil
	}

	// Simple field mapping (structures match from migration)
	// ai-advisor's WandBDetectionRequest matches the original structure
	return &advisorCommon.WandBDetectionRequest{
		Source:      req.Source,
		Type:        req.Type,
		Version:     req.Version,
		WorkloadUID: req.WorkloadUID,
		PodUID:      req.PodUID,
		PodName:     req.PodName,
		Namespace:   req.Namespace,
		Timestamp:   req.Timestamp,
		Evidence: advisorCommon.WandBEvidence{
			WandB: advisorCommon.WandBInfo{
				ID:      req.Evidence.WandB.ID,
				Name:    req.Evidence.WandB.Name,
				Project: req.Evidence.WandB.Project,
				Config:  req.Evidence.WandB.Config,
			},
			PyTorch: advisorCommon.PyTorchInfo{
				Version:       req.Evidence.PyTorch.Version,
				CudaAvailable: req.Evidence.PyTorch.CudaAvailable,
			},
		},
		Hints: advisorCommon.WandBHints{
			PossibleFrameworks: req.Hints.PossibleFrameworks,
			WrapperFrameworks:  req.Hints.WrapperFrameworks,
			BaseFrameworks:     req.Hints.BaseFrameworks,
		},
	}
}
