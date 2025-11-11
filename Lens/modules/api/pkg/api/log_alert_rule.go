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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/pgUtil"
	"github.com/gin-gonic/gin"
)

// CreateLogAlertRule handles POST /v1/log-alert-rules
func CreateLogAlertRule(ctx *gin.Context) {
	var req LogAlertRuleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate request
	if err := validateLogAlertRuleRequest(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get cluster name (from query param or use default)
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Build rule model
	rule := buildLogAlertRuleFromRequest(&req, clusterName)

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Create rule
	if err := facade.CreateLogAlertRule(ctx.Request.Context(), rule); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create log alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to create rule", err))
		return
	}

	// Create initial version
	if req.CreateVersion {
		version := createRuleVersion(rule, 1, "Initial version", req.CreatedBy)
		if err := facade.CreateRuleVersion(ctx.Request.Context(), version); err != nil {
			log.GlobalLogger().WithContext(ctx).Warningf("Failed to create rule version: %v", err)
		}
	}

	log.GlobalLogger().WithContext(ctx).Infof("Created log alert rule: %s (ID: %d) in cluster: %s", rule.Name, rule.ID, clusterName)

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"rule_id":      rule.ID,
		"cluster_name": clusterName,
	}))
}

// ListLogAlertRules handles GET /v1/log-alert-rules
func ListLogAlertRules(ctx *gin.Context) {
	// Get cluster name (from query param or use default)
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		// List rules from default cluster if configured
		defaultCluster := clientsets.GetClusterManager().GetDefaultClusterName()
		if defaultCluster != "" {
			clusterName = defaultCluster
		} else {
			clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
		}
	}

	// Parse filter parameters
	filter := parseLogAlertRuleFilter(ctx)
	filter.ClusterName = clusterName

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// List rules
	rules, total, err := facade.ListLogAlertRules(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list log alert rules: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list rules", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"rules":        rules,
		"total":        total,
		"offset":       filter.Offset,
		"limit":        filter.Limit,
		"cluster_name": clusterName,
	}))
}

// ListLogAlertRulesMultiCluster handles GET /v1/log-alert-rules/multi-cluster
func ListLogAlertRulesMultiCluster(ctx *gin.Context) {
	// Get all clusters
	clusterManager := clientsets.GetClusterManager()
	clusterNames := clusterManager.GetClusterNames()

	// Parse filter parameters
	baseFilter := parseLogAlertRuleFilter(ctx)

	// Collect rules from all clusters
	type ClusterRules struct {
		ClusterName string                   `json:"cluster_name"`
		Rules       []*dbmodel.LogAlertRules `json:"rules"`
		Total       int64                    `json:"total"`
		Error       string                   `json:"error,omitempty"`
	}

	results := make([]ClusterRules, 0, len(clusterNames))

	for _, clusterName := range clusterNames {
		filter := *baseFilter
		filter.ClusterName = clusterName

		facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()
		rules, total, err := facade.ListLogAlertRules(ctx.Request.Context(), &filter)

		result := ClusterRules{
			ClusterName: clusterName,
		}

		if err != nil {
			log.GlobalLogger().WithContext(ctx).Warningf("Failed to list rules from cluster %s: %v", clusterName, err)
			result.Error = err.Error()
		} else {
			result.Rules = rules
			result.Total = total
		}

		results = append(results, result)
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"clusters": results,
	}))
}

// GetLogAlertRule handles GET /v1/log-alert-rules/:id
func GetLogAlertRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Get rule
	rule, err := facade.GetLogAlertRuleByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get log alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get rule", err))
		return
	}

	if rule == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), rule))
}

// UpdateLogAlertRule handles PUT /v1/log-alert-rules/:id
func UpdateLogAlertRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	var req LogAlertRuleRequest
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

	// Get existing rule
	existingRule, err := facade.GetLogAlertRuleByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get log alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get rule", err))
		return
	}

	if existingRule == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	// Update rule fields
	updateRuleFromRequest(existingRule, &req)

	// Save rule
	if err := facade.UpdateLogAlertRule(ctx.Request.Context(), existingRule); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update log alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to update rule", err))
		return
	}

	// Create new version if requested
	if req.CreateVersion {
		// Get latest version number
		versions, _ := facade.ListRuleVersions(ctx.Request.Context(), id)
		nextVersion := len(versions) + 1

		version := createRuleVersion(existingRule, nextVersion, req.ChangeLog, req.UpdatedBy)
		if err := facade.CreateRuleVersion(ctx.Request.Context(), version); err != nil {
			log.GlobalLogger().WithContext(ctx).Warningf("Failed to create rule version: %v", err)
		}
	}

	log.GlobalLogger().WithContext(ctx).Infof("Updated log alert rule: %s (ID: %d)", existingRule.Name, existingRule.ID)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"rule_id": existingRule.ID,
	}))
}

