package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ======================== Request/Response Models ========================

// FrameworkAnnotationRequest represents the request body for annotating workload framework
type FrameworkAnnotationRequest struct {
	Framework        string                 `json:"framework"`         // Framework name (primus, megatron, deepspeed, etc.)
	FrameworkLayer   string                 `json:"framework_layer"`   // Framework layer: wrapper or base (optional)
	WrapperFramework string                 `json:"wrapper_framework"` // Wrapper framework (optional, e.g., primus, lightning)
	BaseFramework    string                 `json:"base_framework"`    // Base framework (optional, e.g., megatron, deepspeed)
	Type             string                 `json:"type"`              // Task type (training, inference), optional
	Confidence       float64                `json:"confidence"`        // Confidence [0.0-1.0], default 1.0 for user annotation
	Evidence         map[string]interface{} `json:"evidence"`          // Optional evidence/notes from user
}

// UpdateMetadataRequest represents the request body for full metadata update
type UpdateMetadataRequest struct {
	Type      string                 `json:"type" binding:"required"`
	Framework string                 `json:"framework" binding:"required"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// AiWorkloadMetadataResponse represents the response for AI workload metadata with conflicts
type AiWorkloadMetadataResponse struct {
	ID                  int32                  `json:"id"`
	WorkloadUID         string                 `json:"workload_uid"`
	Type                string                 `json:"type"`
	Framework           string                 `json:"framework"`
	FrameworkLayer      string                 `json:"framework_layer,omitempty"`   // Framework layer: wrapper or base
	WrapperFramework    string                 `json:"wrapper_framework,omitempty"` // Wrapper framework (e.g., primus, lightning)
	BaseFramework       string                 `json:"base_framework,omitempty"`    // Base framework (e.g., megatron, deepspeed)
	Metadata            map[string]interface{} `json:"metadata"`
	ImagePrefix         string                 `json:"image_prefix"`
	CreatedAt           time.Time              `json:"created_at"`
	HasConflicts        bool                   `json:"has_conflicts"`              // Whether conflicts exist
	UnresolvedConflicts int                    `json:"unresolved_conflicts"`       // Number of unresolved conflicts
	ConflictSummary     []ConflictSummaryItem  `json:"conflict_summary,omitempty"` // Conflict summary
}

// ConflictSummaryItem represents a summary of a detection conflict
type ConflictSummaryItem struct {
	ID                 int64     `json:"id"`
	Source1            string    `json:"source_1"`
	Source2            string    `json:"source_2"`
	Framework1         string    `json:"framework_1"`
	Framework2         string    `json:"framework_2"`
	Confidence1        float64   `json:"confidence_1"`
	Confidence2        float64   `json:"confidence_2"`
	ResolutionStrategy string    `json:"resolution_strategy,omitempty"`
	ResolvedFramework  string    `json:"resolved_framework,omitempty"`
	ResolvedConfidence float64   `json:"resolved_confidence,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	ResolvedAt         time.Time `json:"resolved_at,omitempty"`
}

// DetectionConflictLogDetail represents the detailed conflict log
type DetectionConflictLogDetail struct {
	ID                 int64                  `json:"id"`
	WorkloadUID        string                 `json:"workload_uid"`
	Source1            string                 `json:"source_1"`
	Source2            string                 `json:"source_2"`
	Framework1         string                 `json:"framework_1"`
	Framework2         string                 `json:"framework_2"`
	Confidence1        float64                `json:"confidence_1"`
	Confidence2        float64                `json:"confidence_2"`
	ResolutionStrategy string                 `json:"resolution_strategy"`
	ResolvedFramework  string                 `json:"resolved_framework"`
	ResolvedConfidence float64                `json:"resolved_confidence"`
	Evidence1          map[string]interface{} `json:"evidence_1"`
	Evidence2          map[string]interface{} `json:"evidence_2"`
	CreatedAt          time.Time              `json:"created_at"`
	ResolvedAt         time.Time              `json:"resolved_at"`
}

// ListAiWorkloadMetadataQueryParams represents query parameters for listing AI workload metadata
type ListAiWorkloadMetadataQueryParams struct {
	rest.Page
	Framework        string `form:"framework"`         // Search in both wrapper_framework and base_framework
	WrapperFramework string `form:"wrapper_framework"` // Specific wrapper framework filter
	BaseFramework    string `form:"base_framework"`    // Specific base framework filter
	Type             string `form:"type"`
	HasConflict      *bool  `form:"has_conflict"`
}

