package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aiclient"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/backfill"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ========== Request/Response Types ==========

// GithubWorkflowConfigRequest represents the request body for creating/updating a config
type GithubWorkflowConfigRequest struct {
	Name               string   `json:"name" binding:"required"`
	Description        string   `json:"description"`
	RunnerSetNamespace string   `json:"runner_set_namespace" binding:"required"`
	RunnerSetName      string   `json:"runner_set_name" binding:"required"`
	RunnerSetUID       string   `json:"runner_set_uid"`
	GithubOwner        string   `json:"github_owner" binding:"required"`
	GithubRepo         string   `json:"github_repo" binding:"required"`
	WorkflowFilter     string   `json:"workflow_filter"`
	BranchFilter       string   `json:"branch_filter"`
	FilePatterns       []string `json:"file_patterns" binding:"required"`
	Enabled            *bool    `json:"enabled"`
}

// GithubWorkflowSchemaRequest represents the request body for creating/updating a schema
type GithubWorkflowSchemaRequest struct {
	Name            string   `json:"name" binding:"required"`
	Fields          []Field  `json:"fields" binding:"required"`
	DimensionFields []string `json:"dimension_fields"`
	MetricFields    []string `json:"metric_fields"`
	IsActive        *bool    `json:"is_active"`
}

// Field represents a field definition in a schema
type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
}

// ========== Config Handlers ==========

// CreateGithubWorkflowConfig handles POST /v1/github-workflow-metrics/configs
func CreateGithubWorkflowConfig(ctx *gin.Context) {
	var req GithubWorkflowConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Build config model
	filePatterns, _ := json.Marshal(req.FilePatterns)
	config := &dbmodel.GithubWorkflowConfigs{
		Name:               req.Name,
		Description:        req.Description,
		RunnerSetNamespace: req.RunnerSetNamespace,
		RunnerSetName:      req.RunnerSetName,
		RunnerSetUID:       req.RunnerSetUID,
		GithubOwner:        req.GithubOwner,
		GithubRepo:         req.GithubRepo,
		WorkflowFilter:     req.WorkflowFilter,
		BranchFilter:       req.BranchFilter,
		FilePatterns:       dbmodel.ExtJSON(filePatterns),
		ClusterName:        clusterName,
		Enabled:            true,
	}
	if req.Enabled != nil {
		config.Enabled = *req.Enabled
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	if err := facade.Create(ctx.Request.Context(), config); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to create config", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Created github workflow config: %s (ID: %d)", config.Name, config.ID)
	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"config_id":    config.ID,
		"cluster_name": clusterName,
	}))
}

// ListGithubWorkflowConfigs handles GET /v1/github-workflow-metrics/configs
func ListGithubWorkflowConfigs(ctx *gin.Context) {
	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	filter := &database.GithubWorkflowConfigFilter{
		ClusterName: clusterName,
	}

	if enabledStr := ctx.Query("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		filter.Enabled = &enabled
	}
	if owner := ctx.Query("github_owner"); owner != "" {
		filter.GithubOwner = owner
	}
	if repo := ctx.Query("github_repo"); repo != "" {
		filter.GithubRepo = repo
	}
	if offset, err := strconv.Atoi(ctx.Query("offset")); err == nil {
		filter.Offset = offset
	}
	if limit, err := strconv.Atoi(ctx.Query("limit")); err == nil {
		filter.Limit = limit
	} else {
		filter.Limit = 20
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	configs, total, err := facade.List(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list github workflow configs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list configs", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"configs":      configs,
		"total":        total,
		"offset":       filter.Offset,
		"limit":        filter.Limit,
		"cluster_name": clusterName,
	}))
}

// GetGithubWorkflowConfig handles GET /v1/github-workflow-metrics/configs/:id
func GetGithubWorkflowConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := facade.GetByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get config", err))
		return
	}
	if config == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "config not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), config))
}

// UpdateGithubWorkflowConfig handles PUT /v1/github-workflow-metrics/configs/:id
func UpdateGithubWorkflowConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config id", nil))
		return
	}

	var req GithubWorkflowConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()

	// Get existing config
	config, err := facade.GetByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get config", err))
		return
	}
	if config == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "config not found", nil))
		return
	}

	// Update fields
	config.Name = req.Name
	config.Description = req.Description
	config.RunnerSetNamespace = req.RunnerSetNamespace
	config.RunnerSetName = req.RunnerSetName
	config.RunnerSetUID = req.RunnerSetUID
	config.GithubOwner = req.GithubOwner
	config.GithubRepo = req.GithubRepo
	config.WorkflowFilter = req.WorkflowFilter
	config.BranchFilter = req.BranchFilter
	filePatterns, _ := json.Marshal(req.FilePatterns)
	config.FilePatterns = dbmodel.ExtJSON(filePatterns)
	if req.Enabled != nil {
		config.Enabled = *req.Enabled
	}

	if err := facade.Update(ctx.Request.Context(), config); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to update config", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Updated github workflow config: %s (ID: %d)", config.Name, config.ID)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), config))
}