// DeleteLogAlertRule handles DELETE /v1/log-alert-rules/:id
func DeleteLogAlertRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Delete rule
	if err := facade.DeleteLogAlertRule(ctx.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete log alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to delete rule", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Deleted log alert rule ID: %d", id)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "rule deleted successfully",
	}))
}

// BatchUpdateLogAlertRules handles POST /v1/log-alert-rules/batch-update
func BatchUpdateLogAlertRules(ctx *gin.Context) {
	var req BatchUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	if len(req.RuleIDs) == 0 {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "rule_ids is required", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Update rules
	if err := facade.BatchUpdateEnabledStatus(ctx.Request.Context(), req.RuleIDs, req.Enabled); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to batch update rules: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to batch update rules", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Batch updated %d rules", len(req.RuleIDs))

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"updated_count": len(req.RuleIDs),
	}))
}

// TestLogAlertRule handles POST /v1/log-alert-rules/test
func TestLogAlertRule(ctx *gin.Context) {
	var req TestLogAlertRuleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// TODO: Implement rule testing logic
	// This would create a temporary rule and evaluate sample logs against it

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "rule test not yet implemented",
	}))
}

// GetLogAlertRuleStatistics handles GET /v1/log-alert-rules/:id/statistics
func GetLogAlertRuleStatistics(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Parse time range
	dateFrom, dateTo := parseTimeRange(ctx)

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Get statistics
	summary, err := facade.GetRuleStatisticsSummary(ctx.Request.Context(), id, dateFrom, dateTo)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get rule statistics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get statistics", err))
		return
	}

	// Get detailed statistics
	filter := &database.LogAlertRuleStatisticFilter{
		RuleID:      id,
		ClusterName: clusterName,
		DateFrom:    dateFrom,
		DateTo:      dateTo,
		Limit:       100,
	}

	stats, err := facade.ListRuleStatistics(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list rule statistics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list statistics", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"summary":    summary,
		"statistics": stats,
	}))
}

// GetLogAlertRuleVersions handles GET /v1/log-alert-rules/:id/versions
func GetLogAlertRuleVersions(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// List versions
	versions, err := facade.ListRuleVersions(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list rule versions: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list versions", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"versions": versions,
		"total":    len(versions),
	}))
}

// RollbackLogAlertRule handles POST /v1/log-alert-rules/:id/rollback/:version
func RollbackLogAlertRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	versionStr := ctx.Param("version")
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid version", nil))
		return
	}

	// Get cluster name
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facade for the target cluster
	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	// Get version
	ruleVersion, err := facade.GetRuleVersion(ctx.Request.Context(), id, version)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get rule version: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get version", err))
		return
	}

	if ruleVersion == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "version not found", nil))
		return
	}

	// Get current rule
	currentRule, err := facade.GetLogAlertRuleByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get rule", err))
		return
	}

	if currentRule == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	// Restore configuration from version
	var versionConfig dbmodel.LogAlertRules
	configBytes, _ := json.Marshal(ruleVersion.Config)
	if err := json.Unmarshal(configBytes, &versionConfig); err != nil {
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to parse version config", err))
		return
	}

	// Update current rule with version config
	currentRule.LabelSelectors = versionConfig.LabelSelectors
	currentRule.MatchType = versionConfig.MatchType
	currentRule.MatchConfig = versionConfig.MatchConfig
	currentRule.Severity = versionConfig.Severity
	currentRule.AlertTemplate = versionConfig.AlertTemplate
	currentRule.GroupBy = versionConfig.GroupBy
	currentRule.GroupWait = versionConfig.GroupWait
	currentRule.RepeatInterval = versionConfig.RepeatInterval
	currentRule.RouteConfig = versionConfig.RouteConfig

	// Save rule
	if err := facade.UpdateLogAlertRule(ctx.Request.Context(), currentRule); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to rollback rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to rollback rule", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Rolled back rule %d to version %d", id, version)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "rule rolled back successfully",
	}))
}

