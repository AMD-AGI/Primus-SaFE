package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
)

// FrameworkDetectionHandler 框架检测查询 API 处理器
type FrameworkDetectionHandler struct {
	detectionManager *framework.FrameworkDetectionManager
}

// NewFrameworkDetectionHandler 创建处理器
func NewFrameworkDetectionHandler(
	detectionMgr *framework.FrameworkDetectionManager,
) *FrameworkDetectionHandler {
	return &FrameworkDetectionHandler{
		detectionManager: detectionMgr,
	}
}

// RegisterRoutes 注册路由
func (h *FrameworkDetectionHandler) RegisterRoutes(router *gin.RouterGroup) {
	// 框架检测相关路由
	workloads := router.Group("/workloads")
	{
		// 单个查询
		workloads.GET("/:uid/framework-detection", h.GetDetection)

		// 手动标注/更新
		workloads.POST("/:uid/framework-detection", h.UpdateDetection)

		// 批量查询
		workloads.POST("/framework-detection/batch", h.GetDetectionBatch)
	}

	// 统计信息
	detection := router.Group("/framework-detection")
	{
		detection.GET("/stats", h.GetStats)
	}
}

// 全局 handler 实例
var globalDetectionHandler *FrameworkDetectionHandler

// InitFrameworkDetectionHandler 初始化全局 handler
func InitFrameworkDetectionHandler(detectionMgr *framework.FrameworkDetectionManager) {
	globalDetectionHandler = NewFrameworkDetectionHandler(detectionMgr)
}

// RegisterFrameworkDetectionRoutes 注册框架检测 API 路由（供 bootstrap 调用）
func RegisterFrameworkDetectionRoutes(router *gin.RouterGroup) {
	if globalDetectionHandler == nil {
		logrus.Warn("Framework detection handler not initialized, skipping route registration")
		return
	}
	
	globalDetectionHandler.RegisterRoutes(router)
	logrus.Info("Framework detection API routes registered")
}

// ========== 全局路由处理函数（供 bootstrap 直接调用）==========

// GetFrameworkDetection 获取框架检测结果（全局函数）
func GetFrameworkDetection(c *gin.Context) {
	if globalDetectionHandler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "framework detection service not initialized",
		})
		return
	}
	globalDetectionHandler.GetDetection(c)
}

// UpdateFrameworkDetection 更新框架检测结果（全局函数）
func UpdateFrameworkDetection(c *gin.Context) {
	if globalDetectionHandler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "framework detection service not initialized",
		})
		return
	}
	globalDetectionHandler.UpdateDetection(c)
}

// GetFrameworkDetectionBatch 批量查询框架检测结果（全局函数）
func GetFrameworkDetectionBatch(c *gin.Context) {
	if globalDetectionHandler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "framework detection service not initialized",
		})
		return
	}
	globalDetectionHandler.GetDetectionBatch(c)
}

// GetFrameworkDetectionStats 获取统计信息（全局函数）
func GetFrameworkDetectionStats(c *gin.Context) {
	if globalDetectionHandler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "framework detection service not initialized",
		})
		return
	}
	globalDetectionHandler.GetStats(c)
}

