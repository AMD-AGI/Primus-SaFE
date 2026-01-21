// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/vmrule"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

const (
	// Sync status constants
	SyncStatusPending = "pending"
	SyncStatusSynced  = "synced"
	SyncStatusFailed  = "failed"

	// Default VMRule namespace
	DefaultVMRuleNamespace = "primus-lens"
)

// MetricAlertRuleRequest represents a request to create or update a metric alert rule
type MetricAlertRuleRequest struct {
	Name            string                       `json:"name" binding:"required"`
	ClusterName     string                       `json:"cluster_name" binding:"required"`
	Enabled         bool                         `json:"enabled"`
	Groups          []model.VMRuleGroup          `json:"groups" binding:"required"`
	Description     string                       `json:"description"`
	Labels          map[string]string            `json:"labels,omitempty"`
	Namespace       string                       `json:"namespace,omitempty"` // VMRule namespace, defaults to primus-lens
	AutoSync        bool                         `json:"auto_sync"`           // Auto sync to cluster after creation/update
	ResourceMapping *model.ResourceMappingConfig `json:"resource_mapping,omitempty"`
	AlertEnrichment *model.AlertEnrichmentConfig `json:"alert_enrichment,omitempty"`
	AlertGrouping   *model.AlertGroupingConfig   `json:"alert_grouping,omitempty"`
	AlertRouting    *model.AlertRoutingConfig    `json:"alert_routing,omitempty"`
}

// MetricAlertRuleResponse represents a metric alert rule response
type MetricAlertRuleResponse struct {
	*dbmodel.MetricAlertRules
	SyncEnabled bool `json:"sync_enabled"` // Whether sync is enabled for this rule
}

// CloneRuleRequest represents a request to clone a rule to another cluster
type CloneRuleRequest struct {
	TargetClusterName string `json:"target_cluster_name" binding:"required"`
	NewName           string `json:"new_name"`  // Optional: new rule name
	AutoSync          bool   `json:"auto_sync"` // Auto sync to target cluster
}

// SyncRuleRequest represents a request to sync rule(s) to cluster
type SyncRuleRequest struct {
	RuleIDs []int64 `json:"rule_ids"` // Empty means sync all enabled rules in cluster
}

