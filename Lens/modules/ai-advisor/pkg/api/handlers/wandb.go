package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// WandBHandler handles WandB detection requests
type WandBHandler struct {
	wandbDetector *detection.WandBFrameworkDetector
}

// NewWandBHandler creates a new WandB handler
func NewWandBHandler(detector *detection.WandBFrameworkDetector) *WandBHandler {
	return &WandBHandler{
		wandbDetector: detector,
	}
}

// ReceiveDetection handles WandB detection data upload
//
// POST /api/v1/wandb/detection
func (h *WandBHandler) ReceiveDetection(c *gin.Context) {
	log.Info("====== [AI-Advisor WandB Detection API] Received request ======")

	var req detection.WandBDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warnf("[AI-Advisor WandB Detection API] Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	// 打印请求体详情（Debug 级别）
	if reqJSON, err := json.MarshalIndent(req, "", "  "); err == nil {
		log.Debugf("[AI-Advisor WandB Detection API] Request body:\n%s", string(reqJSON))
	}

	// 支持双层框架的日志输出
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		log.Infof("[AI-Advisor WandB Detection API] Detection request (双层框架) - WorkloadUID: %s, PodName: %s, RunID: %s, Wrapper: %v, Base: %v",
			req.WorkloadUID, req.PodName, req.Evidence.WandB.ID, req.Hints.WrapperFrameworks, req.Hints.BaseFrameworks)
	} else {
		// 向后兼容：旧格式
		log.Infof("[AI-Advisor WandB Detection API] Detection request - WorkloadUID: %s, PodName: %s, RunID: %s, PossibleFrameworks: %v",
			req.WorkloadUID, req.PodName, req.Evidence.WandB.ID, req.Hints.PossibleFrameworks)
	}

	// 打印关键字段信息
	log.Debugf("[AI-Advisor WandB Detection API] Key fields - Source: %s, Type: %s, Version: %s, Namespace: %s, Confidence: %s",
		req.Source, req.Type, req.Version, req.Namespace, req.Hints.Confidence)

	// Process detection
	log.Infof("[AI-Advisor WandB Detection API] Starting detection processing...")
	err := h.wandbDetector.ProcessWandBDetection(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("[AI-Advisor WandB Detection API] Failed to process detection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to process detection",
			"details": err.Error(),
		})
		return
	}

	// 支持双层框架的成功日志
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		log.Infof("[AI-Advisor WandB Detection API] ✓ Detection processed successfully (双层框架) - Wrapper: %v, Base: %v, WorkloadUID: %s",
			req.Hints.WrapperFrameworks, req.Hints.BaseFrameworks, req.WorkloadUID)
	} else {
		log.Infof("[AI-Advisor WandB Detection API] ✓ Detection processed successfully - PossibleFrameworks: %v, WorkloadUID: %s",
			req.Hints.PossibleFrameworks, req.WorkloadUID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Detection processed successfully",
	})
}