// GetDetection 获取框架检测结果
//
// GET /api/v1/workloads/{uid}/framework-detection
//
// Response 200:
//
//	{
//	  "workload_uid": "workload-abc-123",
//	  "framework": "primus",
//	  "type": "training",
//	  "confidence": 0.95,
//	  "status": "verified",
//	  "sources": [...],
//	  "conflicts": [],
//	  "reuse_info": {...},
//	  "updated_at": "2025-11-24T10:00:05Z"
//	}
//
// Response 404:
//
//	{
//	  "error": "workload not found",
//	  "workload_uid": "workload-abc-123"
//	}
func (h *FrameworkDetectionHandler) GetDetection(c *gin.Context) {
	workloadUID := c.Param("uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	// 查询检测结果
	detection, err := h.detectionManager.GetDetection(c.Request.Context(), workloadUID)
	if err != nil {
		logrus.Errorf("Failed to get detection for workload %s: %v", workloadUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get detection",
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

	// 返回检测结果
	c.JSON(http.StatusOK, detection)
}

// UpdateDetectionRequest 更新检测结果请求
type UpdateDetectionRequest struct {
	Source     string                 `json:"source" binding:"required"`
	Framework  string                 `json:"framework" binding:"required"`
	Type       string                 `json:"type"`
	Confidence float64                `json:"confidence" binding:"min=0,max=1"`
	Evidence   map[string]interface{} `json:"evidence"`
}

// UpdateDetection 手动标注/更新框架检测结果
//
// POST /api/v1/workloads/{uid}/framework-detection
//
// Request:
//
//	{
//	  "source": "user",
//	  "framework": "primus",
//	  "type": "training",
//	  "confidence": 1.0,
//	  "evidence": {
//	    "method": "manual_annotation",
//	    "user": "admin@example.com",
//	    "reason": "确认为 Primus 训练任务",
//	    "annotated_at": "2025-11-24T10:30:00Z"
//	  }
//	}
//
// Response 200:
//
//	{
//	  "success": true,
//	  "detection": {...}
//	}
//
// Response 400:
//
//	{
//	  "error": "invalid request",
//	  "details": "framework must be one of: primus, deepspeed, megatron, pytorch, tensorflow"
//	}
func (h *FrameworkDetectionHandler) UpdateDetection(c *gin.Context) {
	workloadUID := c.Param("uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	// 解析请求
	var req UpdateDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	// 验证框架名称
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
			"error":             "invalid framework",
			"valid_frameworks":  validFrameworks,
			"provided_framework": req.Framework,
		})
		return
	}

	// 默认值处理
	if req.Type == "" {
		req.Type = "training"
	}
	if req.Confidence == 0 {
		req.Confidence = 1.0 // 用户标注默认置信度为 1.0
	}
	if req.Evidence == nil {
		req.Evidence = make(map[string]interface{})
	}

	// 添加时间戳
	req.Evidence["updated_at"] = time.Now().Format(time.RFC3339)

	// 上报检测
	err := h.detectionManager.ReportDetection(
		c.Request.Context(),
		workloadUID,
		req.Source,
		req.Framework,
		req.Type,
		req.Confidence,
		req.Evidence,
	)

	if err != nil {
		logrus.Errorf("Failed to report detection for workload %s: %v", workloadUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to report detection",
			"details": err.Error(),
		})
		return
	}

	logrus.Infof("Manual detection reported for workload %s: framework=%s, source=%s",
		workloadUID, req.Framework, req.Source)

	// 返回更新后的结果
	detection, _ := h.detectionManager.GetDetection(c.Request.Context(), workloadUID)

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"detection": detection,
	})
}

// BatchRequest 批量查询请求
type BatchRequest struct {
	WorkloadUIDs []string `json:"workload_uids" binding:"required"`
}

// GetDetectionBatch 批量查询框架检测结果
//
// POST /api/v1/workloads/framework-detection/batch
//
// Request:
//
//	{
//	  "workload_uids": [
//	    "workload-abc-123",
//	    "workload-def-456",
//	    "workload-ghi-789"
//	  ]
//	}
//
// Response 200:
//
//	{
//	  "results": [
//	    {
//	      "workload_uid": "workload-abc-123",
//	      "detection": {...}
//	    },
//	    {
//	      "workload_uid": "workload-def-456",
//	      "detection": {...}
//	    },
//	    {
//	      "workload_uid": "workload-ghi-789",
//	      "error": "not found"
//	    }
//	  ]
//	}
func (h *FrameworkDetectionHandler) GetDetectionBatch(c *gin.Context) {
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

	// 限制批量查询数量
	if len(req.WorkloadUIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "batch size cannot exceed 100",
		})
		return
	}

	results := make([]map[string]interface{}, 0, len(req.WorkloadUIDs))

	for _, uid := range req.WorkloadUIDs {
		detection, err := h.detectionManager.GetDetection(c.Request.Context(), uid)

		result := map[string]interface{}{
			"workload_uid": uid,
		}

		if err != nil {
			result["error"] = err.Error()
		} else if detection == nil {
			result["error"] = "not found"
		} else {
			result["detection"] = detection
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   len(results),
	})
}

// GetStats 获取框架检测统计信息
//
// GET /api/v1/framework-detection/stats
//
// Query Parameters:
//   - start_time: ISO8601 timestamp (optional)
//   - end_time: ISO8601 timestamp (optional)
//   - namespace: string (optional)
//
// Response 200:
//
//	{
//	  "total_workloads": 1000,
//	  "by_framework": {
//	    "primus": 650,
//	    "deepspeed": 250,
//	    "megatron": 80,
//	    "unknown": 20
//	  },
//	  "by_status": {
//	    "verified": 800,
//	    "confirmed": 150,
//	    "suspected": 30,
//	    "unknown": 20
//	  },
//	  "by_source": {
//	    "reuse": 500,
//	    "component": 900,
//	    "log": 800,
//	    "wandb": 300,
//	    "user": 50
//	  },
//	  "average_confidence": 0.88,
//	  "conflict_rate": 0.02,
//	  "reuse_rate": 0.50
//	}
func (h *FrameworkDetectionHandler) GetStats(c *gin.Context) {
	// 解析查询参数
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")
	namespace := c.Query("namespace")

	// 调用统计方法
	stats, err := h.detectionManager.GetStatistics(
		c.Request.Context(),
		startTime,
		endTime,
		namespace,
	)

	if err != nil {
		logrus.Errorf("Failed to get statistics: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get statistics",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, stats)
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

