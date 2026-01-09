// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"io"
	"net/http"
	"path/filepath"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/pyspy"
	"github.com/gin-gonic/gin"
)

// PySpyExecute handles POST /api/v1/pyspy/execute
// Called by Jobs module to execute py-spy sampling
func PySpyExecute(c *gin.Context) {
	var req pyspy.ExecuteRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, err.Error(), nil))
		return
	}

	collector := pyspy.GetCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c, http.StatusServiceUnavailable, "py-spy collector not initialized", nil))
		return
	}

	if !collector.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c, http.StatusServiceUnavailable, "py-spy is disabled", nil))
		return
	}

	response, err := collector.Execute(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if !response.Success {
		c.JSON(http.StatusOK, rest.SuccessResp(c, response))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// PySpyCheck handles POST /api/v1/pyspy/check
// Check py-spy compatibility for a pod
func PySpyCheck(c *gin.Context) {
	var req pyspy.CheckRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, err.Error(), nil))
		return
	}

	collector := pyspy.GetCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c, http.StatusServiceUnavailable, "py-spy collector not initialized", nil))
		return
	}

	response, err := collector.Check(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// PySpyListFiles handles GET /api/v1/pyspy/files
// List local profiling files
func PySpyListFiles(c *gin.Context) {
	collector := pyspy.GetCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c, http.StatusServiceUnavailable, "py-spy collector not initialized", nil))
		return
	}

	req := &pyspy.FileListRequest{
		TaskID: c.Query("task_id"),
		PodUID: c.Query("pod_uid"),
	}

	response := collector.ListFiles(req)
	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// PySpyDownloadFile handles GET /api/v1/pyspy/file/:task_id/:filename
// Download a specific file
func PySpyDownloadFile(c *gin.Context) {
	taskID := c.Param("task_id")
	fileName := c.Param("filename")

	if taskID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, "task_id is required", nil))
		return
	}

	collector := pyspy.GetCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c, http.StatusServiceUnavailable, "py-spy collector not initialized", nil))
		return
	}

	reader, fileInfo, err := collector.ReadFile(taskID, fileName)
	if err != nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c, http.StatusNotFound, err.Error(), nil))
		return
	}

	readCloser, ok := reader.(io.ReadCloser)
	if !ok {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "invalid file reader", nil))
		return
	}
	defer readCloser.Close()

	// Set content type based on format
	contentType := getContentType(fileInfo.Format)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename="+fileInfo.FileName)
	c.Header("Content-Length", string(rune(fileInfo.FileSize)))

	c.Status(http.StatusOK)
	io.Copy(c.Writer, readCloser)
}

// PySpyGetFile handles GET /api/v1/pyspy/file/:task_id
// Get file info for a task
func PySpyGetFile(c *gin.Context) {
	taskID := c.Param("task_id")

	if taskID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, "task_id is required", nil))
		return
	}

	collector := pyspy.GetCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c, http.StatusServiceUnavailable, "py-spy collector not initialized", nil))
		return
	}

	fileInfo, ok := collector.GetFile(taskID)
	if !ok {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c, http.StatusNotFound, "file not found", nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, fileInfo))
}

// PySpyDeleteFile handles DELETE /api/v1/pyspy/file/:task_id
// Delete a file for a task
func PySpyDeleteFile(c *gin.Context) {
	taskID := c.Param("task_id")

	if taskID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, "task_id is required", nil))
		return
	}

	collector := pyspy.GetCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c, http.StatusServiceUnavailable, "py-spy collector not initialized", nil))
		return
	}

	if err := collector.DeleteFile(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"deleted": true}))
}

// PySpyCancelTask handles POST /api/v1/pyspy/cancel/:task_id
// Cancel a running task
func PySpyCancelTask(c *gin.Context) {
	taskID := c.Param("task_id")

	if taskID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, "task_id is required", nil))
		return
	}

	collector := pyspy.GetCollector()
	if collector == nil {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c, http.StatusServiceUnavailable, "py-spy collector not initialized", nil))
		return
	}

	cancelled := collector.CancelTask(taskID)
	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"cancelled": cancelled}))
}

// PySpyStatus handles GET /api/v1/pyspy/status
// Get py-spy collector status
func PySpyStatus(c *gin.Context) {
	collector := pyspy.GetCollector()
	if collector == nil {
		c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
			"initialized": false,
		}))
		return
	}

	fileCount, totalSize := collector.GetStorageStats()
	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"initialized": true,
		"enabled":     collector.IsEnabled(),
		"file_count":  fileCount,
		"total_size":  totalSize,
	}))
}

// getContentType returns the content type for a format
func getContentType(format string) string {
	switch format {
	case "flamegraph":
		return "image/svg+xml"
	case "speedscope":
		return "application/json"
	case "raw":
		return "text/plain"
	default:
		ext := filepath.Ext(format)
		switch ext {
		case ".svg":
			return "image/svg+xml"
		case ".json":
			return "application/json"
		default:
			return "application/octet-stream"
		}
	}
}

