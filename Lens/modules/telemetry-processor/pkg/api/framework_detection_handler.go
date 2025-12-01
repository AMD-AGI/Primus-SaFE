package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	advisorCommon "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/logs"
)

// UpdateDetectionRequest 更新检测结果请求
type UpdateDetectionRequest struct {
	Source     string                 `json:"source" binding:"required"`
	Framework  string                 `json:"framework" binding:"required"`
	Type       string                 `json:"type"`
	Confidence float64                `json:"confidence" binding:"min=0,max=1"`
	Evidence   map[string]interface{} `json:"evidence"`
}

// BatchRequest 批量查询请求
type BatchRequest struct {
	WorkloadUIDs []string `json:"workload_uids" binding:"required"`
}

// contains 检查字符串是否在切片中
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ========== 框架检测 API 代理函数（转发到 AI Advisor）==========
// 所有框架检测相关的请求都转发到 ai-advisor 服务处理

// GetFrameworkDetection 查询框架检测结果（转发到 AI Advisor）
func GetFrameworkDetection(c *gin.Context) {
	workloadUID := c.Param("uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	// Get AI Advisor client
	aiAdvisor := logs.GetAIAdvisorClient()
	if aiAdvisor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "AI Advisor client not initialized",
		})
		return
	}

	// Query from AI Advisor
	detection, err := aiAdvisor.GetDetection(workloadUID)
	if err != nil {
		logrus.Errorf("Failed to query detection from AI Advisor for workload %s: %v", workloadUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get detection from AI Advisor",
		})
		return
	}

	if detection == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":        "workload not found",
			"workload_uid": workloadUID,
		})
		return
	}

	// Return detection result
	c.JSON(http.StatusOK, detection)
}

// UpdateFrameworkDetection 更新框架检测结果（转发到 AI Advisor）
func UpdateFrameworkDetection(c *gin.Context) {
	workloadUID := c.Param("uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	// Get AI Advisor client
	aiAdvisor := logs.GetAIAdvisorClient()
	if aiAdvisor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "AI Advisor client not initialized",
		})
		return
	}

	// Parse request
	var req UpdateDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	// Validate framework name
	validFrameworks := []string{
		"primus",
		"deepspeed",
		"megatron",
		"pytorch",
		"tensorflow",
		"jax",
		"unknown",
	}
	if !contains(validFrameworks, req.Framework) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":              "invalid framework",
			"valid_frameworks":   validFrameworks,
			"provided_framework": req.Framework,
		})
		return
	}

	// Default values
	if req.Type == "" {
		req.Type = "training"
	}
	if req.Confidence == 0 {
		req.Confidence = 1.0 // User annotation defaults to 1.0
	}
	if req.Evidence == nil {
		req.Evidence = make(map[string]interface{})
	}

	// Add timestamp
	req.Evidence["updated_at"] = time.Now().Format(time.RFC3339)

	// Report to AI Advisor
	advisorReq := &advisorCommon.DetectionRequest{
		WorkloadUID: workloadUID,
		Source:      req.Source,
		Framework:   req.Framework,
		Type:        req.Type,
		Confidence:  req.Confidence,
		Evidence:    req.Evidence,
	}

	resp, err := aiAdvisor.ReportDetection(advisorReq)
	if err != nil {
		logrus.Errorf("Failed to report detection to AI Advisor for workload %s: %v", workloadUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to report detection to AI Advisor",
			"details": err.Error(),
		})
		return
	}

	logrus.Infof("Detection reported to AI Advisor for workload %s: framework=%s, source=%s",
		workloadUID, req.Framework, req.Source)

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"detection": resp,
	})
}

// GetFrameworkDetectionBatch 批量查询框架检测结果（转发到 AI Advisor）
func GetFrameworkDetectionBatch(c *gin.Context) {
	var req BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	if len(req.WorkloadUIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uids is required and must not be empty",
		})
		return
	}

	// Limit batch size
	if len(req.WorkloadUIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "batch size cannot exceed 100",
		})
		return
	}

	// Get AI Advisor client
	aiAdvisor := logs.GetAIAdvisorClient()
	if aiAdvisor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "AI Advisor client not initialized",
		})
		return
	}

	// Batch query from AI Advisor
	advisorReq := &advisorCommon.BatchDetectionRequest{
		WorkloadUIDs: req.WorkloadUIDs,
	}

	batchResp, err := aiAdvisor.GetDetectionBatch(advisorReq)
	if err != nil {
		logrus.Errorf("Failed to batch query detections from AI Advisor: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to batch query from AI Advisor",
		})
		return
	}

	// Convert response format (if needed)
	results := make([]map[string]interface{}, 0, len(batchResp.Results))
	for _, result := range batchResp.Results {
		r := map[string]interface{}{
			"workload_uid": result.WorkloadUID,
		}
		if result.Error != "" {
			r["error"] = result.Error
		} else {
			r["detection"] = result.Detection
		}
		results = append(results, r)
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   len(results),
	})
}

// GetFrameworkDetectionStats 获取统计信息（转发到 AI Advisor）
func GetFrameworkDetectionStats(c *gin.Context) {
	// Parse query parameters
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")
	namespace := c.Query("namespace")

	// Get AI Advisor client
	aiAdvisor := logs.GetAIAdvisorClient()
	if aiAdvisor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "AI Advisor client not initialized",
		})
		return
	}

	// Query stats from AI Advisor
	stats, err := aiAdvisor.GetDetectionStats(startTime, endTime, namespace)
	if err != nil {
		logrus.Errorf("Failed to get statistics from AI Advisor: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get statistics from AI Advisor",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
