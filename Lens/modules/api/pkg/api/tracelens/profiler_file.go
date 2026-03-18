// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// ProfilerFileContentChunk represents a chunk of profiler file content
type ProfilerFileContentChunk struct {
	Content         []byte `gorm:"column:content"`
	ContentEncoding string `gorm:"column:content_encoding"`
	ChunkIndex      int    `gorm:"column:chunk_index"`
	TotalChunks     int    `gorm:"column:total_chunks"`
}

// ProfilerFileListItem represents a profiler file in the list response
type ProfilerFileListItem struct {
	ID          int32  `gorm:"column:id" json:"id"`
	FileName    string `gorm:"column:file_name" json:"file_name"`
	FileType    string `gorm:"column:file_type" json:"file_type"`
	FileSize    int64  `gorm:"column:file_size" json:"file_size"`
	WorkloadUID string `gorm:"column:workload_uid" json:"workload_uid"`
	CreatedAt   string `gorm:"column:created_at" json:"created_at"`
}

// ListProfilerFilesForWorkload is a route alias for /workloads/:uid/profiler-files
// It extracts workload_uid from the URL path param and delegates to ListProfilerFiles.
// GET /v1/workloads/:uid/profiler-files?cluster={cluster}
func ListProfilerFilesForWorkload(c *gin.Context) {
	uid := c.Param("uid")
	if uid != "" {
		c.Request.URL.RawQuery = "workload_uid=" + uid + "&" + c.Request.URL.RawQuery
	}
	ListProfilerFiles(c)
}

// ListProfilerFiles returns a list of profiler files for a workload
// GET /v1/profiler/files?workload_uid={workload_uid}&cluster={cluster}
func ListProfilerFiles(c *gin.Context) {
	workloadUID := c.Query("workload_uid")
	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workload_uid is required"})
		return
	}

	// Get cluster
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get profiler files from database
	facade := database.GetFacadeForCluster(clients.ClusterName)
	db := facade.GetTraceLensSession().GetDB()

	var files []ProfilerFileListItem
	err = db.WithContext(c).
		Table("profiler_files").
		Select("id, file_name, file_type, file_size, workload_uid, created_at").
		Where("workload_uid = ?", workloadUID).
		Order("created_at DESC").
		Find(&files).Error

	if err != nil {
		log.Errorf("Failed to list profiler files for workload %s: %v", workloadUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list profiler files"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"meta": gin.H{"code": 2000, "message": "OK"},
		"data": files,
	})
}

// GetProfilerFileContent downloads the content of a profiler file
// GET /v1/profiler/files/:id/content
func GetProfilerFileContent(c *gin.Context) {
	fileIDStr := c.Param("id")
	fileID, err := strconv.ParseInt(fileIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	// Get cluster
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get file metadata
	facade := database.GetFacadeForCluster(clients.ClusterName)
	db := facade.GetTraceLensSession().GetDB()

	var fileInfo struct {
		ID          int32  `gorm:"column:id"`
		FileName    string `gorm:"column:file_name"`
		FileSize    int64  `gorm:"column:file_size"`
		StorageType string `gorm:"column:storage_type"`
	}

	err = db.WithContext(c).
		Table("profiler_files").
		Where("id = ?", fileID).
		First(&fileInfo).Error

	if err != nil {
		log.Errorf("Failed to get profiler file %d: %v", fileID, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	// Only support database storage type
	if fileInfo.StorageType != "database" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "unsupported storage type",
			"storage_type": fileInfo.StorageType,
		})
		return
	}

	// Get file content chunks
	var chunks []ProfilerFileContentChunk
	err = db.WithContext(c).
		Table("profiler_file_content").
		Select("content, content_encoding, chunk_index, total_chunks").
		Where("profiler_file_id = ?", fileID).
		Order("chunk_index ASC").
		Find(&chunks).Error

	if err != nil {
		log.Errorf("Failed to get profiler file content %d: %v", fileID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get file content"})
		return
	}

	if len(chunks) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "file content not found"})
		return
	}

	// Combine all chunks
	var combinedContent bytes.Buffer
	for _, chunk := range chunks {
		combinedContent.Write(chunk.Content)
	}

	content := combinedContent.Bytes()
	contentEncoding := ""
	if len(chunks) > 0 {
		contentEncoding = chunks[0].ContentEncoding
	}

	log.Infof("Serving profiler file %d (%s), size: %d bytes, encoding: %s", fileID, fileInfo.FileName, len(content), contentEncoding)

	// Check if raw download is requested (preserves original file as-is without browser decompression)
	rawDownload := c.Query("raw") == "true"

	// Set response headers
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileInfo.FileName))
	c.Header("Content-Length", strconv.Itoa(len(content)))

	// For raw download, don't set Content-Encoding so browser saves the file as-is
	// For non-raw download, set Content-Encoding: gzip so browser auto-decompresses
	if contentEncoding == "gzip" && !rawDownload {
		c.Header("Content-Encoding", "gzip")
	}
	// Always include original encoding info for reference
	if contentEncoding != "" {
		c.Header("X-Original-Content-Encoding", contentEncoding)
	}

	c.Data(http.StatusOK, "application/octet-stream", content)
}

// GetProfilerFileInfo returns metadata about a profiler file
// GET /v1/profiler/files/:id
func GetProfilerFileInfo(c *gin.Context) {
	fileIDStr := c.Param("id")
	fileID, err := strconv.ParseInt(fileIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file id"})
		return
	}

	// Get cluster
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get file metadata
	facade := database.GetFacadeForCluster(clients.ClusterName)
	db := facade.GetTraceLensSession().GetDB()

	var fileInfo struct {
		ID           int32  `gorm:"column:id" json:"id"`
		WorkloadUID  string `gorm:"column:workload_uid" json:"workload_uid"`
		FileName     string `gorm:"column:file_name" json:"file_name"`
		FileType     string `gorm:"column:file_type" json:"file_type"`
		FileSize     int64  `gorm:"column:file_size" json:"file_size"`
		StorageType  string `gorm:"column:storage_type" json:"storage_type"`
		CollectedAt  string `gorm:"column:collected_at" json:"collected_at"`
		PodName      string `gorm:"column:pod_name" json:"pod_name"`
		PodNamespace string `gorm:"column:pod_namespace" json:"pod_namespace"`
	}

	err = db.WithContext(c).
		Table("profiler_files").
		Where("id = ?", fileID).
		First(&fileInfo).Error

	if err != nil {
		log.Errorf("Failed to get profiler file %d: %v", fileID, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"meta": gin.H{"code": 2000, "message": "OK"},
		"data": fileInfo,
	})
}

