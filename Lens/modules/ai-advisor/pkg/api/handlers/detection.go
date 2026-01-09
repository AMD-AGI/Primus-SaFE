// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package handlers

import (
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// DetectionHandler handles framework detection APIs
type DetectionHandler struct {
	detectionManager *framework.FrameworkDetectionManager
}

// NewDetectionHandler creates a new detection handler
func NewDetectionHandler(detectionMgr *framework.FrameworkDetectionManager) *DetectionHandler {
	return &DetectionHandler{
		detectionManager: detectionMgr,
	}
}

// ReportDetection handles detection reporting from any source with dual-layer framework support
//
// POST /api/v1/detection
func (h *DetectionHandler) ReportDetection(c *gin.Context) {
	var req common.DetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	// Set defaults
	if req.Type == "" {
		req.Type = "training"
	}
	if req.Evidence == nil {
		req.Evidence = make(map[string]interface{})
	}
	req.Evidence["reported_at"] = time.Now().Format(time.RFC3339)

	// Extract primary framework from Frameworks array (for compatibility with detection manager)
	var primaryFramework string
	if len(req.Frameworks) > 0 {
		primaryFramework = req.Frameworks[0]
	}

	// Report to detection manager with dual-layer framework support
	err := h.detectionManager.ReportDetectionWithLayers(
		c.Request.Context(),
		req.WorkloadUID,
		req.Source,
		primaryFramework,
		req.Type,
		req.Confidence,
		req.Evidence,
		req.FrameworkLayer,
		req.WrapperFramework,
		req.BaseFramework,
	)

	if err != nil {
		log.Errorf("Failed to report detection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to report detection",
			"details": err.Error(),
		})
		return
	}

	// Get updated detection result
	detection, err := h.detectionManager.GetDetection(c.Request.Context(), req.WorkloadUID)
	if err != nil {
		log.Errorf("Failed to get detection after report: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get detection result",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, detection)
}

// GetDetection retrieves detection result for a workload
//
// GET /api/v1/detection/workloads/:uid
func (h *DetectionHandler) GetDetection(c *gin.Context) {
	workloadUID := c.Param("uid")
	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	detection, err := h.detectionManager.GetDetection(c.Request.Context(), workloadUID)
	if err != nil {
		log.Errorf("Failed to get detection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get detection",
			"details": err.Error(),
		})
		return
	}

	if detection == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":        "detection not found",
			"workload_uid": workloadUID,
		})
		return
	}

	c.JSON(http.StatusOK, detection)
}

// BatchGetDetection retrieves detection results for multiple workloads
//
// POST /api/v1/detection/batch
func (h *DetectionHandler) BatchGetDetection(c *gin.Context) {
	type BatchRequest struct {
		WorkloadUIDs []string `json:"workload_uids" binding:"required"`
	}

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
			"error": "workload_uids cannot be empty",
		})
		return
	}

	if len(req.WorkloadUIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "batch size cannot exceed 100",
		})
		return
	}

	results := make(map[string]*common.Detection)
	for _, uid := range req.WorkloadUIDs {
		detection, err := h.detectionManager.GetDetection(c.Request.Context(), uid)
		if err != nil {
			log.Warnf("Failed to get detection for %s: %v", uid, err)
			continue
		}
		if detection != nil {
			// Convert to common.Detection with dual-layer framework info
			results[uid] = &common.Detection{
				WorkloadUID:      uid,
				Frameworks:       detection.Frameworks,
				Type:             detection.Type,
				Confidence:       detection.Confidence,
				Status:           string(detection.Status),
				UpdatedAt:        detection.UpdatedAt,
				FrameworkLayer:   detection.FrameworkLayer,
				WrapperFramework: detection.WrapperFramework,
				BaseFramework:    detection.BaseFramework,
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   len(results),
	})
}

// UpdateDetection updates detection result (manual annotation)
//
// PUT /api/v1/detection/workloads/:uid
func (h *DetectionHandler) UpdateDetection(c *gin.Context) {
	workloadUID := c.Param("uid")
	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	var req common.DetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	// Override workload UID from path
	req.WorkloadUID = workloadUID

	// Set defaults
	if req.Source == "" {
		req.Source = "manual"
	}
	if req.Type == "" {
		req.Type = "training"
	}
	if req.Confidence == 0 {
		req.Confidence = 1.0 // Manual annotation has high confidence
	}
	if req.Evidence == nil {
		req.Evidence = make(map[string]interface{})
	}
	req.Evidence["updated_at"] = time.Now().Format(time.RFC3339)

	// Extract primary framework from Frameworks array
	var primaryFramework string
	if len(req.Frameworks) > 0 {
		primaryFramework = req.Frameworks[0]
	}

	// Report detection with dual-layer framework support
	err := h.detectionManager.ReportDetectionWithLayers(
		c.Request.Context(),
		req.WorkloadUID,
		req.Source,
		primaryFramework,
		req.Type,
		req.Confidence,
		req.Evidence,
		req.FrameworkLayer,
		req.WrapperFramework,
		req.BaseFramework,
	)

	if err != nil {
		log.Errorf("Failed to update detection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to update detection",
			"details": err.Error(),
		})
		return
	}

	// Get updated detection result
	detection, err := h.detectionManager.GetDetection(c.Request.Context(), req.WorkloadUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get updated detection",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, detection)
}

// GetStats retrieves detection statistics
//
// GET /api/v1/detection/stats
func (h *DetectionHandler) GetStats(c *gin.Context) {
	// Parse query parameters
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")
	namespace := c.Query("namespace")

	stats, err := h.detectionManager.GetStatistics(
		c.Request.Context(),
		startTime,
		endTime,
		namespace,
	)

	if err != nil {
		log.Errorf("Failed to get statistics: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get statistics",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