// ======================== API Handlers ========================

// GetAiWorkloadMetadata retrieves AI workload metadata by workload UID with conflict information
// GET /ai-workload-metadata/:workload_uid
func GetAiWorkloadMetadata(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	// Get metadata
	metadata, err := database.GetFacadeForCluster(clients.ClusterName).GetAiWorkloadMetadata().GetAiWorkloadMetadata(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get AI workload metadata: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get metadata", err))
		return
	}

	if metadata == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "metadata not found", nil))
		return
	}

	// Get conflict logs
	conflicts, _, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionConflictLog().ListDetectionConflictLogsByWorkloadUID(ctx.Request.Context(), workloadUID, 100, 0)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get conflict logs: %v", err)
		// Don't fail the request, just log the error
		conflicts = []*dbmodel.DetectionConflictLog{}
	}

	// Build response with conflict information
	response := buildMetadataResponseWithConflicts(metadata, conflicts)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// ListAiWorkloadMetadata lists AI workload metadata with optional filters and conflict status
// GET /ai-workload-metadata
func ListAiWorkloadMetadata(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	var queryParams ListAiWorkloadMetadataQueryParams
	if err := ctx.ShouldBindQuery(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid query parameters", err))
		return
	}

	// For simplicity, we'll list all and filter in memory
	// In production, you'd want to implement proper database filtering
	db := database.GetFacadeForCluster(clients.ClusterName).GetAiWorkloadMetadata()

	// Get all metadata (you can implement pagination in facade later)
	allMetadata, err := db.ListAiWorkloadMetadataByUIDs(ctx.Request.Context(), []string{})
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list AI workload metadata: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list metadata", err))
		return
	}

	// Build responses with conflict information
	responses := []AiWorkloadMetadataResponse{}
	for _, metadata := range allMetadata {
		// Get conflict logs
		conflicts, _, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionConflictLog().ListDetectionConflictLogsByWorkloadUID(ctx.Request.Context(), metadata.WorkloadUID, 100, 0)
		if err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to get conflict logs for workload %s: %v", metadata.WorkloadUID, err)
			conflicts = []*dbmodel.DetectionConflictLog{}
		}

		response := buildMetadataResponseWithConflicts(metadata, conflicts)

		// Apply filters
		if queryParams.Type != "" && response.Type != queryParams.Type {
			continue
		}

		// Framework filter: search in wrapper_framework, base_framework, or primary framework
		if queryParams.Framework != "" {
			matched := false
			if response.Framework == queryParams.Framework {
				matched = true
			}
			if response.WrapperFramework == queryParams.Framework {
				matched = true
			}
			if response.BaseFramework == queryParams.Framework {
				matched = true
			}
			if !matched {
				continue
			}
		}

		// Specific wrapper framework filter
		if queryParams.WrapperFramework != "" && response.WrapperFramework != queryParams.WrapperFramework {
			continue
		}

		// Specific base framework filter
		if queryParams.BaseFramework != "" && response.BaseFramework != queryParams.BaseFramework {
			continue
		}

		// Filter by conflict status if specified
		if queryParams.HasConflict != nil {
			if *queryParams.HasConflict != response.HasConflicts {
				continue
			}
		}

		responses = append(responses, response)
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"data":  responses,
		"total": len(responses),
	}))
}