// DeleteGithubWorkflowConfig handles DELETE /v1/github-workflow-metrics/configs/:id
func DeleteGithubWorkflowConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	if err := facade.Delete(ctx.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to delete config", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Deleted github workflow config ID: %d", id)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{"deleted": true}))
}

// ========== Run Handlers ==========

// ListGithubWorkflowRuns handles GET /v1/github-workflow-metrics/configs/:config_id/runs
func ListGithubWorkflowRuns(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	filter := &database.GithubWorkflowRunFilter{
		ConfigID: configID,
	}

	if status := ctx.Query("status"); status != "" {
		filter.Status = status
	}
	if triggerSource := ctx.Query("trigger_source"); triggerSource != "" {
		filter.TriggerSource = triggerSource
	}
	if offset, err := strconv.Atoi(ctx.Query("offset")); err == nil {
		filter.Offset = offset
	}
	if limit, err := strconv.Atoi(ctx.Query("limit")); err == nil {
		filter.Limit = limit
	} else {
		filter.Limit = 20
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := facade.List(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list github workflow runs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list runs", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"runs":   runs,
		"total":  total,
		"offset": filter.Offset,
		"limit":  filter.Limit,
	}))
}

// GetGithubWorkflowRun handles GET /v1/github-workflow-metrics/runs/:id
func GetGithubWorkflowRun(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid run id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	run, err := facade.GetByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow run: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get run", err))
		return
	}
	if run == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "run not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), run))
}

// ========== Schema Handlers ==========

// CreateGithubWorkflowSchema handles POST /v1/github-workflow-metrics/configs/:config_id/schemas
func CreateGithubWorkflowSchema(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	var req GithubWorkflowSchemaRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Build schema model
	fields, _ := json.Marshal(req.Fields)
	dimensionFields, _ := json.Marshal(req.DimensionFields)
	metricFields, _ := json.Marshal(req.MetricFields)

	schema := &dbmodel.GithubWorkflowMetricSchemas{
		ConfigID:        configID,
		Name:            req.Name,
		Fields:          dbmodel.ExtJSON(fields),
		DimensionFields: dbmodel.ExtJSON(dimensionFields),
		MetricFields:    dbmodel.ExtJSON(metricFields),
		IsActive:        true,
		GeneratedBy:     database.SchemaGeneratedByUser,
	}
	if req.IsActive != nil {
		schema.IsActive = *req.IsActive
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	if err := facade.Create(ctx.Request.Context(), schema); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create github workflow schema: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to create schema", err))
		return
	}

	// If this schema is active, update the config's metric_schema_id
	if schema.IsActive {
		configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
		if err := configFacade.UpdateMetricSchemaID(ctx.Request.Context(), configID, schema.ID); err != nil {
			log.GlobalLogger().WithContext(ctx).Warningf("Failed to update config metric_schema_id: %v", err)
		}
	}

	log.GlobalLogger().WithContext(ctx).Infof("Created github workflow schema: %s (ID: %d)", schema.Name, schema.ID)
	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"schema_id": schema.ID,
		"version":   schema.Version,
	}))
}

// ListGithubWorkflowSchemas handles GET /v1/github-workflow-metrics/configs/:config_id/schemas
func ListGithubWorkflowSchemas(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	schemas, err := facade.ListByConfig(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list github workflow schemas: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list schemas", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"schemas": schemas,
		"total":   len(schemas),
	}))
}

// GetGithubWorkflowSchema handles GET /v1/github-workflow-metrics/schemas/:id
func GetGithubWorkflowSchema(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid schema id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	schema, err := facade.GetByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow schema: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get schema", err))
		return
	}
	if schema == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "schema not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), schema))
}

// SetGithubWorkflowSchemaActive handles POST /v1/github-workflow-metrics/schemas/:id/activate
func SetGithubWorkflowSchemaActive(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid schema id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()

	// Get schema to find config_id
	schema, err := facade.GetByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow schema: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get schema", err))
		return
	}
	if schema == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "schema not found", nil))
		return
	}

	// Set this schema as active
	if err := facade.SetActive(ctx.Request.Context(), schema.ConfigID, id); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to set schema active: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to set schema active", err))
		return
	}

	// Update config's metric_schema_id
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	if err := configFacade.UpdateMetricSchemaID(ctx.Request.Context(), schema.ConfigID, id); err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to update config metric_schema_id: %v", err)
	}

	log.GlobalLogger().WithContext(ctx).Infof("Set github workflow schema %d as active for config %d", id, schema.ConfigID)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{"activated": true}))
}