// CloneLogAlertRule handles POST /v1/log-alert-rules/:id/clone
func CloneLogAlertRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	var req CloneLogAlertRuleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get source cluster name
	sourceCluster := ctx.Query("cluster")
	if sourceCluster == "" {
		sourceCluster = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get target cluster name
	targetCluster := req.TargetClusterName
	if targetCluster == "" {
		targetCluster = sourceCluster
	}

	// Get source rule
	sourceFacade := database.GetFacadeForCluster(sourceCluster).GetLogAlertRule()
	sourceRule, err := sourceFacade.GetLogAlertRuleByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get source rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get source rule", err))
		return
	}

	if sourceRule == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "source rule not found", nil))
		return
	}

	// Clone rule
	newRule := &dbmodel.LogAlertRules{
		Name:           req.NewName,
		Description:    sourceRule.Description,
		ClusterName:    targetCluster,
		Enabled:        req.Enabled,
		Priority:       sourceRule.Priority,
		LabelSelectors: sourceRule.LabelSelectors,
		MatchType:      sourceRule.MatchType,
		MatchConfig:    sourceRule.MatchConfig,
		Severity:       sourceRule.Severity,
		AlertTemplate:  sourceRule.AlertTemplate,
		GroupBy:        sourceRule.GroupBy,
		GroupWait:      sourceRule.GroupWait,
		RepeatInterval: sourceRule.RepeatInterval,
		RouteConfig:    sourceRule.RouteConfig,
		CreatedBy:      req.CreatedBy,
	}

	// Create in target cluster
	targetFacade := database.GetFacadeForCluster(targetCluster).GetLogAlertRule()
	if err := targetFacade.CreateLogAlertRule(ctx.Request.Context(), newRule); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to clone rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to clone rule", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Cloned rule %d to new rule %d in cluster %s", id, newRule.ID, targetCluster)

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"rule_id":      newRule.ID,
		"cluster_name": targetCluster,
	}))
}

// Request/Response models
type LogAlertRuleRequest struct {
	Name           string                 `json:"name" binding:"required"`
	Description    string                 `json:"description"`
	Enabled        bool                   `json:"enabled"`
	Priority       int                    `json:"priority"`
	LabelSelectors []interface{}          `json:"label_selectors" binding:"required"`
	MatchType      string                 `json:"match_type" binding:"required"`
	MatchConfig    map[string]interface{} `json:"match_config" binding:"required"`
	Severity       string                 `json:"severity"`
	AlertTemplate  map[string]interface{} `json:"alert_template"`
	GroupBy        []string               `json:"group_by"`
	GroupWait      int64                  `json:"group_wait"`
	RepeatInterval int64                  `json:"repeat_interval"`
	RouteConfig    map[string]interface{} `json:"route_config"`
	CreatedBy      string                 `json:"created_by"`
	UpdatedBy      string                 `json:"updated_by"`
	CreateVersion  bool                   `json:"create_version"`
	ChangeLog      string                 `json:"change_log"`
}

type BatchUpdateRequest struct {
	RuleIDs []int64 `json:"rule_ids" binding:"required"`
	Enabled bool    `json:"enabled"`
}

type TestLogAlertRuleRequest struct {
	Rule       LogAlertRuleRequest `json:"rule" binding:"required"`
	SampleLogs []interface{}       `json:"sample_logs" binding:"required"`
}

type CloneLogAlertRuleRequest struct {
	NewName           string `json:"new_name" binding:"required"`
	TargetClusterName string `json:"target_cluster_name"`
	Enabled           bool   `json:"enabled"`
	CreatedBy         string `json:"created_by"`
}

// Helper functions

func validateLogAlertRuleRequest(req *LogAlertRuleRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.MatchType == "" {
		return fmt.Errorf("match_type is required")
	}
	if len(req.LabelSelectors) == 0 {
		return fmt.Errorf("label_selectors is required")
	}
	if len(req.MatchConfig) == 0 {
		return fmt.Errorf("match_config is required")
	}
	return nil
}

func parseLogAlertRuleFilter(ctx *gin.Context) *database.LogAlertRuleFilter {
	filter := &database.LogAlertRuleFilter{
		Offset: 0,
		Limit:  50,
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

	if enabledStr := ctx.Query("enabled"); enabledStr != "" {
		if enabled, err := strconv.ParseBool(enabledStr); err == nil {
			filter.Enabled = &enabled
		}
	}

	filter.MatchType = ctx.Query("match_type")
	filter.Severity = ctx.Query("severity")
	filter.CreatedBy = ctx.Query("created_by")
	filter.Keyword = ctx.Query("keyword")

	if priorityStr := ctx.Query("priority"); priorityStr != "" {
		if priority, err := strconv.Atoi(priorityStr); err == nil {
			filter.Priority = &priority
		}
	}

	return filter
}

func parseTimeRange(ctx *gin.Context) (time.Time, time.Time) {
	dateFrom := time.Now().AddDate(0, 0, -7) // Default: last 7 days
	dateTo := time.Now()

	if fromStr := ctx.Query("from"); fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			dateFrom = t
		}
	}

	if toStr := ctx.Query("to"); toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			dateTo = t
		}
	}

	return dateFrom, dateTo
}

