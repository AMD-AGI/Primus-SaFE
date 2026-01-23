// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// AlertRuleAdviceRequest represents a request to create or update an alert rule advice
type AlertRuleAdviceRequest struct {
	RuleType        string                 `json:"rule_type" binding:"required"` // log/metric
	Name            string                 `json:"name" binding:"required"`
	Title           string                 `json:"title" binding:"required"`
	Description     string                 `json:"description"`
	Category        string                 `json:"category" binding:"required"` // performance/error/resource/security/availability
	ClusterName     string                 `json:"cluster_name" binding:"required"`
	TargetResource  string                 `json:"target_resource"`
	TargetName      string                 `json:"target_name"`
	RuleConfig      map[string]interface{} `json:"rule_config" binding:"required"`
	Severity        string                 `json:"severity"`
	Priority        int                    `json:"priority"`
	Reason          string                 `json:"reason"`
	Evidence        map[string]interface{} `json:"evidence"`
	InspectionID    string                 `json:"inspection_id"`
	InspectionTime  *time.Time             `json:"inspection_time"`
	Tags            []string               `json:"tags"`
	ConfidenceScore float64                `json:"confidence_score"`
	ExpiresAt       *time.Time             `json:"expires_at"`
	CreatedBy       string                 `json:"created_by"`
}

// UpdateAdviceStatusRequest represents a request to update advice status
type UpdateAdviceStatusRequest struct {
	Status      string `json:"status" binding:"required"` // reviewed/accepted/rejected
	ReviewedBy  string `json:"reviewed_by"`
	ReviewNotes string `json:"review_notes"`
}

// BatchUpdateAdviceStatusRequest represents a request to batch update advice status
type BatchUpdateAdviceStatusRequest struct {
	AdviceIDs   []int64 `json:"advice_ids" binding:"required"`
	Status      string  `json:"status" binding:"required"`
	ReviewedBy  string  `json:"reviewed_by"`
	ReviewNotes string  `json:"review_notes"`
}

// ApplyAdviceRequest represents a request to apply an advice and create an actual alert rule
type ApplyAdviceRequest struct {
	CreateEnabled bool   `json:"create_enabled"` // Whether to create the rule as enabled
	CustomName    string `json:"custom_name"`    // Optional: override the rule name
	CreatedBy     string `json:"created_by"`
}

// CreateAlertRuleAdvice handles POST /v1/alert-rule-advices
func CreateAlertRuleAdvice(ctx *gin.Context) {
	var req AlertRuleAdviceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate request
	if err := validateAlertRuleAdviceRequest(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Build advice model
	advice, err := buildAlertRuleAdviceFromRequest(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "failed to build advice: "+err.Error(), nil))
		return
	}

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Create advice
	if err := facade.CreateAlertRuleAdvices(ctx.Request.Context(), advice); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create alert rule advice: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to create advice", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Created alert rule advice: %s (ID: %d)", advice.Name, advice.ID)

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"advice_id": advice.ID,
		"message":   "alert rule advice created successfully",
	}))
}

// BatchCreateAlertRuleAdvices handles POST /v1/alert-rule-advices/batch
func BatchCreateAlertRuleAdvices(ctx *gin.Context) {
	var reqs []AlertRuleAdviceRequest
	if err := ctx.ShouldBindJSON(&reqs); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	if len(reqs) == 0 {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "at least one advice is required", nil))
		return
	}

	// Build advice models
	advices := make([]*dbmodel.AlertRuleAdvices, 0, len(reqs))
	for i, req := range reqs {
		if err := validateAlertRuleAdviceRequest(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest,
				fmt.Sprintf("validation failed for advice %d: %s", i, err.Error()), nil))
			return
		}
		advice, err := buildAlertRuleAdviceFromRequest(&req)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest,
				fmt.Sprintf("failed to build advice %d: %s", i, err.Error()), nil))
			return
		}
		advices = append(advices, advice)
	}

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Batch create advices
	if err := facade.BatchCreateAlertRuleAdvicess(ctx.Request.Context(), advices); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to batch create alert rule advices: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to batch create advices", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Batch created %d alert rule advices", len(advices))

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"count":   len(advices),
		"message": fmt.Sprintf("successfully created %d alert rule advices", len(advices)),
	}))
}