// ========== Metrics Handlers ==========

// ListGithubWorkflowMetrics handles GET /v1/github-workflow-metrics/configs/:config_id/metrics
func ListGithubWorkflowMetrics(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	filter := &database.GithubWorkflowMetricsFilter{
		ConfigID: configID,
	}

	if runIDStr := ctx.Query("run_id"); runIDStr != "" {
		if runID, err := strconv.ParseInt(runIDStr, 10, 64); err == nil {
			filter.RunID = runID
		}
	}
	if schemaIDStr := ctx.Query("schema_id"); schemaIDStr != "" {
		if schemaID, err := strconv.ParseInt(schemaIDStr, 10, 64); err == nil {
			filter.SchemaID = schemaID
		}
	}
	if startStr := ctx.Query("start"); startStr != "" {
		if start, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.Start = &start
		}
	}
	if endStr := ctx.Query("end"); endStr != "" {
		if end, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.End = &end
		}
	}
	if offset, err := strconv.Atoi(ctx.Query("offset")); err == nil {
		filter.Offset = offset
	}
	if limit, err := strconv.Atoi(ctx.Query("limit")); err == nil {
		filter.Limit = limit
	} else {
		filter.Limit = 100
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	metrics, total, err := facade.List(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list github workflow metrics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list metrics", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"metrics": metrics,
		"total":   total,
		"offset":  filter.Offset,
		"limit":   filter.Limit,
	}))
}

// GetGithubWorkflowMetricsByRun handles GET /v1/github-workflow-metrics/runs/:run_id/metrics
func GetGithubWorkflowMetricsByRun(ctx *gin.Context) {
	runIDStr := ctx.Param("id")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid run_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	metrics, err := facade.ListByRun(ctx.Request.Context(), runID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list github workflow metrics by run: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list metrics", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"metrics": metrics,
		"total":   len(metrics),
	}))
}

// GetGithubWorkflowMetricsStats handles GET /v1/github-workflow-metrics/configs/:config_id/stats
func GetGithubWorkflowMetricsStats(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get config
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := configFacade.GetByID(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get config", err))
		return
	}
	if config == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "config not found", nil))
		return
	}

	// Get run counts by status
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	pendingRuns, _, _ := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Status:   database.WorkflowRunStatusPending,
		Limit:    0,
	})
	completedRuns, _, _ := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Status:   database.WorkflowRunStatusCompleted,
		Limit:    0,
	})
	failedRuns, _, _ := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Status:   database.WorkflowRunStatusFailed,
		Limit:    0,
	})

	// Get total metrics count
	metricsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	totalMetrics, _ := metricsFacade.CountByConfig(ctx.Request.Context(), configID)

	// Get active schema
	schemaFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	activeSchema, _ := schemaFacade.GetActiveByConfig(ctx.Request.Context(), configID)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"config_id":        configID,
		"config_name":      config.Name,
		"enabled":          config.Enabled,
		"pending_runs":     len(pendingRuns),
		"completed_runs":   len(completedRuns),
		"failed_runs":      len(failedRuns),
		"total_metrics":    totalMetrics,
		"active_schema_id": getSchemaID(activeSchema),
		"last_checked_at":  config.LastCheckedAt,
	}))
}

func getSchemaID(schema *dbmodel.GithubWorkflowMetricSchemas) int64 {
	if schema == nil {
		return 0
	}
	return schema.ID
}

// BackfillRequest represents the request body for triggering backfill
type BackfillRequest struct {
	StartTime    string   `json:"start_time" binding:"required"`
	EndTime      string   `json:"end_time" binding:"required"`
	WorkloadUIDs []string `json:"workload_uids,omitempty"`
	DryRun       bool     `json:"dry_run,omitempty"`
}

// TriggerBackfill handles POST /v1/github-workflow-metrics/configs/:config_id/backfill
func TriggerBackfill(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	var req BackfillRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid start_time format, expected RFC3339", nil))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid end_time format, expected RFC3339", nil))
		return
	}

	if endTime.Before(startTime) {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "end_time must be after start_time", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Verify config exists
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := configFacade.GetByID(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get config", err))
		return
	}
	if config == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "config not found", nil))
		return
	}

	// Create backfill task
	taskManager := backfill.GetTaskManager()
	task := taskManager.CreateTask(configID, startTime, endTime, req.WorkloadUIDs, clusterName, req.DryRun)

	log.GlobalLogger().WithContext(ctx).Infof("Backfill task %s created for config %d (dry_run=%v)", task.ID, configID, req.DryRun)

	ctx.JSON(http.StatusAccepted, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message":   "Backfill task created",
		"task_id":   task.ID,
		"config_id": configID,
		"status":    task.Status,
	}))
}