func buildLogAlertRuleFromRequest(req *LogAlertRuleRequest, clusterName string) *dbmodel.LogAlertRules {
	labelsBytes, _ := json.Marshal(req.LabelSelectors)
	var labelsExt dbmodel.ExtType
	json.Unmarshal(labelsBytes, &labelsExt)

	matchConfigBytes, _ := json.Marshal(req.MatchConfig)
	var matchConfigExt dbmodel.ExtType
	json.Unmarshal(matchConfigBytes, &matchConfigExt)

	alertTemplateBytes, _ := json.Marshal(req.AlertTemplate)
	var alertTemplateExt dbmodel.ExtType
	json.Unmarshal(alertTemplateBytes, &alertTemplateExt)

	routeConfigBytes, _ := json.Marshal(req.RouteConfig)
	var routeConfigExt dbmodel.ExtType
	json.Unmarshal(routeConfigBytes, &routeConfigExt)

	rule := &dbmodel.LogAlertRules{
		Name:           req.Name,
		Description:    req.Description,
		ClusterName:    clusterName,
		Enabled:        req.Enabled,
		Priority:       int32(req.Priority),
		LabelSelectors: labelsExt,
		MatchType:      req.MatchType,
		MatchConfig:    matchConfigExt,
		Severity:       req.Severity,
		AlertTemplate:  alertTemplateExt,
		GroupBy:        pgUtil.StringArrayToPgArray(req.GroupBy),
		GroupWait:      req.GroupWait,
		RepeatInterval: req.RepeatInterval,
		RouteConfig:    routeConfigExt,
		CreatedBy:      req.CreatedBy,
	}

	// Set defaults
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	if rule.Priority == 0 {
		rule.Priority = 5
	}
	if rule.GroupWait == 0 {
		rule.GroupWait = 30
	}
	if rule.RepeatInterval == 0 {
		rule.RepeatInterval = 3600
	}

	return rule
}

func updateRuleFromRequest(rule *dbmodel.LogAlertRules, req *LogAlertRuleRequest) {
	if req.Name != "" {
		rule.Name = req.Name
	}
	if req.Description != "" {
		rule.Description = req.Description
	}
	rule.Enabled = req.Enabled
	if req.Priority > 0 {
		rule.Priority = int32(req.Priority)
	}
	if len(req.LabelSelectors) > 0 {
		labelsBytes, _ := json.Marshal(req.LabelSelectors)
		var labelsExt dbmodel.ExtType
		json.Unmarshal(labelsBytes, &labelsExt)
		rule.LabelSelectors = labelsExt
	}
	if req.MatchType != "" {
		rule.MatchType = req.MatchType
	}
	if len(req.MatchConfig) > 0 {
		matchConfigBytes, _ := json.Marshal(req.MatchConfig)
		var matchConfigExt dbmodel.ExtType
		json.Unmarshal(matchConfigBytes, &matchConfigExt)
		rule.MatchConfig = matchConfigExt
	}
	if req.Severity != "" {
		rule.Severity = req.Severity
	}
	if len(req.AlertTemplate) > 0 {
		alertTemplateBytes, _ := json.Marshal(req.AlertTemplate)
		var alertTemplateExt dbmodel.ExtType
		json.Unmarshal(alertTemplateBytes, &alertTemplateExt)
		rule.AlertTemplate = alertTemplateExt
	}
	if len(req.GroupBy) > 0 {
		rule.GroupBy = pgUtil.StringArrayToPgArray(req.GroupBy)
	}
	if req.GroupWait > 0 {
		rule.GroupWait = req.GroupWait
	}
	if req.RepeatInterval > 0 {
		rule.RepeatInterval = req.RepeatInterval
	}
	if len(req.RouteConfig) > 0 {
		routeConfigBytes, _ := json.Marshal(req.RouteConfig)
		var routeConfigExt dbmodel.ExtType
		json.Unmarshal(routeConfigBytes, &routeConfigExt)
		rule.RouteConfig = routeConfigExt
	}
	if req.UpdatedBy != "" {
		rule.UpdatedBy = req.UpdatedBy
	}
}

func createRuleVersion(rule *dbmodel.LogAlertRules, version int, changeLog, createdBy string) *dbmodel.LogAlertRuleVersions {
	configBytes, _ := json.Marshal(rule)
	var configExt dbmodel.ExtType
	json.Unmarshal(configBytes, &configExt)

	now := time.Now()
	return &dbmodel.LogAlertRuleVersions{
		RuleID:     rule.ID,
		Version:    int32(version),
		Config:     configExt,
		Status:     "active",
		DeployedAt: now,
		CreatedBy:  createdBy,
		ChangeLog:  changeLog,
	}
}