// ListAlertRuleAdvices handles GET /v1/alert-rule-advices
func ListAlertRuleAdvices(ctx *gin.Context) {
	// Parse filter parameters
	filter := parseAlertRuleAdviceFilter(ctx)

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// List advices
	advices, total, err := facade.ListAlertRuleAdvicess(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list alert rule advices: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list advices", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"advices": advices,
		"total":   total,
		"offset":  filter.Offset,
		"limit":   filter.Limit,
	}))
}

// GetAlertRuleAdvice handles GET /v1/alert-rule-advices/:id
func GetAlertRuleAdvice(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid advice ID", nil))
		return
	}

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Get advice
	advice, err := facade.GetAlertRuleAdvicesByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert rule advice: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get advice", err))
		return
	}

	if advice == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "advice not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), advice))
}

// UpdateAlertRuleAdvice handles PUT /v1/alert-rule-advices/:id
func UpdateAlertRuleAdvice(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid advice ID", nil))
		return
	}

	var req AlertRuleAdviceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Get existing advice
	existingAdvice, err := facade.GetAlertRuleAdvicesByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert rule advice: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get advice", err))
		return
	}

	if existingAdvice == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "advice not found", nil))
		return
	}

	// Update advice fields
	if err := updateAdviceFromRequest(existingAdvice, &req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "failed to update advice: "+err.Error(), nil))
		return
	}

	// Save advice
	if err := facade.UpdateAlertRuleAdvices(ctx.Request.Context(), existingAdvice); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update alert rule advice: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to update advice", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Updated alert rule advice: ID %d", id)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"advice_id": id,
		"message":   "alert rule advice updated successfully",
	}))
}

// DeleteAlertRuleAdvice handles DELETE /v1/alert-rule-advices/:id
func DeleteAlertRuleAdvice(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid advice ID", nil))
		return
	}

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Delete advice
	if err := facade.DeleteAlertRuleAdvices(ctx.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete alert rule advice: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to delete advice", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Deleted alert rule advice: ID %d", id)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "alert rule advice deleted successfully",
	}))
}