// AnnotateWorkloadFramework annotates a workload with framework information (user annotation)
// POST /ai-workload-metadata/:workload_uid/annotate
func AnnotateWorkloadFramework(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	var req FrameworkAnnotationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid request body", err))
		return
	}

	// Validate: at least one framework field must be provided
	if req.Framework == "" && req.WrapperFramework == "" && req.BaseFramework == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "at least one of framework, wrapper_framework, or base_framework is required", nil))
		return
	}

	// If Framework is not provided, infer it from other fields
	if req.Framework == "" {
		if req.WrapperFramework != "" {
			req.Framework = req.WrapperFramework
		} else if req.BaseFramework != "" {
			req.Framework = req.BaseFramework
		}
	}

	// Set default values
	if req.Type == "" {
		req.Type = "training"
	}
	if req.Confidence == 0 {
		req.Confidence = 1.0 // Default confidence for user annotation is 1.0
	}
	if req.Evidence == nil {
		req.Evidence = make(map[string]interface{})
	}

	// Add user annotation info to evidence
	req.Evidence["method"] = "user_annotation"
	req.Evidence["annotated_at"] = time.Now().Format(time.RFC3339)

	// Add framework layer info to evidence if provided
	if req.FrameworkLayer != "" {
		req.Evidence["framework_layer"] = req.FrameworkLayer
	}
	if req.WrapperFramework != "" {
		req.Evidence["wrapper_framework"] = req.WrapperFramework
	}
	if req.BaseFramework != "" {
		req.Evidence["base_framework"] = req.BaseFramework
	}

	metadataFacade := database.GetFacadeForCluster(clients.ClusterName).GetAiWorkloadMetadata()

	// Check if metadata already exists
	existing, err := metadataFacade.GetAiWorkloadMetadata(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get AI workload metadata: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get metadata", err))
		return
	}

	now := time.Now()

	// Create new detection source
	newSource := map[string]interface{}{
		"source":      "user",
		"framework":   req.Framework,
		"type":        req.Type,
		"confidence":  req.Confidence,
		"detected_at": now.Format(time.RFC3339),
		"evidence":    req.Evidence,
	}

	// Add framework layer fields to source if provided
	if req.FrameworkLayer != "" {
		newSource["framework_layer"] = req.FrameworkLayer
	}
	if req.WrapperFramework != "" {
		newSource["wrapper_framework"] = req.WrapperFramework
	}
	if req.BaseFramework != "" {
		newSource["base_framework"] = req.BaseFramework
	}

	if existing == nil {
		// Create new metadata
		frameworkDetection := map[string]interface{}{
			"framework":  req.Framework,
			"type":       req.Type,
			"confidence": req.Confidence,
			"status":     "confirmed", // User annotation is directly confirmed
			"sources":    []interface{}{newSource},
			"conflicts":  []interface{}{},
			"version":    "1.0",
			"updated_at": now.Format(time.RFC3339),
		}

		// Add framework layer fields if provided
		if req.FrameworkLayer != "" {
			frameworkDetection["framework_layer"] = req.FrameworkLayer
		}
		if req.WrapperFramework != "" {
			frameworkDetection["wrapper_framework"] = req.WrapperFramework
		}
		if req.BaseFramework != "" {
			frameworkDetection["base_framework"] = req.BaseFramework
		}

		metadata := &dbmodel.AiWorkloadMetadata{
			WorkloadUID: workloadUID,
			Type:        req.Type,
			Framework:   req.Framework,
			Metadata: map[string]interface{}{
				"framework_detection": frameworkDetection,
			},
			CreatedAt: now,
		}

		if err := metadataFacade.CreateAiWorkloadMetadata(ctx.Request.Context(), metadata); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to create AI workload metadata: %v", err)
			ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to create metadata", err))
			return
		}

		log.GlobalLogger().WithContext(ctx).Infof("Created framework annotation for workload %s: framework=%s", workloadUID, req.Framework)
		ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), metadata))
		return
	}

	// Update existing metadata
	if existing.Metadata == nil {
		existing.Metadata = make(map[string]interface{})
	}

	// Get existing framework_detection
	var frameworkDetection map[string]interface{}
	if detectionData, ok := existing.Metadata["framework_detection"]; ok {
		if detectionMap, ok := detectionData.(map[string]interface{}); ok {
			frameworkDetection = detectionMap
		} else {
			frameworkDetection = make(map[string]interface{})
		}
	} else {
		frameworkDetection = make(map[string]interface{})
	}

	// Update or add sources
	var sources []interface{}
	if existingSources, ok := frameworkDetection["sources"].([]interface{}); ok {
		// Check if user annotation already exists
		foundUserSource := false
		for i, s := range existingSources {
			if sourceMap, ok := s.(map[string]interface{}); ok {
				if sourceMap["source"] == "user" {
					// Update existing user annotation
					existingSources[i] = newSource
					foundUserSource = true
					break
				}
			}
		}
		if !foundUserSource {
			sources = append(existingSources, newSource)
		} else {
			sources = existingSources
		}
	} else {
		sources = []interface{}{newSource}
	}

	// Update framework_detection
	frameworkDetection["framework"] = req.Framework
	frameworkDetection["type"] = req.Type
	frameworkDetection["confidence"] = req.Confidence
	frameworkDetection["status"] = "confirmed" // Status is confirmed after user annotation
	frameworkDetection["sources"] = sources
	frameworkDetection["updated_at"] = now.Format(time.RFC3339)
	if frameworkDetection["version"] == nil {
		frameworkDetection["version"] = "1.0"
	}

	// Update framework layer fields if provided
	if req.FrameworkLayer != "" {
		frameworkDetection["framework_layer"] = req.FrameworkLayer
	}
	if req.WrapperFramework != "" {
		frameworkDetection["wrapper_framework"] = req.WrapperFramework
	}
	if req.BaseFramework != "" {
		frameworkDetection["base_framework"] = req.BaseFramework
	}

	// Update metadata
	existing.Metadata["framework_detection"] = frameworkDetection
	existing.Framework = req.Framework
	existing.Type = req.Type

	if err := metadataFacade.UpdateAiWorkloadMetadata(ctx.Request.Context(), existing); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update AI workload metadata: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to update metadata", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Updated framework annotation for workload %s: framework=%s", workloadUID, req.Framework)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), existing))
}