// GetBackfillStatus handles GET /v1/github-workflow-metrics/configs/:config_id/backfill/status
func GetBackfillStatus(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	taskManager := backfill.GetTaskManager()
	tasks := taskManager.GetTasksByConfig(configID)

	// Find the most recent task
	var latestTask *backfill.BackfillTask
	for _, task := range tasks {
		if latestTask == nil || task.CreatedAt.After(latestTask.CreatedAt) {
			latestTask = task
		}
	}

	if latestTask == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "no backfill tasks found for this config", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"task_id":        latestTask.ID,
		"config_id":      latestTask.ConfigID,
		"status":         latestTask.Status,
		"total":          latestTask.TotalRuns,
		"processed":      latestTask.ProcessedRuns,
		"failed":         latestTask.FailedRuns,
		"created_at":     latestTask.CreatedAt,
		"started_at":     latestTask.StartedAt,
		"completed_at":   latestTask.CompletedAt,
		"error_message":  latestTask.ErrorMessage,
	}))
}

// CancelBackfill handles POST /v1/github-workflow-metrics/configs/:config_id/backfill/cancel
func CancelBackfill(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	taskManager := backfill.GetTaskManager()
	tasks := taskManager.GetTasksByConfig(configID)

	// Find in-progress or pending tasks
	cancelled := 0
	for _, task := range tasks {
		if task.Status == backfill.BackfillStatusPending || task.Status == backfill.BackfillStatusInProgress {
			if err := taskManager.CancelTask(task.ID); err != nil {
				log.Warnf("Failed to cancel task %s: %v", task.ID, err)
			} else {
				cancelled++
			}
		}
	}

	if cancelled == 0 {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "no active backfill tasks found", nil))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Cancelled %d backfill tasks for config %d", cancelled, configID)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message":   "Backfill cancelled",
		"cancelled": cancelled,
	}))
}

// ListBackfillTasks handles GET /v1/github-workflow-metrics/configs/:config_id/backfill/tasks
func ListBackfillTasks(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	taskManager := backfill.GetTaskManager()
	tasks := taskManager.GetTasksByConfig(configID)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"tasks": tasks,
		"total": len(tasks),
	}))
}

// RetryFailedRuns handles POST /v1/github-workflow-metrics/configs/:config_id/runs/batch-retry
func RetryFailedRuns(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()

	// Get failed runs
	failedRuns, err := runFacade.ListByConfigAndStatus(ctx.Request.Context(), configID, database.WorkflowRunStatusFailed)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list failed runs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list failed runs", err))
		return
	}

	if len(failedRuns) == 0 {
		ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
			"message": "No failed runs to retry",
			"retried": 0,
		}))
		return
	}

	// Reset failed runs to pending
	retried := 0
	for _, run := range failedRuns {
		if err := runFacade.ResetToPending(ctx.Request.Context(), run.ID); err != nil {
			log.Warnf("Failed to reset run %d: %v", run.ID, err)
			continue
		}
		retried++
	}

	log.GlobalLogger().WithContext(ctx).Infof("Reset %d failed runs to pending for config %d", retried, configID)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "Failed runs reset to pending",
		"retried": retried,
		"total":   len(failedRuns),
	}))
}

// ========== EphemeralRunner Discovery APIs ==========

// ========== AI Schema Generation APIs ==========

// RegenerateSchemaRequest represents the request body for schema regeneration
type RegenerateSchemaRequest struct {
	// SampleFiles are optional sample file contents to use for schema generation
	SampleFiles []SampleFileContent `json:"sample_files,omitempty"`
	// CustomPrompt is an optional custom prompt for AI
	CustomPrompt string `json:"custom_prompt,omitempty"`
}

// SampleFileContent represents a sample file for schema generation
type SampleFileContent struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	FileType string `json:"file_type"`
	Content  string `json:"content"`
}

