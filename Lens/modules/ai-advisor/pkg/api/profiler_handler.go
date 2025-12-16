package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/profiler"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// ProfilerHandler profiler API handler
type ProfilerHandler struct {
	lifecycleMgr *profiler.LifecycleManager
	metadataMgr  *profiler.MetadataManager
}

// NewProfilerHandler creates handler
func NewProfilerHandler(
	lifecycleMgr *profiler.LifecycleManager,
	metadataMgr *profiler.MetadataManager,
) *ProfilerHandler {
	return &ProfilerHandler{
		lifecycleMgr: lifecycleMgr,
		metadataMgr:  metadataMgr,
	}
}

// TriggerCleanup manually triggers cleanup
// POST /api/v1/profiler/cleanup
func (h *ProfilerHandler) TriggerCleanup(c *gin.Context) {
	result, err := h.lifecycleMgr.CleanupExpiredFiles(c.Request.Context())
	if err != nil {
		log.Errorf("Cleanup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Cleanup completed",
		"deleted_count": result.DeletedCount,
		"freed_bytes":   result.FreedSpace,
		"duration":      result.Duration.String(),
		"errors":        result.Errors,
	})
}

// QueryFiles queries files
// GET /api/v1/profiler/workloads/:uid/files
func (h *ProfilerHandler) QueryFiles(c *gin.Context) {
	workloadUID := c.Param("uid")

	// TODO: Implement QueryFiles in MetadataManager
	// query := &profiler.FileQueryRequest{
	// 	WorkloadUID: workloadUID,
	// 	FileType:    c.Query("file_type"),
	// 	StartDate:   c.Query("start_date"),
	// 	EndDate:     c.Query("end_date"),
	// 	Limit:       parseIntQuery(c, "limit", 50),
	// 	Offset:      parseIntQuery(c, "offset", 0),
	// }
	//
	// files, total, err := h.metadataMgr.QueryFiles(c.Request.Context(), query)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	// 	return
	// }

	_ = h.metadataMgr // Suppress unused warning
	c.JSON(http.StatusOK, gin.H{
		"workload_uid": workloadUID,
		"total":        0,
		"files":        []interface{}{},
		"message":      "QueryFiles not yet implemented",
	})
}

// GetStorageStats gets storage statistics
// GET /api/v1/profiler/stats
func (h *ProfilerHandler) GetStorageStats(c *gin.Context) {
	// TODO: Implement GetStorageStats in MetadataManager
	// stats, err := h.metadataMgr.GetStorageStats(c.Request.Context())
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	// 	return
	// }

	c.JSON(http.StatusOK, gin.H{
		"message": "GetStorageStats not yet implemented",
		"stats":   map[string]interface{}{},
	})
}

// GetFile gets file details
// GET /api/v1/profiler/files/:id
func (h *ProfilerHandler) GetFile(c *gin.Context) {
	fileIDStr := c.Param("id")
	fileID, err := strconv.ParseInt(fileIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	// TODO: Implement GetFile in MetadataManager
	// file, err := h.metadataMgr.GetFile(c.Request.Context(), fileID)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	// 	return
	// }
	//
	// if file == nil {
	// 	c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
	// 	return
	// }

	c.JSON(http.StatusOK, gin.H{
		"file_id": fileID,
		"message": "GetFile not yet implemented",
	})
}

// GetDownloadURL gets file download URL
// GET /api/v1/profiler/files/:id/download
func (h *ProfilerHandler) GetDownloadURL(c *gin.Context) {
	fileIDStr := c.Param("id")
	fileID, err := strconv.ParseInt(fileIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	expiresStr := c.DefaultQuery("expires_in", "24h")
	expires, err := parseDuration(expiresStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expires_in format"})
		return
	}
	_ = expires // Suppress unused variable warning

	// TODO: Implement GenerateDownloadURL in MetadataManager
	//url, err := h.metadataMgr.GenerateDownloadURL(c.Request.Context(), fileID, expires)
	//if err != nil {
	//	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	//	return
	//}

	c.JSON(http.StatusOK, gin.H{
		"file_id":      fileID,
		"download_url": fmt.Sprintf("/api/v1/profiler/files/%d/download", fileID),
		"expires_in":   expiresStr,
		"message":      "Direct download endpoint (not yet implemented)",
	})
}

// RegisterRoutes registers routes
func (h *ProfilerHandler) RegisterRoutes(r *gin.RouterGroup) {
	profilerGroup := r.Group("/profiler")
	{
		profilerGroup.POST("/cleanup", h.TriggerCleanup)
		profilerGroup.GET("/workloads/:uid/files", h.QueryFiles)
		profilerGroup.GET("/stats", h.GetStorageStats)
		profilerGroup.GET("/files/:id", h.GetFile)
		profilerGroup.GET("/files/:id/download", h.GetDownloadURL)
	}
}

// parseIntQuery parses integer query parameter
func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	val := c.Query(key)
	if val == "" {
		return defaultVal
	}

	intVal, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}

	return intVal
}

// parseDuration parses duration string
func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