// UpdateAiWorkloadMetadata updates existing AI workload metadata
// PUT /ai-workload-metadata/:workload_uid
func UpdateAiWorkloadMetadata(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	var req UpdateMetadataRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid request body", err))
		return
	}

	// Get existing metadata
	existing, err := database.GetFacadeForCluster(clients.ClusterName).GetAiWorkloadMetadata().GetAiWorkloadMetadata(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get AI workload metadata: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get metadata", err))
		return
	}

	if existing == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "metadata not found", nil))
		return
	}

	// Update fields
	existing.Type = req.Type
	existing.Framework = req.Framework
	existing.Metadata = req.Metadata

	if err := database.GetFacadeForCluster(clients.ClusterName).GetAiWorkloadMetadata().UpdateAiWorkloadMetadata(ctx.Request.Context(), existing); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update AI workload metadata: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to update metadata", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), existing))
}

// DeleteAiWorkloadMetadata deletes AI workload metadata
// DELETE /ai-workload-metadata/:workload_uid
func DeleteAiWorkloadMetadata(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	if err := database.GetFacadeForCluster(clients.ClusterName).GetAiWorkloadMetadata().DeleteAiWorkloadMetadata(ctx.Request.Context(), workloadUID); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete AI workload metadata: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to delete metadata", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "metadata deleted successfully",
	}))
}

// GetDetectionConflictLogs retrieves detection conflict logs for a specific workload
// GET /ai-workload-metadata/:workload_uid/conflicts
func GetDetectionConflictLogs(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	// Parse pagination parameters
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	conflicts, total, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionConflictLog().ListDetectionConflictLogsByWorkloadUID(ctx.Request.Context(), workloadUID, pageSize, offset)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get conflict logs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get conflict logs", err))
		return
	}

	// Convert to detail response
	details := make([]DetectionConflictLogDetail, 0, len(conflicts))
	for _, conflict := range conflicts {
		details = append(details, convertConflictToDetail(conflict))
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"data":      details,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}))
}

// ListAllDetectionConflicts lists all recent detection conflicts across all workloads
// GET /detection-conflicts
func ListAllDetectionConflicts(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	// Parse pagination parameters
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	conflicts, total, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionConflictLog().ListRecentConflicts(ctx.Request.Context(), pageSize, offset)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list conflict logs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list conflict logs", err))
		return
	}

	// Convert to detail response
	details := make([]DetectionConflictLogDetail, 0, len(conflicts))
	for _, conflict := range conflicts {
		details = append(details, convertConflictToDetail(conflict))
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"data":      details,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}))
}

// ======================== Helper Functions ========================

// buildMetadataResponseWithConflicts builds a response with conflict information
func buildMetadataResponseWithConflicts(metadata *dbmodel.AiWorkloadMetadata, conflicts []*dbmodel.DetectionConflictLog) AiWorkloadMetadataResponse {
	response := AiWorkloadMetadataResponse{
		ID:          metadata.ID,
		WorkloadUID: metadata.WorkloadUID,
		Type:        metadata.Type,
		Framework:   metadata.Framework,
		Metadata:    metadata.Metadata,
		ImagePrefix: metadata.ImagePrefix,
		CreatedAt:   metadata.CreatedAt,
	}

	// Extract framework layer information from metadata
	extractFrameworkLayerInfo(&response, metadata.Metadata)

	if len(conflicts) > 0 {
		response.HasConflicts = true
		unresolvedCount := 0
		conflictSummary := []ConflictSummaryItem{}

		for _, conflict := range conflicts {
			if conflict.ResolutionStrategy == "" {
				unresolvedCount++
			}

			conflictSummary = append(conflictSummary, ConflictSummaryItem{
				ID:                 conflict.ID,
				Source1:            conflict.Source1,
				Source2:            conflict.Source2,
				Framework1:         conflict.Framework1,
				Framework2:         conflict.Framework2,
				Confidence1:        conflict.Confidence1,
				Confidence2:        conflict.Confidence2,
				ResolutionStrategy: conflict.ResolutionStrategy,
				ResolvedFramework:  conflict.ResolvedFramework,
				ResolvedConfidence: conflict.ResolvedConfidence,
				CreatedAt:          conflict.CreatedAt,
				ResolvedAt:         conflict.ResolvedAt,
			})
		}

		response.UnresolvedConflicts = unresolvedCount
		response.ConflictSummary = conflictSummary
	}

	return response
}