// RegenerateGithubWorkflowSchema handles POST /v1/github-workflow-metrics/configs/:config_id/schemas/regenerate
// Uses AI to analyze sample files and generate a metric schema
func RegenerateGithubWorkflowSchema(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	var req RegenerateSchemaRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		// Request body is optional
		req = RegenerateSchemaRequest{}
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get config
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := configFacade.GetByID(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get config", err))
		return
	}
	if config == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "config not found", nil))
		return
	}

	// Build AI request
	aiInput := aitopics.ExtractMetricsInput{
		ConfigID:   configID,
		ConfigName: config.Name,
		Files:      make([]aitopics.FileContent, len(req.SampleFiles)),
		Options: &aitopics.ExtractMetricsOptions{
			GenerateSchemaOnly: true,
			IncludeExplanation: true,
		},
		CustomPrompt: req.CustomPrompt,
	}

	for i, f := range req.SampleFiles {
		aiInput.Files[i] = aitopics.FileContent{
			Path:     f.Path,
			Name:     f.Name,
			FileType: f.FileType,
			Content:  f.Content,
		}
	}

	// Check if AI is available
	aiClient := aiclient.GetGlobalClient()
	if aiClient == nil || !aiClient.IsAvailable(ctx.Request.Context(), aitopics.TopicGithubMetricsExtract) {
		log.Warnf("AI client not available for schema generation")
		ctx.JSON(http.StatusServiceUnavailable, rest.ErrorResp(ctx.Request.Context(), http.StatusServiceUnavailable, "AI service not available", nil))
		return
	}

	// Invoke AI to generate schema
	aiCtx := aiclient.WithClusterID(ctx.Request.Context(), clusterName)
	resp, err := aiClient.InvokeSync(aiCtx, aitopics.TopicGithubMetricsExtract, aiInput)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to invoke AI for schema generation: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "AI invocation failed", err))
		return
	}

	if !resp.IsSuccess() {
		log.GlobalLogger().WithContext(ctx).Errorf("AI returned error: %s", resp.Message)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, resp.Message, nil))
		return
	}

	// Parse AI response
	var aiOutput aitopics.ExtractMetricsOutput
	if err := resp.UnmarshalPayload(&aiOutput); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse AI response: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to parse AI response", err))
		return
	}

	if aiOutput.Schema == nil {
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "AI did not generate a schema", nil))
		return
	}

	// Convert AI schema to database schema
	schemaFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()

	fields, _ := json.Marshal(aiOutput.Schema.Fields)
	dimensionFields, _ := json.Marshal(aiOutput.Schema.DimensionFields)
	metricFields, _ := json.Marshal(aiOutput.Schema.MetricFields)

	schema := &dbmodel.GithubWorkflowMetricSchemas{
		ConfigID:        configID,
		Name:            aiOutput.Schema.Name,
		Fields:          dbmodel.ExtJSON(fields),
		DimensionFields: dbmodel.ExtJSON(dimensionFields),
		MetricFields:    dbmodel.ExtJSON(metricFields),
		IsActive:        false, // User must manually activate
		GeneratedBy:     database.SchemaGeneratedByAI,
	}

	if err := schemaFacade.Create(ctx.Request.Context(), schema); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create AI-generated schema: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to save schema", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("AI generated schema %d for config %d", schema.ID, configID)
	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"schema_id":   schema.ID,
		"version":     schema.Version,
		"name":        schema.Name,
		"explanation": aiOutput.Explanation,
		"fields":      aiOutput.Schema.Fields,
	}))
}

// PreviewSchemaExtraction handles POST /v1/github-workflow-metrics/configs/:config_id/schemas/preview
// Uses AI to preview metrics extraction with a given schema
func PreviewSchemaExtraction(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	type PreviewRequest struct {
		SampleFiles []SampleFileContent `json:"sample_files" binding:"required"`
		SchemaID    *int64              `json:"schema_id,omitempty"`
	}

	var req PreviewRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get config
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := configFacade.GetByID(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get config", err))
		return
	}
	if config == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "config not found", nil))
		return
	}

	// Build AI request
	aiInput := aitopics.ExtractMetricsInput{
		ConfigID:   configID,
		ConfigName: config.Name,
		Files:      make([]aitopics.FileContent, len(req.SampleFiles)),
		Options: &aitopics.ExtractMetricsOptions{
			IncludeRawData:     true,
			IncludeExplanation: true,
			MaxRecordsPerFile:  10, // Limit preview records
		},
	}

	for i, f := range req.SampleFiles {
		aiInput.Files[i] = aitopics.FileContent{
			Path:     f.Path,
			Name:     f.Name,
			FileType: f.FileType,
			Content:  f.Content,
		}
	}

	// Get existing schema if specified
	if req.SchemaID != nil {
		schemaFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
		schema, err := schemaFacade.GetByID(ctx.Request.Context(), *req.SchemaID)
		if err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to get schema: %v", err)
			ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get schema", err))
			return
		}
		if schema != nil {
			aiInput.ExistingSchema = convertDBSchemaToAISchema(schema)
		}
	}

	// Check if AI is available
	aiClient := aiclient.GetGlobalClient()
	if aiClient == nil || !aiClient.IsAvailable(ctx.Request.Context(), aitopics.TopicGithubMetricsExtract) {
		log.Warnf("AI client not available for preview")
		ctx.JSON(http.StatusServiceUnavailable, rest.ErrorResp(ctx.Request.Context(), http.StatusServiceUnavailable, "AI service not available", nil))
		return
	}

	// Invoke AI
	aiCtx := aiclient.WithClusterID(ctx.Request.Context(), clusterName)
	resp, err := aiClient.InvokeSync(aiCtx, aitopics.TopicGithubMetricsExtract, aiInput)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to invoke AI for preview: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "AI invocation failed", err))
		return
	}

	if !resp.IsSuccess() {
		log.GlobalLogger().WithContext(ctx).Errorf("AI returned error: %s", resp.Message)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, resp.Message, nil))
		return
	}

	// Parse AI response
	var aiOutput aitopics.ExtractMetricsOutput
	if err := resp.UnmarshalPayload(&aiOutput); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse AI response: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to parse AI response", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"schema":          aiOutput.Schema,
		"metrics":         aiOutput.Metrics,
		"files_processed": aiOutput.FilesProcessed,
		"total_records":   aiOutput.TotalRecords,
		"errors":          aiOutput.Errors,
		"explanation":     aiOutput.Explanation,
	}))
}