// CreateMetricAlertRule handles POST /api/metric-alert-rules
func CreateMetricAlertRule(c *gin.Context) {
	var req MetricAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate groups
	if len(req.Groups) == 0 {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "at least one rule group is required", nil))
		return
	}

	// Set default namespace if not provided
	namespace := req.Namespace
	if namespace == "" {
		namespace = DefaultVMRuleNamespace
	}

	// Get user from context if available
	createdBy, _ := c.Get("user")
	createdByStr := ""
	if createdBy != nil {
		createdByStr = fmt.Sprintf("%v", createdBy)
	}

	// Create rule model
	var groupsExt dbmodel.ExtJSON
	groupsBytes, err := json.Marshal(req.Groups)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid groups format: "+err.Error(), nil))
		return
	}
	groupsExt = dbmodel.ExtJSON(groupsBytes)

	var labelsExt dbmodel.ExtType
	if req.Labels != nil {
		labelsBytes, err := json.Marshal(req.Labels)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid labels format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(labelsBytes, &labelsExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid labels format: "+err.Error(), nil))
			return
		}
	}

	// Convert resource mapping to ExtType
	var resourceMappingExt dbmodel.ExtType
	if req.ResourceMapping != nil {
		resourceMappingBytes, err := json.Marshal(req.ResourceMapping)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid resource_mapping format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(resourceMappingBytes, &resourceMappingExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid resource_mapping format: "+err.Error(), nil))
			return
		}
	}

	// Convert alert enrichment to ExtType
	var alertEnrichmentExt dbmodel.ExtType
	if req.AlertEnrichment != nil {
		alertEnrichmentBytes, err := json.Marshal(req.AlertEnrichment)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid alert_enrichment format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(alertEnrichmentBytes, &alertEnrichmentExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid alert_enrichment format: "+err.Error(), nil))
			return
		}
	}

	// Convert alert grouping to ExtType
	var alertGroupingExt dbmodel.ExtType
	if req.AlertGrouping != nil {
		alertGroupingBytes, err := json.Marshal(req.AlertGrouping)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid alert_grouping format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(alertGroupingBytes, &alertGroupingExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid alert_grouping format: "+err.Error(), nil))
			return
		}
	}

	// Convert alert routing to ExtType
	var alertRoutingExt dbmodel.ExtType
	if req.AlertRouting != nil {
		alertRoutingBytes, err := json.Marshal(req.AlertRouting)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid alert_routing format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(alertRoutingBytes, &alertRoutingExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid alert_routing format: "+err.Error(), nil))
			return
		}
	}

	rule := &dbmodel.MetricAlertRules{
		Name:            req.Name,
		ClusterName:     req.ClusterName,
		Enabled:         req.Enabled,
		Groups:          groupsExt,
		Description:     req.Description,
		Labels:          labelsExt,
		ResourceMapping: resourceMappingExt,
		AlertEnrichment: alertEnrichmentExt,
		AlertGrouping:   alertGroupingExt,
		AlertRouting:    alertRoutingExt,
		SyncStatus:      SyncStatusPending,
		CreatedBy:       createdByStr,
	}

	// Check if rule already exists
	facade := database.GetFacade().GetMetricAlertRule()
	existing, err := facade.GetMetricAlertRuleByNameAndCluster(c.Request.Context(), req.Name, req.ClusterName)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to check existing rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, rest.ErrorResp(c.Request.Context(), http.StatusConflict, "rule with same name already exists in this cluster", nil))
		return
	}

	// Save to database
	if err := facade.CreateMetricAlertRule(c.Request.Context(), rule); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to create metric alert rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Auto sync if requested
	if req.AutoSync && req.Enabled {
		go syncRuleToCluster(rule, namespace)
	}

	c.JSON(http.StatusCreated, rest.SuccessResp(c.Request.Context(), gin.H{
		"rule_id": rule.ID,
		"message": "metric alert rule created successfully",
	}))
}

// ListMetricAlertRules handles GET /api/metric-alert-rules
func ListMetricAlertRules(c *gin.Context) {
	name := c.Query("name")
	clusterName := c.Query("cluster_name")
	enabledStr := c.Query("enabled")
	syncStatus := c.Query("sync_status")

	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := &database.MetricAlertRuleFilter{
		Offset: (pageNum - 1) * pageSize,
		Limit:  pageSize,
	}

	if name != "" {
		filter.Name = &name
	}
	if clusterName != "" {
		filter.ClusterName = &clusterName
	}
	if enabledStr != "" {
		enabled, err := strconv.ParseBool(enabledStr)
		if err == nil {
			filter.Enabled = &enabled
		}
	}
	if syncStatus != "" {
		filter.SyncStatus = &syncStatus
	}

	facade := database.GetFacade().GetMetricAlertRule()
	rules, total, err := facade.ListMetricAlertRules(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to list metric alert rules: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"data":     rules,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}))
}

// GetMetricAlertRule handles GET /api/metric-alert-rules/:id
func GetMetricAlertRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	facade := database.GetFacade().GetMetricAlertRule()
	rule, err := facade.GetMetricAlertRuleByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get metric alert rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if rule == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), rule))
}