// BatchDeleteAlertRuleAdvices handles POST /v1/alert-rule-advices/batch-delete
func BatchDeleteAlertRuleAdvices(ctx *gin.Context) {
	var req struct {
		AdviceIDs []int64 `json:"advice_ids" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	if len(req.AdviceIDs) == 0 {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "advice_ids is required", nil))
		return
	}

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Batch delete
	if err := facade.BatchDeleteAlertRuleAdvicess(ctx.Request.Context(), req.AdviceIDs); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to batch delete advices: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to batch delete advices", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Batch deleted %d advices", len(req.AdviceIDs))

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"deleted_count": len(req.AdviceIDs),
		"message":       fmt.Sprintf("successfully deleted %d advices", len(req.AdviceIDs)),
	}))
}

// UpdateAdviceStatus handles POST /v1/alert-rule-advices/:id/status
func UpdateAdviceStatus(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid advice ID", nil))
		return
	}

	var req UpdateAdviceStatusRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate status
	validStatuses := []string{"reviewed", "accepted", "rejected"}
	isValid := false
	for _, s := range validStatuses {
		if req.Status == s {
			isValid = true
			break
		}
	}
	if !isValid {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid status, must be one of: reviewed, accepted, rejected", nil))
		return
	}

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Update status
	if err := facade.UpdateAdviceStatus(ctx.Request.Context(), id, req.Status, req.ReviewedBy, req.ReviewNotes); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update advice status: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to update status", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Updated advice %d status to %s", id, req.Status)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"advice_id": id,
		"status":    req.Status,
		"message":   "advice status updated successfully",
	}))
}

// BatchUpdateAdviceStatus handles POST /v1/alert-rule-advices/batch-status
func BatchUpdateAdviceStatus(ctx *gin.Context) {
	var req BatchUpdateAdviceStatusRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	if len(req.AdviceIDs) == 0 {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "advice_ids is required", nil))
		return
	}

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Batch update status
	if err := facade.BatchUpdateStatus(ctx.Request.Context(), req.AdviceIDs, req.Status, req.ReviewedBy, req.ReviewNotes); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to batch update advice status: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to batch update status", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Batch updated status for %d advices to %s", len(req.AdviceIDs), req.Status)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"updated_count": len(req.AdviceIDs),
		"status":        req.Status,
		"message":       fmt.Sprintf("successfully updated status for %d advices", len(req.AdviceIDs)),
	}))
}

// GetAdviceSummary handles GET /v1/alert-rule-advices/summary
func GetAdviceSummary(ctx *gin.Context) {
	// Parse filter parameters
	filter := parseAlertRuleAdviceFilter(ctx)

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Get summary
	summary, err := facade.GetAdviceSummary(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get advice summary: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get summary", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), summary))
}

// GetAdviceStatistics handles GET /v1/alert-rule-advices/statistics
func GetAdviceStatistics(ctx *gin.Context) {
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Parse time range
	dateFrom, dateTo := parseTimeRange(ctx)

	// Get facade
	facade := database.GetFacade().GetAlertRuleAdvice()

	// Get statistics
	stats, err := facade.GetAdviceStatistics(ctx.Request.Context(), clusterName, dateFrom, dateTo)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get advice statistics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get statistics", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"statistics": stats,
		"date_from":  dateFrom.Format("2006-01-02"),
		"date_to":    dateTo.Format("2006-01-02"),
	}))
}

// ApplyAlertRuleAdvice handles POST /v1/alert-rule-advices/:id/apply
// This endpoint creates an actual alert rule from the advice
func ApplyAlertRuleAdvice(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid advice ID", nil))
		return
	}

	var req ApplyAdviceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get facade
	adviceFacade := database.GetFacade().GetAlertRuleAdvice()

	// Get advice
	advice, err := adviceFacade.GetAlertRuleAdvicesByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert rule advice: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get advice", err))
		return
	}

	if advice == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "advice not found", nil))
		return
	}

	// Check if already applied
	if advice.Status == "applied" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "advice already applied", nil))
		return
	}

	// Apply the advice based on rule type
	var ruleID int64
	if advice.RuleType == "log" {
		ruleID, err = applyLogAlertAdvice(ctx, advice, &req)
	} else if advice.RuleType == "metric" {
		ruleID, err = applyMetricAlertAdvice(ctx, advice, &req)
	} else {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "unsupported rule type", nil))
		return
	}

	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to apply advice: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to apply advice: "+err.Error(), err))
		return
	}

	// Mark advice as applied
	if err := adviceFacade.MarkAsApplied(ctx.Request.Context(), id, ruleID); err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to mark advice as applied: %v", err)
	}

	log.GlobalLogger().WithContext(ctx).Infof("Applied advice %d, created %s alert rule %d", id, advice.RuleType, ruleID)

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"advice_id": id,
		"rule_type": advice.RuleType,
		"rule_id":   ruleID,
		"message":   fmt.Sprintf("advice applied successfully, %s alert rule created", advice.RuleType),
	}))
}

// Helper functions

func validateAlertRuleAdviceRequest(req *AlertRuleAdviceRequest) error {
	if req.RuleType != "log" && req.RuleType != "metric" {
		return fmt.Errorf("rule_type must be 'log' or 'metric'")
	}
	validCategories := []string{"performance", "error", "resource", "security", "availability"}
	isValidCategory := false
	for _, c := range validCategories {
		if req.Category == c {
			isValidCategory = true
			break
		}
	}
	if !isValidCategory {
		return fmt.Errorf("category must be one of: performance, error, resource, security, availability")
	}
	if req.Priority < 1 || req.Priority > 10 {
		return fmt.Errorf("priority must be between 1 and 10")
	}
	if req.ConfidenceScore < 0 || req.ConfidenceScore > 1 {
		return fmt.Errorf("confidence_score must be between 0 and 1")
	}
	return nil
}

func buildAlertRuleAdviceFromRequest(req *AlertRuleAdviceRequest) (*dbmodel.AlertRuleAdvices, error) {
	// Convert rule config to ExtType
	ruleConfigBytes, err := json.Marshal(req.RuleConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rule_config: %v", err)
	}
	var ruleConfigExt dbmodel.ExtType
	if err := json.Unmarshal(ruleConfigBytes, &ruleConfigExt); err != nil {
		return nil, fmt.Errorf("failed to convert rule_config: %v", err)
	}

	// Convert evidence to ExtType if present
	var evidenceExt dbmodel.ExtType
	if req.Evidence != nil {
		evidenceBytes, err := json.Marshal(req.Evidence)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal evidence: %v", err)
		}
		if err := json.Unmarshal(evidenceBytes, &evidenceExt); err != nil {
			return nil, fmt.Errorf("failed to convert evidence: %v", err)
		}
	}

	// Set defaults
	inspectionTime := time.Now()
	if req.InspectionTime != nil {
		inspectionTime = *req.InspectionTime
	}

	severity := req.Severity
	if severity == "" {
		severity = "warning"
	}

	priority := req.Priority
	if priority == 0 {
		priority = 5
	}

	// Convert tags array to JSON string
	tagsStr := ""
	if len(req.Tags) > 0 {
		tagsBytes, _ := json.Marshal(req.Tags)
		tagsStr = string(tagsBytes)
	}

	// Handle ExpiresAt pointer
	var expiresAt time.Time
	if req.ExpiresAt != nil {
		expiresAt = *req.ExpiresAt
	}

	advice := &dbmodel.AlertRuleAdvices{
		RuleType:        req.RuleType,
		Name:            req.Name,
		Title:           req.Title,
		Description:     req.Description,
		Category:        req.Category,
		ClusterName:     req.ClusterName,
		TargetResource:  req.TargetResource,
		TargetName:      req.TargetName,
		RuleConfig:      ruleConfigExt,
		Severity:        severity,
		Priority:        int32(priority),
		Reason:          req.Reason,
		Evidence:        evidenceExt,
		Status:          "pending",
		InspectionID:    req.InspectionID,
		InspectionTime:  inspectionTime,
		Tags:            tagsStr,
		ConfidenceScore: req.ConfidenceScore,
		ExpiresAt:       expiresAt,
		CreatedBy:       req.CreatedBy,
	}

	return advice, nil
}

func updateAdviceFromRequest(advice *dbmodel.AlertRuleAdvices, req *AlertRuleAdviceRequest) error {
	if req.Name != "" {
		advice.Name = req.Name
	}
	if req.Title != "" {
		advice.Title = req.Title
	}
	if req.Description != "" {
		advice.Description = req.Description
	}
	if req.Category != "" {
		advice.Category = req.Category
	}
	if req.Severity != "" {
		advice.Severity = req.Severity
	}
	if req.Priority > 0 {
		advice.Priority = int32(req.Priority)
	}
	if req.Reason != "" {
		advice.Reason = req.Reason
	}
	if len(req.Tags) > 0 {
		tagsBytes, _ := json.Marshal(req.Tags)
		advice.Tags = string(tagsBytes)
	}
	if req.ConfidenceScore > 0 {
		advice.ConfidenceScore = req.ConfidenceScore
	}

	// Update rule config if provided
	if len(req.RuleConfig) > 0 {
		ruleConfigBytes, err := json.Marshal(req.RuleConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal rule_config: %v", err)
		}
		var ruleConfigExt dbmodel.ExtType
		if err := json.Unmarshal(ruleConfigBytes, &ruleConfigExt); err != nil {
			return fmt.Errorf("failed to convert rule_config: %v", err)
		}
		advice.RuleConfig = ruleConfigExt
	}

	// Update evidence if provided
	if req.Evidence != nil {
		evidenceBytes, err := json.Marshal(req.Evidence)
		if err != nil {
			return fmt.Errorf("failed to marshal evidence: %v", err)
		}
		var evidenceExt dbmodel.ExtType
		if err := json.Unmarshal(evidenceBytes, &evidenceExt); err != nil {
			return fmt.Errorf("failed to convert evidence: %v", err)
		}
		advice.Evidence = evidenceExt
	}

	return nil
}

func parseAlertRuleAdviceFilter(ctx *gin.Context) *database.AlertRuleAdvicesFilter {
	filter := &database.AlertRuleAdvicesFilter{
		Offset:    0,
		Limit:     50,
		OrderDesc: true,
	}

	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = offset
		}
	}

	if limitStr := ctx.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	}

	filter.ClusterName = ctx.Query("cluster")
	filter.RuleType = ctx.Query("rule_type")
	filter.Category = ctx.Query("category")
	filter.Status = ctx.Query("status")
	filter.InspectionID = ctx.Query("inspection_id")
	filter.TargetResource = ctx.Query("target_resource")
	filter.TargetName = ctx.Query("target_name")
	filter.Keyword = ctx.Query("keyword")
	filter.OrderBy = ctx.Query("order_by")

	if excludeExpiredStr := ctx.Query("exclude_expired"); excludeExpiredStr != "" {
		if excludeExpired, err := strconv.ParseBool(excludeExpiredStr); err == nil {
			filter.ExcludeExpired = excludeExpired
		}
	}

	if minPriorityStr := ctx.Query("min_priority"); minPriorityStr != "" {
		if minPriority, err := strconv.Atoi(minPriorityStr); err == nil {
			filter.MinPriority = &minPriority
		}
	}

	if maxPriorityStr := ctx.Query("max_priority"); maxPriorityStr != "" {
		if maxPriority, err := strconv.Atoi(maxPriorityStr); err == nil {
			filter.MaxPriority = &maxPriority
		}
	}

	if minConfidenceStr := ctx.Query("min_confidence"); minConfidenceStr != "" {
		if minConfidence, err := strconv.ParseFloat(minConfidenceStr, 64); err == nil {
			filter.MinConfidence = &minConfidence
		}
	}

	if maxConfidenceStr := ctx.Query("max_confidence"); maxConfidenceStr != "" {
		if maxConfidence, err := strconv.ParseFloat(maxConfidenceStr, 64); err == nil {
			filter.MaxConfidence = &maxConfidence
		}
	}

	return filter
}

func applyLogAlertAdvice(ctx *gin.Context, advice *dbmodel.AlertRuleAdvices, req *ApplyAdviceRequest) (int64, error) {
	// Convert advice rule_config to LogAlertRuleRequest
	var logConfig map[string]interface{}
	configBytes, _ := json.Marshal(advice.RuleConfig)
	if err := json.Unmarshal(configBytes, &logConfig); err != nil {
		return 0, fmt.Errorf("failed to parse log rule config: %v", err)
	}

	// Extract required fields
	labelSelectors, _ := logConfig["label_selectors"].([]interface{})
	matchType, _ := logConfig["match_type"].(string)
	matchConfig, _ := logConfig["match_config"].(map[string]interface{})

	ruleName := advice.Name
	if req.CustomName != "" {
		ruleName = req.CustomName
	}

	logReq := LogAlertRuleRequest{
		Name:           ruleName,
		Description:    advice.Description,
		Enabled:        req.CreateEnabled,
		Priority:       int(advice.Priority),
		LabelSelectors: labelSelectors,
		MatchType:      matchType,
		MatchConfig:    matchConfig,
		Severity:       advice.Severity,
		CreatedBy:      req.CreatedBy,
	}

	// Extract optional fields
	if alertTemplate, ok := logConfig["alert_template"].(map[string]interface{}); ok {
		logReq.AlertTemplate = alertTemplate
	}
	if groupBy, ok := logConfig["group_by"].([]interface{}); ok {
		logReq.GroupBy = make([]string, len(groupBy))
		for i, g := range groupBy {
			logReq.GroupBy[i], _ = g.(string)
		}
	}
	if groupWait, ok := logConfig["group_wait"].(float64); ok {
		logReq.GroupWait = int64(groupWait)
	}
	if repeatInterval, ok := logConfig["repeat_interval"].(float64); ok {
		logReq.RepeatInterval = int64(repeatInterval)
	}
	if routeConfig, ok := logConfig["route_config"].(map[string]interface{}); ok {
		logReq.RouteConfig = routeConfig
	}

	// Build and create log alert rule
	rule := buildLogAlertRuleFromRequest(&logReq, advice.ClusterName)
	facade := database.GetFacadeForCluster(advice.ClusterName).GetLogAlertRule()
	if err := facade.CreateLogAlertRule(ctx.Request.Context(), rule); err != nil {
		return 0, err
	}

	return rule.ID, nil
}

func applyMetricAlertAdvice(ctx *gin.Context, advice *dbmodel.AlertRuleAdvices, req *ApplyAdviceRequest) (int64, error) {
	// Convert advice rule_config to MetricAlertRuleRequest
	var metricConfig map[string]interface{}
	configBytes, _ := json.Marshal(advice.RuleConfig)
	if err := json.Unmarshal(configBytes, &metricConfig); err != nil {
		return 0, fmt.Errorf("failed to parse metric rule config: %v", err)
	}

	// Extract groups
	groupsData, ok := metricConfig["groups"]
	if !ok {
		return 0, fmt.Errorf("groups not found in metric rule config")
	}
	groupsBytes, _ := json.Marshal(groupsData)
	var groups []model.VMRuleGroup
	if err := json.Unmarshal(groupsBytes, &groups); err != nil {
		return 0, fmt.Errorf("failed to parse groups: %v", err)
	}

	ruleName := advice.Name
	if req.CustomName != "" {
		ruleName = req.CustomName
	}

	metricReq := MetricAlertRuleRequest{
		Name:        ruleName,
		ClusterName: advice.ClusterName,
		Enabled:     req.CreateEnabled,
		Groups:      groups,
		Description: advice.Description,
	}

	// Extract optional labels
	if labels, ok := metricConfig["labels"].(map[string]interface{}); ok {
		metricReq.Labels = make(map[string]string)
		for k, v := range labels {
			if vStr, ok := v.(string); ok {
				metricReq.Labels[k] = vStr
			}
		}
	}

	// Build and create metric alert rule using existing logic
	// This is a simplified version - you may need to adapt based on your existing implementation
	var groupsExt dbmodel.ExtType
	groupsBytes2, _ := json.Marshal(metricReq.Groups)
	json.Unmarshal(groupsBytes2, &groupsExt)

	var labelsExt dbmodel.ExtType
	if metricReq.Labels != nil {
		labelsBytes, _ := json.Marshal(metricReq.Labels)
		json.Unmarshal(labelsBytes, &labelsExt)
	}

	rule := &dbmodel.MetricAlertRules{
		Name:        metricReq.Name,
		ClusterName: metricReq.ClusterName,
		Enabled:     metricReq.Enabled,
		Groups:      groupsExt,
		Description: metricReq.Description,
		Labels:      labelsExt,
		SyncStatus:  "pending",
		CreatedBy:   req.CreatedBy,
	}

	facade := database.GetFacade().GetMetricAlertRule()
	if err := facade.CreateMetricAlertRule(ctx.Request.Context(), rule); err != nil {
		return 0, err
	}

	return rule.ID, nil
}
