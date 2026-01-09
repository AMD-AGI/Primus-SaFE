// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/pgUtil"
	"github.com/gin-gonic/gin"
)

// ListLogAlertRuleTemplates handles GET /v1/log-alert-rule-templates
func ListLogAlertRuleTemplates(ctx *gin.Context) {
	category := ctx.Query("category")

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// List templates
	templates, err := facade.ListLogAlertRuleTemplates(ctx.Request.Context(), category)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list log alert rule templates: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list templates", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"templates": templates,
		"total":     len(templates),
	}))
}

// GetLogAlertRuleTemplate handles GET /v1/log-alert-rule-templates/:id
func GetLogAlertRuleTemplate(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid template ID", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Get template
	template, err := facade.GetLogAlertRuleTemplateByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get log alert rule template: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get template", err))
		return
	}

	if template == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "template not found", nil))
		return
	}

	// Convert to response model with proper tags format
	response := convertTemplateToResponse(template)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// CreateRuleFromTemplate handles POST /v1/log-alert-rule-templates/:id/instantiate
func CreateRuleFromTemplate(ctx *gin.Context) {
	idStr := ctx.Param("id")
	templateID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid template ID", nil))
		return
	}

	var req CreateRuleFromTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Get template
	template, err := facade.GetLogAlertRuleTemplateByID(ctx.Request.Context(), templateID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get template: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get template", err))
		return
	}

	if template == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "template not found", nil))
		return
	}

	// Create rule from template
	rule := &dbmodel.LogAlertRules{
		Name:           req.Name,
		Description:    req.Description,
		ClusterName:    clusterName,
		Enabled:        req.Enabled,
		Priority:       int32(req.Priority),
		LabelSelectors: template.TemplateConfig["label_selectors"].(dbmodel.ExtType),
		MatchType:      template.TemplateConfig["match_type"].(string),
		MatchConfig:    template.TemplateConfig["match_config"].(dbmodel.ExtType),
		Severity:       template.TemplateConfig["severity"].(string),
		AlertTemplate:  template.TemplateConfig["alert_template"].(dbmodel.ExtType),
		CreatedBy:      req.CreatedBy,
	}

	// Apply overrides
	if req.Overrides != nil {
		applyTemplateOverrides(rule, req.Overrides)
	}

	// Set defaults
	if rule.Priority == 0 {
		rule.Priority = 5
	}
	if rule.GroupWait == 0 {
		rule.GroupWait = 30
	}
	if rule.RepeatInterval == 0 {
		rule.RepeatInterval = 3600
	}

	// Create rule
	if err := facade.CreateLogAlertRule(ctx.Request.Context(), rule); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create rule from template: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to create rule", err))
		return
	}

	// Increment template usage count
	if err := facade.IncrementTemplateUsage(ctx.Request.Context(), templateID); err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to increment template usage: %v", err)
	}

	log.GlobalLogger().WithContext(ctx).Infof("Created rule %d from template %d", rule.ID, templateID)

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"rule_id":      rule.ID,
		"cluster_name": clusterName,
	}))
}

// CreateLogAlertRuleTemplate handles POST /v1/log-alert-rule-templates
func CreateLogAlertRuleTemplate(ctx *gin.Context) {
	var req CreateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate request
	if req.Name == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "name is required", nil))
		return
	}
	if req.Category == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "category is required", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Build template
	template := &dbmodel.LogAlertRuleTemplates{
		Name:           req.Name,
		Category:       req.Category,
		Description:    req.Description,
		TemplateConfig: req.TemplateConfig,
		Tags:           pgUtil.StringArrayToPgArray(req.Tags), // Convert []string to PostgreSQL TEXT[] format
		IsBuiltin:      false,
		CreatedBy:      req.CreatedBy,
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Create template
	if err := facade.CreateLogAlertRuleTemplate(ctx.Request.Context(), template); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create template: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to create template", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Created log alert rule template: %s (ID: %d)", template.Name, template.ID)

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"template_id": template.ID,
	}))
}

// DeleteLogAlertRuleTemplate handles DELETE /v1/log-alert-rule-templates/:id
func DeleteLogAlertRuleTemplate(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid template ID", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get template to check if it's builtin
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()
	template, err := facade.GetLogAlertRuleTemplateByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get template: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get template", err))
		return
	}

	if template == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "template not found", nil))
		return
	}

	// Prevent deletion of builtin templates
	if template.IsBuiltin {
		ctx.JSON(http.StatusForbidden, rest.ErrorResp(ctx.Request.Context(), http.StatusForbidden, "cannot delete builtin template", nil))
		return
	}

	// Delete template
	if err := facade.DeleteLogAlertRuleTemplate(ctx.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete template: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to delete template", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Deleted log alert rule template ID: %d", id)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "template deleted successfully",
	}))
}

// Request models
type CreateRuleFromTemplateRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	Overrides   map[string]interface{} `json:"overrides"`
	CreatedBy   string                 `json:"created_by"`
}

type CreateTemplateRequest struct {
	Name           string          `json:"name" binding:"required"`
	Category       string          `json:"category" binding:"required"`
	Description    string          `json:"description"`
	TemplateConfig dbmodel.ExtType `json:"template_config" binding:"required"`
	Tags           []string        `json:"tags"`
	CreatedBy      string          `json:"created_by"`
}

// Response models
type LogAlertRuleTemplateResponse struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Category       string          `json:"category"`
	Description    string          `json:"description"`
	TemplateConfig dbmodel.ExtType `json:"template_config"`
	Tags           []string        `json:"tags"`
	IsBuiltin      bool            `json:"is_builtin"`
	UsageCount     int64           `json:"usage_count"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	CreatedBy      string          `json:"created_by"`
}

// Helper functions

// convertTemplateToResponse converts dbmodel.LogAlertRuleTemplates to response model
func convertTemplateToResponse(template *dbmodel.LogAlertRuleTemplates) *LogAlertRuleTemplateResponse {
	if template == nil {
		return nil
	}

	return &LogAlertRuleTemplateResponse{
		ID:             template.ID,
		Name:           template.Name,
		Category:       template.Category,
		Description:    template.Description,
		TemplateConfig: template.TemplateConfig,
		Tags:           pgUtil.PgArrayToStringArray(template.Tags),
		IsBuiltin:      template.IsBuiltin,
		UsageCount:     template.UsageCount,
		CreatedAt:      template.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      template.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedBy:      template.CreatedBy,
	}
}

func applyTemplateOverrides(rule *dbmodel.LogAlertRules, overrides map[string]interface{}) {
	if selectors, ok := overrides["label_selectors"]; ok {
		if selectorsExt, ok := selectors.(dbmodel.ExtType); ok {
			rule.LabelSelectors = selectorsExt
		}
	}
	if severity, ok := overrides["severity"].(string); ok {
		rule.Severity = severity
	}
	if priority, ok := overrides["priority"].(int); ok {
		rule.Priority = int32(priority)
	}
	if groupBy, ok := overrides["group_by"].([]string); ok {
		rule.GroupBy = pgUtil.StringArrayToPgArray(groupBy)
	}
}