// UpdateMetricAlertRule handles PUT /api/metric-alert-rules/:id
func UpdateMetricAlertRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	var req MetricAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get existing rule
	facade := database.GetFacade().GetMetricAlertRule()
	rule, err := facade.GetMetricAlertRuleByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get metric alert rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if rule == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	// Get user from context if available
	updatedBy, _ := c.Get("user")
	updatedByStr := ""
	if updatedBy != nil {
		updatedByStr = fmt.Sprintf("%v", updatedBy)
	}

	// Update fields
	rule.Name = req.Name
	rule.Enabled = req.Enabled
	rule.Description = req.Description
	rule.UpdatedBy = updatedByStr

	if len(req.Groups) > 0 {
		groupsBytes, err := json.Marshal(req.Groups)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid groups format: "+err.Error(), nil))
			return
		}
		rule.Groups = dbmodel.ExtJSON(groupsBytes)
	}

	if req.Labels != nil {
		var labelsExt dbmodel.ExtType
		labelsBytes, err := json.Marshal(req.Labels)
		if err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid labels format: "+err.Error(), nil))
			return
		}
		if err := json.Unmarshal(labelsBytes, &labelsExt); err != nil {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid labels format: "+err.Error(), nil))
			return
		}
		rule.Labels = labelsExt
	}

	// Update resource mapping
	if req.ResourceMapping != nil {
		var resourceMappingExt dbmodel.ExtType
		resourceMappingBytes, _ := json.Marshal(req.ResourceMapping)
		json.Unmarshal(resourceMappingBytes, &resourceMappingExt)
		rule.ResourceMapping = resourceMappingExt
	}

	// Update alert enrichment
	if req.AlertEnrichment != nil {
		var alertEnrichmentExt dbmodel.ExtType
		alertEnrichmentBytes, _ := json.Marshal(req.AlertEnrichment)
		json.Unmarshal(alertEnrichmentBytes, &alertEnrichmentExt)
		rule.AlertEnrichment = alertEnrichmentExt
	}

	// Update alert grouping
	if req.AlertGrouping != nil {
		var alertGroupingExt dbmodel.ExtType
		alertGroupingBytes, _ := json.Marshal(req.AlertGrouping)
		json.Unmarshal(alertGroupingBytes, &alertGroupingExt)
		rule.AlertGrouping = alertGroupingExt
	}

	// Update alert routing
	if req.AlertRouting != nil {
		var alertRoutingExt dbmodel.ExtType
		alertRoutingBytes, _ := json.Marshal(req.AlertRouting)
		json.Unmarshal(alertRoutingBytes, &alertRoutingExt)
		rule.AlertRouting = alertRoutingExt
	}

	// Mark as pending sync after update
	rule.SyncStatus = SyncStatusPending

	// Save to database
	if err := facade.UpdateMetricAlertRule(c.Request.Context(), rule); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to update metric alert rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Auto sync if requested
	namespace := req.Namespace
	if namespace == "" {
		namespace = DefaultVMRuleNamespace
	}
	if req.AutoSync && req.Enabled {
		go syncRuleToCluster(rule, namespace)
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"rule_id": rule.ID,
		"message": "metric alert rule updated successfully",
	}))
}

// DeleteMetricAlertRule handles DELETE /api/metric-alert-rules/:id
func DeleteMetricAlertRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	// Get rule to check cluster and name for deletion
	facade := database.GetFacade().GetMetricAlertRule()
	rule, err := facade.GetMetricAlertRuleByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get metric alert rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if rule == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	// Delete from database
	if err := facade.DeleteMetricAlertRule(c.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to delete metric alert rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Try to delete VMRule from cluster (best effort)
	namespace := c.DefaultQuery("namespace", DefaultVMRuleNamespace)
	go deleteVMRuleFromCluster(rule.ClusterName, rule.Name, namespace)

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"message": "metric alert rule deleted successfully",
	}))
}