// extractFrameworkLayerInfo extracts framework layer information from metadata
func extractFrameworkLayerInfo(response *AiWorkloadMetadataResponse, metadata map[string]interface{}) {
	if metadata == nil {
		return
	}

	// Try to extract from framework_detection field
	frameworkDetection, ok := metadata["framework_detection"]
	if !ok {
		return
	}

	detectionMap, ok := frameworkDetection.(map[string]interface{})
	if !ok {
		return
	}

	// Extract framework_layer
	if layer, ok := detectionMap["framework_layer"].(string); ok {
		response.FrameworkLayer = layer
	}

	// Extract wrapper_framework
	if wrapperFw, ok := detectionMap["wrapper_framework"].(string); ok {
		response.WrapperFramework = wrapperFw
	}

	// Extract base_framework
	if baseFw, ok := detectionMap["base_framework"].(string); ok {
		response.BaseFramework = baseFw
	}

	// If not found in top level, try to extract from sources
	if response.FrameworkLayer == "" || response.WrapperFramework == "" || response.BaseFramework == "" {
		if sources, ok := detectionMap["sources"].([]interface{}); ok && len(sources) > 0 {
			// Try to find from the most recent source (usually the first one after sorting)
			for _, source := range sources {
				sourceMap, ok := source.(map[string]interface{})
				if !ok {
					continue
				}

				// Extract from source evidence
				if evidence, ok := sourceMap["evidence"].(map[string]interface{}); ok {
					if response.FrameworkLayer == "" {
						if layer, ok := evidence["framework_layer"].(string); ok {
							response.FrameworkLayer = layer
						}
					}
					if response.WrapperFramework == "" {
						if wrapperFw, ok := evidence["wrapper_framework"].(string); ok {
							response.WrapperFramework = wrapperFw
						}
					}
					if response.BaseFramework == "" {
						if baseFw, ok := evidence["base_framework"].(string); ok {
							response.BaseFramework = baseFw
						}
					}
				}

				// Also check source level fields
				if response.FrameworkLayer == "" {
					if layer, ok := sourceMap["framework_layer"].(string); ok {
						response.FrameworkLayer = layer
					}
				}
				if response.WrapperFramework == "" {
					if wrapperFw, ok := sourceMap["wrapper_framework"].(string); ok {
						response.WrapperFramework = wrapperFw
					}
				}
				if response.BaseFramework == "" {
					if baseFw, ok := sourceMap["base_framework"].(string); ok {
						response.BaseFramework = baseFw
					}
				}

				// If we found all fields, we can stop
				if response.FrameworkLayer != "" && response.WrapperFramework != "" && response.BaseFramework != "" {
					break
				}
			}
		}
	}
}

// convertConflictToDetail converts a conflict log model to detail response
func convertConflictToDetail(conflict *dbmodel.DetectionConflictLog) DetectionConflictLogDetail {
	return DetectionConflictLogDetail{
		ID:                 conflict.ID,
		WorkloadUID:        conflict.WorkloadUID,
		Source1:            conflict.Source1,
		Source2:            conflict.Source2,
		Framework1:         conflict.Framework1,
		Framework2:         conflict.Framework2,
		Confidence1:        conflict.Confidence1,
		Confidence2:        conflict.Confidence2,
		ResolutionStrategy: conflict.ResolutionStrategy,
		ResolvedFramework:  conflict.ResolvedFramework,
		ResolvedConfidence: conflict.ResolvedConfidence,
		Evidence1:          conflict.Evidence1,
		Evidence2:          conflict.Evidence2,
		CreatedAt:          conflict.CreatedAt,
		ResolvedAt:         conflict.ResolvedAt,
	}
}