// convertDBSchemaToAISchema converts a database schema to AI schema format
func convertDBSchemaToAISchema(dbSchema *dbmodel.GithubWorkflowMetricSchemas) *aitopics.MetricSchema {
	schema := &aitopics.MetricSchema{
		Name:    dbSchema.Name,
		Version: dbSchema.Version,
	}

	// Parse fields
	var fields []aitopics.SchemaField
	if err := dbSchema.Fields.UnmarshalTo(&fields); err == nil {
		schema.Fields = fields
	}

	// Parse dimension fields
	var dimensionFields []string
	if err := dbSchema.DimensionFields.UnmarshalTo(&dimensionFields); err == nil {
		schema.DimensionFields = dimensionFields
	}

	// Parse metric fields
	var metricFields []string
	if err := dbSchema.MetricFields.UnmarshalTo(&metricFields); err == nil {
		schema.MetricFields = metricFields
	}

	return schema
}

// ExtractMetricsWithAI extracts metrics from files using AI
// This is an internal function used by the collector job
func ExtractMetricsWithAI(ctx context.Context, config *dbmodel.GithubWorkflowConfigs, files []aitopics.FileContent, existingSchema *aitopics.MetricSchema) (*aitopics.ExtractMetricsOutput, error) {
	aiClient := aiclient.GetGlobalClient()
	if aiClient == nil {
		return nil, aiclient.ErrAgentUnavailable
	}

	aiInput := aitopics.ExtractMetricsInput{
		ConfigID:       config.ID,
		ConfigName:     config.Name,
		Files:          files,
		ExistingSchema: existingSchema,
		Options: &aitopics.ExtractMetricsOptions{
			IncludeRawData:     false,
			IncludeExplanation: false,
		},
	}

	resp, err := aiClient.InvokeSync(ctx, aitopics.TopicGithubMetricsExtract, aiInput)
	if err != nil {
		return nil, err
	}

	if !resp.IsSuccess() {
		return nil, aiclient.NewAPIError(resp.Code, resp.Message)
	}

	var output aitopics.ExtractMetricsOutput
	if err := resp.UnmarshalPayload(&output); err != nil {
		return nil, err
	}

	return &output, nil
}

