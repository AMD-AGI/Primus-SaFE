// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package exporter

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MountInfo represents mount information for API response
type MountInfo struct {
	Name           string `json:"name"`
	MountPath      string `json:"mountPath"`
	StorageType    string `json:"storageType"`
	FilesystemName string `json:"filesystemName"`
}

// MetricsResponse represents the cached metrics for API response
type MetricsResponse struct {
	Name           string  `json:"name"`
	MountPath      string  `json:"mountPath"`
	StorageType    string  `json:"storageType"`
	FilesystemName string  `json:"filesystemName"`
	TotalBytes     uint64  `json:"totalBytes"`
	UsedBytes      uint64  `json:"usedBytes"`
	AvailableBytes uint64  `json:"availableBytes"`
	UsagePercent   float64 `json:"usagePercent"`
	TotalInodes    uint64  `json:"totalInodes,omitempty"`
	UsedInodes     uint64  `json:"usedInodes,omitempty"`
	FreeInodes     uint64  `json:"freeInodes,omitempty"`
	Error          string  `json:"error,omitempty"`
}

// handleListMounts returns the list of configured mounts
func (e *Exporter) handleListMounts(c *gin.Context) {
	mounts := e.collector.GetMounts()
	result := make([]MountInfo, 0, len(mounts))

	for _, m := range mounts {
		result = append(result, MountInfo{
			Name:           m.Name,
			MountPath:      m.MountPath,
			StorageType:    m.StorageType,
			FilesystemName: m.FilesystemName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"mounts": result,
		"count":  len(result),
	})
}

// handleHealthCheck returns the health status
func (e *Exporter) handleHealthCheck(c *gin.Context) {
	e.mu.RLock()
	lastCollected := e.lastCollected
	metricsCount := len(e.metricsCache)
	e.mu.RUnlock()

	healthy := !lastCollected.IsZero()

	c.JSON(http.StatusOK, gin.H{
		"healthy":       healthy,
		"lastCollected": lastCollected,
		"mountsCount":   len(e.collector.GetMounts()),
		"metricsCount":  metricsCount,
	})
}

// handleMetricsCache returns the cached metrics
func (e *Exporter) handleMetricsCache(c *gin.Context) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]MetricsResponse, 0, len(e.metricsCache))
	for _, m := range e.metricsCache {
		resp := MetricsResponse{
			Name:           m.Name,
			MountPath:      m.MountPath,
			StorageType:    m.StorageType,
			FilesystemName: m.FilesystemName,
			TotalBytes:     m.TotalBytes,
			UsedBytes:      m.UsedBytes,
			AvailableBytes: m.AvailableBytes,
			UsagePercent:   m.UsagePercent,
			TotalInodes:    m.TotalInodes,
			UsedInodes:     m.UsedInodes,
			FreeInodes:     m.FreeInodes,
		}
		if m.Error != nil {
			resp.Error = m.Error.Error()
		}
		result = append(result, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"lastCollected": e.lastCollected,
		"metrics":       result,
	})
}
