// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package exporter

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// FilesystemResponse represents filesystem info for API response
type FilesystemResponse struct {
	Name             string `json:"name"`
	StorageClassName string `json:"storageClassName"`
	FilesystemName   string `json:"filesystemName"`
	StorageType      string `json:"storageType"`
	VolumeType       string `json:"volumeType,omitempty"`
}

// MetricsResponse represents the cached metrics for API response
type MetricsResponse struct {
	FilesystemName string    `json:"filesystemName"`
	StorageType    string    `json:"storageType"`
	TotalBytes     uint64    `json:"totalBytes"`
	UsedBytes      uint64    `json:"usedBytes"`
	AvailableBytes uint64    `json:"availableBytes"`
	UsagePercent   float64   `json:"usagePercent"`
	TotalInodes    uint64    `json:"totalInodes,omitempty"`
	UsedInodes     uint64    `json:"usedInodes,omitempty"`
	FreeInodes     uint64    `json:"freeInodes,omitempty"`
	CollectedAt    time.Time `json:"collectedAt"`
	Error          string    `json:"error,omitempty"`
}

// handleListFilesystems returns the list of discovered filesystems
func (e *Exporter) handleListFilesystems(c *gin.Context) {
	filesystems := e.controller.GetFilesystems()
	result := make([]FilesystemResponse, 0, len(filesystems))

	for _, fs := range filesystems {
		result = append(result, FilesystemResponse{
			Name:             fs.Name,
			StorageClassName: fs.StorageClassName,
			FilesystemName:   fs.FilesystemName,
			StorageType:      fs.StorageType,
			VolumeType:       fs.VolumeType,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"filesystems": result,
		"count":       len(result),
	})
}

// handleHealthCheck returns the health status
func (e *Exporter) handleHealthCheck(c *gin.Context) {
	e.mu.RLock()
	lastCollected := e.lastCollected
	e.mu.RUnlock()

	filesystems := e.controller.GetFilesystems()
	metrics := e.controller.GetMetrics()

	healthy := len(filesystems) > 0 && len(metrics) > 0

	c.JSON(http.StatusOK, gin.H{
		"healthy":          healthy,
		"lastCollected":    lastCollected,
		"filesystemsCount": len(filesystems),
		"metricsCount":     len(metrics),
	})
}

// handleMetricsCache returns the cached metrics
func (e *Exporter) handleMetricsCache(c *gin.Context) {
	e.mu.RLock()
	lastCollected := e.lastCollected
	e.mu.RUnlock()

	metrics := e.controller.GetMetrics()
	result := make([]MetricsResponse, 0, len(metrics))

	for _, m := range metrics {
		resp := MetricsResponse{
			FilesystemName: m.FilesystemName,
			StorageType:    m.StorageType,
			TotalBytes:     m.TotalBytes,
			UsedBytes:      m.UsedBytes,
			AvailableBytes: m.AvailableBytes,
			UsagePercent:   m.UsagePercent,
			TotalInodes:    m.TotalInodes,
			UsedInodes:     m.UsedInodes,
			FreeInodes:     m.FreeInodes,
			CollectedAt:    m.CollectedAt,
		}
		if m.Error != nil {
			resp.Error = m.Error.Error()
		}
		result = append(result, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"lastCollected": lastCollected,
		"metrics":       result,
	})
}