// CloneMetricAlertRule handles POST /api/metric-alert-rules/:id/clone
func CloneMetricAlertRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	var req CloneRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get source rule
	facade := database.GetFacade().GetMetricAlertRule()
	sourceRule, err := facade.GetMetricAlertRuleByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get source rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if sourceRule == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "source rule not found", nil))
		return
	}

	// Get user from context if available
	createdBy, _ := c.Get("user")
	createdByStr := ""
	if createdBy != nil {
		createdByStr = fmt.Sprintf("%v", createdBy)
	}

	// Create new rule for target cluster
	newName := req.NewName
	if newName == "" {
		newName = sourceRule.Name
	}

	// Check if rule already exists in target cluster
	existing, err := facade.GetMetricAlertRuleByNameAndCluster(c.Request.Context(), newName, req.TargetClusterName)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to check existing rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, rest.ErrorResp(c.Request.Context(), http.StatusConflict, "rule with same name already exists in target cluster", nil))
		return
	}

	newRule := &dbmodel.MetricAlertRules{
		Name:            newName,
		ClusterName:     req.TargetClusterName,
		Enabled:         sourceRule.Enabled,
		Groups:          sourceRule.Groups,
		Description:     fmt.Sprintf("Cloned from %s in cluster %s", sourceRule.Name, sourceRule.ClusterName),
		Labels:          sourceRule.Labels,
		ResourceMapping: sourceRule.ResourceMapping,
		AlertEnrichment: sourceRule.AlertEnrichment,
		AlertGrouping:   sourceRule.AlertGrouping,
		AlertRouting:    sourceRule.AlertRouting,
		SyncStatus:      SyncStatusPending,
		CreatedBy:       createdByStr,
	}

	// Save to database
	if err := facade.CreateMetricAlertRule(c.Request.Context(), newRule); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to clone metric alert rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Auto sync if requested
	if req.AutoSync && newRule.Enabled {
		namespace := c.DefaultQuery("namespace", DefaultVMRuleNamespace)
		go syncRuleToCluster(newRule, namespace)
	}

	c.JSON(http.StatusCreated, rest.SuccessResp(c.Request.Context(), gin.H{
		"rule_id": newRule.ID,
		"message": "metric alert rule cloned successfully",
	}))
}

// SyncMetricAlertRulesToCluster handles POST /api/metric-alert-rules/sync
func SyncMetricAlertRulesToCluster(c *gin.Context) {
	var req SyncRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	clusterName := c.Query("cluster_name")
	namespace := c.DefaultQuery("namespace", DefaultVMRuleNamespace)

	facade := database.GetFacade().GetMetricAlertRule()
	var rulesToSync []*dbmodel.MetricAlertRules
	var err error

	if len(req.RuleIDs) > 0 {
		// Sync specific rules
		for _, ruleID := range req.RuleIDs {
			rule, err := facade.GetMetricAlertRuleByID(c.Request.Context(), ruleID)
			if err != nil {
				log.GlobalLogger().WithContext(c).Errorf("Failed to get rule %d: %v", ruleID, err)
				continue
			}
			if rule != nil && rule.Enabled {
				rulesToSync = append(rulesToSync, rule)
			}
		}
	} else if clusterName != "" {
		// Sync all enabled rules in cluster
		enabled := true
		rulesToSync, err = facade.ListRulesByCluster(c.Request.Context(), clusterName, &enabled)
		if err != nil {
			log.GlobalLogger().WithContext(c).Errorf("Failed to list rules for cluster: %v", err)
			c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "either rule_ids or cluster_name must be provided", nil))
		return
	}

	// Sync rules
	successCount := 0
	failCount := 0
	for _, rule := range rulesToSync {
		if err := syncRuleToClusterSync(rule, namespace); err != nil {
			log.GlobalLogger().WithContext(c).Errorf("Failed to sync rule %d: %v", rule.ID, err)
			failCount++
		} else {
			successCount++
		}
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"total":   len(rulesToSync),
		"success": successCount,
		"failed":  failCount,
		"message": fmt.Sprintf("synced %d rules successfully, %d failed", successCount, failCount),
	}))
}

// GetVMRuleStatus handles GET /api/metric-alert-rules/:id/status
func GetVMRuleStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	// Get rule from database
	facade := database.GetFacade().GetMetricAlertRule()
	rule, err := facade.GetMetricAlertRuleByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get metric alert rule: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if rule == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	// Fetch latest status from Kubernetes
	namespace := c.DefaultQuery("namespace", DefaultVMRuleNamespace)
	status, err := fetchVMRuleStatus(rule.ClusterName, rule.Name, namespace)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to fetch VMRule status from cluster: %v", err)
		// Return database status if K8s fetch fails
		c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
			"rule_id":       rule.ID,
			"name":          rule.Name,
			"cluster_name":  rule.ClusterName,
			"sync_status":   rule.SyncStatus,
			"sync_message":  rule.SyncMessage,
			"last_sync_at":  rule.LastSyncAt,
			"vmrule_status": rule.VmruleStatus,
			"status_source": "database",
		}))
		return
	}

	// Update status in database
	var statusExt dbmodel.ExtType
	statusBytes, err := json.Marshal(status)
	if err == nil {
		if err := json.Unmarshal(statusBytes, &statusExt); err == nil {
			facade.UpdateVMRuleStatus(c.Request.Context(), id, statusExt)
		}
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"rule_id":       rule.ID,
		"name":          rule.Name,
		"cluster_name":  rule.ClusterName,
		"sync_status":   rule.SyncStatus,
		"sync_message":  rule.SyncMessage,
		"last_sync_at":  rule.LastSyncAt,
		"vmrule_status": status,
		"status_source": "kubernetes",
	}))
}