// ListEphemeralRunners handles GET /v1/github-workflow-metrics/configs/:config_id/runners
// Lists completed EphemeralRunners for a config
func ListEphemeralRunners(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get config
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := configFacade.GetByID(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get config", err))
		return
	}
	if config == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "config not found", nil))
		return
	}

	// Parse query parameters
	limit := 100
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	since := time.Now().Add(-24 * time.Hour) // Default: last 24 hours
	if sinceStr := ctx.Query("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	// List completed EphemeralRunners
	workloadFacade := database.GetFacadeForCluster(clusterName).GetWorkload()
	runners, err := workloadFacade.ListCompletedWorkloadsByKindAndNamespace(
		ctx.Request.Context(),
		"EphemeralRunner",
		config.RunnerSetNamespace,
		since,
		limit,
	)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list ephemeral runners: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list runners", err))
		return
	}

	// Check which runners already have run records
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	processedUIDs := make(map[string]bool)

	for _, runner := range runners {
		existingRun, _ := runFacade.GetByConfigAndWorkload(ctx.Request.Context(), configID, runner.UID)
		if existingRun != nil {
			processedUIDs[runner.UID] = true
		}
	}

	// Build response
	type RunnerInfo struct {
		UID           string     `json:"uid"`
		Name          string     `json:"name"`
		Namespace     string     `json:"namespace"`
		StartedAt     *time.Time `json:"started_at,omitempty"`
		CompletedAt   *time.Time `json:"completed_at,omitempty"`
		GithubRunID   string     `json:"github_run_id,omitempty"`
		GithubJobID   string     `json:"github_job_id,omitempty"`
		WorkflowName  string     `json:"workflow_name,omitempty"`
		Branch        string     `json:"branch,omitempty"`
		Processed     bool       `json:"processed"`
	}

	runnerInfos := make([]RunnerInfo, 0, len(runners))
	for _, runner := range runners {
		info := RunnerInfo{
			UID:         runner.UID,
			Name:        runner.Name,
			Namespace:   runner.Namespace,
			Processed:   processedUIDs[runner.UID],
		}

		if !runner.CreatedAt.IsZero() {
			info.StartedAt = &runner.CreatedAt
		}
		if !runner.EndAt.IsZero() {
			info.CompletedAt = &runner.EndAt
		}

		// Extract annotations
		if runner.Annotations != nil {
			if v, ok := runner.Annotations["actions.github.com/run-id"].(string); ok {
				info.GithubRunID = v
			}
			if v, ok := runner.Annotations["actions.github.com/job-id"].(string); ok {
				info.GithubJobID = v
			}
			if v, ok := runner.Annotations["actions.github.com/workflow"].(string); ok {
				info.WorkflowName = v
			}
			if v, ok := runner.Annotations["actions.github.com/branch"].(string); ok {
				info.Branch = v
			}
		}

		runnerInfos = append(runnerInfos, info)
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"runners": runnerInfos,
		"total":   len(runnerInfos),
		"config": gin.H{
			"id":        config.ID,
			"name":      config.Name,
			"namespace": config.RunnerSetNamespace,
		},
	}))
}

// ========== Advanced Metrics Query APIs (Phase 4) ==========

// MetricsAdvancedQueryRequest represents the request for advanced metrics query
type MetricsAdvancedQueryRequest struct {
	Start         string                 `json:"start,omitempty"`
	End           string                 `json:"end,omitempty"`
	Dimensions    map[string]interface{} `json:"dimensions,omitempty"`
	MetricFilters map[string]interface{} `json:"metric_filters,omitempty"`
	SortBy        string                 `json:"sort_by,omitempty"`
	SortOrder     string                 `json:"sort_order,omitempty"`
	Offset        int                    `json:"offset,omitempty"`
	Limit         int                    `json:"limit,omitempty"`
}

// QueryGithubWorkflowMetricsAdvanced handles POST /v1/github-workflow-metrics/configs/:config_id/metrics/query
// Advanced query with JSONB dimension filtering
func QueryGithubWorkflowMetricsAdvanced(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	var req MetricsAdvancedQueryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Build query
	query := &database.MetricsAdvancedQuery{
		ConfigID:      configID,
		Dimensions:    req.Dimensions,
		MetricFilters: req.MetricFilters,
		SortBy:        req.SortBy,
		SortOrder:     req.SortOrder,
		Offset:        req.Offset,
		Limit:         req.Limit,
	}

	if req.Limit == 0 {
		query.Limit = 100
	}

	if req.Start != "" {
		if t, err := time.Parse(time.RFC3339, req.Start); err == nil {
			query.Start = &t
		}
	}
	if req.End != "" {
		if t, err := time.Parse(time.RFC3339, req.End); err == nil {
			query.End = &t
		}
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	metrics, total, err := facade.QueryWithDimensions(ctx.Request.Context(), query)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to query metrics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to query metrics", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"metrics": metrics,
		"total":   total,
		"offset":  query.Offset,
		"limit":   query.Limit,
	}))
}

// MetricsAggregationRequest represents the request for metrics aggregation
type MetricsAggregationRequest struct {
	Start       string                 `json:"start,omitempty"`
	End         string                 `json:"end,omitempty"`
	Dimensions  map[string]interface{} `json:"dimensions,omitempty"`
	GroupBy     []string               `json:"group_by,omitempty"`
	MetricField string                 `json:"metric_field" binding:"required"`
	AggFunc     string                 `json:"agg_func,omitempty"` // avg, sum, min, max, count
	Interval    string                 `json:"interval,omitempty"` // 1h, 6h, 1d, 1w
}

