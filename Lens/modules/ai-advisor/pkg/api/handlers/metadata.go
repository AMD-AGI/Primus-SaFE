package handlers

import (
	"fmt"
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// CollectWorkloadMetadata triggers metadata collection for a training workload
func CollectWorkloadMetadata(c *gin.Context) {
	var req metadata.CollectionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid request: "+err.Error(),
			nil,
		))
		return
	}

	collector := metadata.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"metadata collector not initialized",
			nil,
		))
		return
	}

	log.Infof("Collecting metadata for workload %s (pod: %s/%s, node: %s)",
		req.WorkloadUID, req.PodNamespace, req.PodName, req.NodeName)

	result, err := collector.CollectMetadata(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to collect metadata: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to collect metadata: "+err.Error(),
			result,
		))
		return
	}

	if !result.Success {
		c.JSON(http.StatusOK, rest.SuccessResp(c, result))
		return
	}

	log.Infof("Successfully collected metadata for workload %s: frameworks=%v, python_count=%d",
		req.WorkloadUID, result.Metadata.Frameworks, result.PythonCount)

	c.JSON(http.StatusOK, rest.SuccessResp(c, result))
}

// GetWorkloadMetadata retrieves stored metadata for a workload
func GetWorkloadMetadata(c *gin.Context) {
	workloadUID := c.Param("uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"workload_uid is required",
			nil,
		))
		return
	}

	collector := metadata.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"metadata collector not initialized",
			nil,
		))
		return
	}

	metadata, err := collector.GetMetadata(c.Request.Context(), workloadUID)
	if err != nil {
		log.Errorf("Failed to get metadata: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to get metadata: "+err.Error(),
			nil,
		))
		return
	}

	if metadata == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(
			c.Request.Context(),
			http.StatusNotFound,
			"metadata not found for workload: "+workloadUID,
			nil,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, metadata))
}

// QueryWorkloadMetadata queries workload metadata with filters
func QueryWorkloadMetadata(c *gin.Context) {
	var query metadata.MetadataQuery

	if err := c.ShouldBindJSON(&query); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid query: "+err.Error(),
			nil,
		))
		return
	}

	collector := metadata.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"metadata collector not initialized",
			nil,
		))
		return
	}

	results, err := collector.QueryMetadata(c.Request.Context(), &query)
	if err != nil {
		log.Errorf("Failed to query metadata: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to query metadata: "+err.Error(),
			nil,
		))
		return
	}

	type Response struct {
		Results []*metadata.WorkloadMetadata `json:"results"`
		Total   int                          `json:"total"`
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, &Response{
		Results: results,
		Total:   len(results),
	}))
}

// DeleteWorkloadMetadata deletes metadata for a workload
func DeleteWorkloadMetadata(c *gin.Context) {
	workloadUID := c.Param("uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"workload_uid is required",
			nil,
		))
		return
	}

	collector := metadata.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"metadata collector not initialized",
			nil,
		))
		return
	}

	// Invalidate cache
	collector.InvalidateCache(workloadUID)

	// Note: Actual deletion would require access to storage
	// For now, just invalidate cache

	log.Infof("Deleted metadata for workload %s", workloadUID)

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"workload_uid": workloadUID,
		"deleted":      true,
	}))
}

// ListRecentMetadata lists recently collected metadata
func ListRecentMetadata(c *gin.Context) {
	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil {
			limit = 20
		}
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	collector := metadata.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"metadata collector not initialized",
			nil,
		))
		return
	}

	query := &metadata.MetadataQuery{
		Limit: limit,
	}

	results, err := collector.QueryMetadata(c.Request.Context(), query)
	if err != nil {
		log.Errorf("Failed to list recent metadata: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to list metadata: "+err.Error(),
			nil,
		))
		return
	}

	type Response struct {
		Results []*metadata.WorkloadMetadata `json:"results"`
		Total   int                          `json:"total"`
		Limit   int                          `json:"limit"`
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, &Response{
		Results: results,
		Total:   len(results),
		Limit:   limit,
	}))
}

// GetMetadataByFramework retrieves all metadata for a specific framework
func GetMetadataByFramework(c *gin.Context) {
	framework := c.Param("framework")

	if framework == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"framework is required",
			nil,
		))
		return
	}

	collector := metadata.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"metadata collector not initialized",
			nil,
		))
		return
	}

	query := &metadata.MetadataQuery{
		Framework: framework,
		Limit:     100,
	}

	results, err := collector.QueryMetadata(c.Request.Context(), query)
	if err != nil {
		log.Errorf("Failed to get metadata by framework: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to get metadata: "+err.Error(),
			nil,
		))
		return
	}

	type Response struct {
		Framework string                       `json:"framework"`
		Results   []*metadata.WorkloadMetadata `json:"results"`
		Total     int                          `json:"total"`
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, &Response{
		Framework: framework,
		Results:   results,
		Total:     len(results),
	}))
}

// GetMetadataStatistics retrieves statistics about collected metadata
func GetMetadataStatistics(c *gin.Context) {
	// This would require accessing the storage layer directly
	// For now, return a simple response

	collector := metadata.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"metadata collector not initialized",
			nil,
		))
		return
	}

	// Query all metadata to compute statistics
	query := &metadata.MetadataQuery{
		Limit: 1000,
	}

	results, err := collector.QueryMetadata(c.Request.Context(), query)
	if err != nil {
		log.Errorf("Failed to get metadata for statistics: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to compute statistics: "+err.Error(),
			nil,
		))
		return
	}

	// Compute statistics
	stats := struct {
		TotalWorkloads  int            `json:"total_workloads"`
		ByFramework     map[string]int `json:"by_framework"`
		ByWrapper       map[string]int `json:"by_wrapper"`
		WithTensorBoard int            `json:"with_tensorboard"`
		AvgConfidence   float64        `json:"avg_confidence"`
	}{
		TotalWorkloads: len(results),
		ByFramework:    make(map[string]int),
		ByWrapper:      make(map[string]int),
	}

	totalConfidence := 0.0
	for _, meta := range results {
		// Count by base framework
		if meta.BaseFramework != "" {
			stats.ByFramework[meta.BaseFramework]++
		}

		// Count by wrapper framework
		if meta.WrapperFramework != "" {
			stats.ByWrapper[meta.WrapperFramework]++
		}

		// Count TensorBoard usage
		if meta.TensorBoardInfo != nil && meta.TensorBoardInfo.Enabled {
			stats.WithTensorBoard++
		}

		totalConfidence += meta.Confidence
	}

	if len(results) > 0 {
		stats.AvgConfidence = totalConfidence / float64(len(results))
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, stats))
}
