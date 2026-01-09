// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package handlers

import (
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// LogDetectionReport represents a log detection report from telemetry-processor
type LogDetectionReport struct {
	WorkloadUID    string    `json:"workload_uid" binding:"required"`
	DetectedAt     time.Time `json:"detected_at"`
	LogTimestamp   time.Time `json:"log_timestamp" binding:"required"`
	Framework      string    `json:"framework"`
	Confidence     float64   `json:"confidence"`
	PatternMatched string    `json:"pattern_matched"`
	WorkloadType   string    `json:"workload_type"`
}

// CoverageStatusResponse represents coverage status for a workload
type CoverageStatusResponse struct {
	WorkloadUID string                     `json:"workload_uid"`
	Coverages   []*model.DetectionCoverage `json:"coverages"`
	TotalCount  int                        `json:"total_count"`
}

// CoordinatorHandler handles detection coordinator related APIs
type CoordinatorHandler struct {
	coverageFacade database.DetectionCoverageFacadeInterface
	evidenceStore  *detection.EvidenceStore
}

// NewCoordinatorHandler creates a new coordinator handler
func NewCoordinatorHandler() *CoordinatorHandler {
	return &CoordinatorHandler{
		coverageFacade: database.NewDetectionCoverageFacade(),
		evidenceStore:  detection.NewEvidenceStore(),
	}
}

// HandleLogReport handles log detection reports from telemetry-processor
// This is called when telemetry-processor detects framework patterns in logs
//
// POST /api/v1/detection/log-report
func (h *CoordinatorHandler) HandleLogReport(c *gin.Context) {
	var report LogDetectionReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	ctx := c.Request.Context()

	// Set default detected_at if not provided
	if report.DetectedAt.IsZero() {
		report.DetectedAt = time.Now()
	}

	// Update log_available_to in detection_coverage
	if err := h.coverageFacade.UpdateLogAvailableTime(ctx, report.WorkloadUID, report.LogTimestamp); err != nil {
		log.Warnf("Failed to update log available time for workload %s: %v", report.WorkloadUID, err)
	}

	// If framework was detected, store as evidence
	if report.Framework != "" {
		workloadType := report.WorkloadType
		if workloadType == "" {
			workloadType = "training"
		}

		confidence := report.Confidence
		if confidence == 0 {
			confidence = 0.8 // Default confidence for log detection
		}

		req := &detection.StoreEvidenceRequest{
			WorkloadUID:  report.WorkloadUID,
			Source:       constant.DetectionSourceLog,
			SourceType:   "passive", // Passive detection from telemetry-processor
			Framework:    report.Framework,
			WorkloadType: workloadType,
			Confidence:   confidence,
			Evidence: map[string]interface{}{
				"pattern_matched": report.PatternMatched,
				"log_timestamp":   report.LogTimestamp.Format(time.RFC3339),
				"detected_at":     report.DetectedAt.Format(time.RFC3339),
				"method":          "log_stream_detection",
			},
		}

		if err := h.evidenceStore.StoreEvidence(ctx, req); err != nil {
			log.Warnf("Failed to store log evidence for workload %s: %v", report.WorkloadUID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to store evidence",
				"details": err.Error(),
			})
			return
		}

		// Also update the covered time range
		if err := h.coverageFacade.UpdateCoveredTimeRange(ctx, report.WorkloadUID, report.LogTimestamp, report.LogTimestamp); err != nil {
			log.Warnf("Failed to update covered time range for workload %s: %v", report.WorkloadUID, err)
		}

		log.Infof("Log detection report received for workload %s: framework=%s, confidence=%.2f",
			report.WorkloadUID, report.Framework, confidence)
	} else {
		log.Debugf("Log detection report received for workload %s (no framework detected, updating log available time)",
			report.WorkloadUID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "ok",
		"workload_uid": report.WorkloadUID,
	})
}

// GetCoverageStatus retrieves detection coverage status for a workload
//
// GET /api/v1/detection/coverage/:uid
func (h *CoordinatorHandler) GetCoverageStatus(c *gin.Context) {
	workloadUID := c.Param("uid")
	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	ctx := c.Request.Context()

	coverages, err := h.coverageFacade.ListCoverageByWorkload(ctx, workloadUID)
	if err != nil {
		log.Errorf("Failed to get coverage for workload %s: %v", workloadUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get coverage",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, CoverageStatusResponse{
		WorkloadUID: workloadUID,
		Coverages:   coverages,
		TotalCount:  len(coverages),
	})
}

// InitializeCoverage initializes detection coverage for a workload
//
// POST /api/v1/detection/coverage/:uid/initialize
func (h *CoordinatorHandler) InitializeCoverage(c *gin.Context) {
	workloadUID := c.Param("uid")
	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	ctx := c.Request.Context()

	if err := h.coverageFacade.InitializeCoverageForWorkload(ctx, workloadUID); err != nil {
		log.Errorf("Failed to initialize coverage for workload %s: %v", workloadUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to initialize coverage",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "ok",
		"workload_uid": workloadUID,
		"message":      "coverage initialized",
	})
}

// GetUncoveredLogWindow retrieves uncovered log time window for a workload
//
// GET /api/v1/detection/coverage/:uid/log-window
func (h *CoordinatorHandler) GetUncoveredLogWindow(c *gin.Context) {
	workloadUID := c.Param("uid")
	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workload_uid is required",
		})
		return
	}

	ctx := c.Request.Context()

	from, to, err := h.coverageFacade.GetUncoveredLogWindow(ctx, workloadUID)
	if err != nil {
		log.Errorf("Failed to get uncovered log window for workload %s: %v", workloadUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get log window",
			"details": err.Error(),
		})
		return
	}

	if from == nil || to == nil {
		c.JSON(http.StatusOK, gin.H{
			"workload_uid": workloadUID,
			"has_gap":      false,
			"message":      "no uncovered log window",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workload_uid": workloadUID,
		"has_gap":      true,
		"from":         from.Format(time.RFC3339),
		"to":           to.Format(time.RFC3339),
	})
}