// syncRuleToCluster syncs a rule to its target cluster (async)
func syncRuleToCluster(rule *dbmodel.MetricAlertRules, namespace string) {
	_ = syncRuleToClusterSync(rule, namespace)
}

// syncRuleToClusterSync syncs a rule to its target cluster (sync)
func syncRuleToClusterSync(rule *dbmodel.MetricAlertRules, namespace string) error {
	// Create VMRule manager
	manager, err := vmrule.NewVMRuleManager(rule.ClusterName, namespace)
	if err != nil {
		log.GlobalLogger().Errorf("Failed to create VMRule manager for cluster %s: %v", rule.ClusterName, err)
		// Update sync status to failed
		facade := database.GetFacade().GetMetricAlertRule()
		facade.UpdateSyncStatus(context.TODO(), rule.ID, SyncStatusFailed, fmt.Sprintf("Failed to create VMRule manager: %v", err))
		return err
	}

	// Create or update VMRule in cluster
	if err := manager.CreateOrUpdateVMRule(context.TODO(), rule); err != nil {
		log.GlobalLogger().Errorf("Failed to sync VMRule to cluster %s: %v", rule.ClusterName, err)
		// Update sync status to failed
		facade := database.GetFacade().GetMetricAlertRule()
		facade.UpdateSyncStatus(context.TODO(), rule.ID, SyncStatusFailed, fmt.Sprintf("Failed to sync to cluster: %v", err))
		return err
	}

	// Update sync status to synced
	facade := database.GetFacade().GetMetricAlertRule()
	if err := facade.UpdateSyncStatus(context.TODO(), rule.ID, SyncStatusSynced, "Successfully synced to cluster"); err != nil {
		log.GlobalLogger().Errorf("Failed to update sync status: %v", err)
	}

	// Update rule in database with new UID
	if err := facade.UpdateMetricAlertRule(context.TODO(), rule); err != nil {
		log.GlobalLogger().Errorf("Failed to update rule with VMRule UID: %v", err)
	}

	log.GlobalLogger().Infof("Successfully synced metric alert rule %d to cluster %s", rule.ID, rule.ClusterName)
	return nil
}

// deleteVMRuleFromCluster deletes a VMRule from cluster (async)
func deleteVMRuleFromCluster(clusterName, ruleName, namespace string) {
	manager, err := vmrule.NewVMRuleManager(clusterName, namespace)
	if err != nil {
		log.GlobalLogger().Errorf("Failed to create VMRule manager for cluster %s: %v", clusterName, err)
		return
	}

	if err := manager.DeleteVMRule(context.TODO(), ruleName); err != nil {
		log.GlobalLogger().Errorf("Failed to delete VMRule from cluster %s: %v", clusterName, err)
		return
	}

	log.GlobalLogger().Infof("Successfully deleted VMRule %s from cluster %s", ruleName, clusterName)
}

// fetchVMRuleStatus fetches VMRule status from Kubernetes
func fetchVMRuleStatus(clusterName, ruleName, namespace string) (*model.VMRuleStatus, error) {
	manager, err := vmrule.NewVMRuleManager(clusterName, namespace)
	if err != nil {
		return nil, err
	}

	status, err := manager.GetVMRuleStatus(context.TODO(), ruleName)
	if err != nil {
		return nil, err
	}

	return status, nil
}