// GetGithubWorkflowMetricsAggregation handles POST /v1/github-workflow-metrics/configs/:config_id/metrics/aggregate
// Returns aggregated metrics by time interval
func GetGithubWorkflowMetricsAggregation(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	var req MetricsAggregationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Build query
	query := &database.MetricsAggregationQuery{
		ConfigID:    configID,
		Dimensions:  req.Dimensions,
		GroupBy:     req.GroupBy,
		MetricField: req.MetricField,
		AggFunc:     req.AggFunc,
		Interval:    req.Interval,
	}

	if query.AggFunc == "" {
		query.AggFunc = "avg"
	}
	if query.Interval == "" {
		query.Interval = "1d"
	}

	if req.Start != "" {
		if t, err := time.Parse(time.RFC3339, req.Start); err == nil {
			query.Start = &t
		}
	}
	if req.End != "" {
		if t, err := time.Parse(time.RFC3339, req.End); err == nil {
			query.End = &t
		}
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	results, err := facade.GetAggregatedMetrics(ctx.Request.Context(), query)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to aggregate metrics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to aggregate metrics", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"results":      results,
		"metric_field": query.MetricField,
		"agg_func":     query.AggFunc,
		"interval":     query.Interval,
		"group_by":     query.GroupBy,
	}))
}

// GetGithubWorkflowMetricsSummary handles GET /v1/github-workflow-metrics/configs/:config_id/summary
// Returns summary statistics for a config
func GetGithubWorkflowMetricsSummary(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	var start, end *time.Time
	if startStr := ctx.Query("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = &t
		}
	}
	if endStr := ctx.Query("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = &t
		}
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	summary, err := facade.GetMetricsSummary(ctx.Request.Context(), configID, start, end)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get metrics summary: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get summary", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), summary))
}

// MetricsTrendsRequest represents the request for metrics trends
type MetricsTrendsRequest struct {
	Start        string                 `json:"start,omitempty"`
	End          string                 `json:"end,omitempty"`
	Dimensions   map[string]interface{} `json:"dimensions,omitempty"`
	MetricFields []string               `json:"metric_fields" binding:"required"`
	Interval     string                 `json:"interval,omitempty"` // 1h, 6h, 1d
	GroupBy      []string               `json:"group_by,omitempty"`
}

// GetGithubWorkflowMetricsTrends handles POST /v1/github-workflow-metrics/configs/:config_id/metrics/trends
// Returns time-series trends for specified metrics
func GetGithubWorkflowMetricsTrends(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	var req MetricsTrendsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Build query
	query := &database.MetricsTrendsQuery{
		ConfigID:     configID,
		Dimensions:   req.Dimensions,
		MetricFields: req.MetricFields,
		Interval:     req.Interval,
		GroupBy:      req.GroupBy,
	}

	if query.Interval == "" {
		query.Interval = "1d"
	}

	if req.Start != "" {
		if t, err := time.Parse(time.RFC3339, req.Start); err == nil {
			query.Start = &t
		}
	}
	if req.End != "" {
		if t, err := time.Parse(time.RFC3339, req.End); err == nil {
			query.End = &t
		}
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	result, err := facade.GetMetricsTrends(ctx.Request.Context(), query)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get metrics trends: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get trends", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), result))
}

// GetGithubWorkflowMetricsDimensions handles GET /v1/github-workflow-metrics/configs/:config_id/dimensions
// Returns available dimensions and their distinct values
func GetGithubWorkflowMetricsDimensions(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	var start, end *time.Time
	if startStr := ctx.Query("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = &t
		}
	}
	if endStr := ctx.Query("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = &t
		}
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()

	// Get available dimensions
	dimensions, err := facade.GetAvailableDimensions(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get dimensions: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get dimensions", err))
		return
	}

	// Get distinct values for each dimension
	dimensionValues := make(map[string][]string)
	for _, dim := range dimensions {
		values, err := facade.GetDistinctDimensionValues(ctx.Request.Context(), configID, dim, start, end)
		if err != nil {
			log.Warnf("Failed to get values for dimension %s: %v", dim, err)
			continue
		}
		dimensionValues[dim] = values
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"dimensions": dimensions,
		"values":     dimensionValues,
	}))
}

// GetGithubWorkflowMetricsFields handles GET /v1/github-workflow-metrics/configs/:config_id/fields
// Returns available metric fields
func GetGithubWorkflowMetricsFields(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName := ctx.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()

	// Get available dimensions
	dimensions, err := facade.GetAvailableDimensions(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get dimensions: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get dimensions", err))
		return
	}

	// Get available metric fields
	metricFields, err := facade.GetAvailableMetricFields(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get metric fields: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get metric fields", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"dimension_fields": dimensions,
		"metric_fields":    metricFields,
	}))
}

