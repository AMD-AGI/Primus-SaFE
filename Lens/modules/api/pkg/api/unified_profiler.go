// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ============================================================================
// Profiler Files GET Endpoints
// ============================================================================

// ProfilerFileListItem represents a profiler file in the list response
type ProfilerFileListItem struct {
	ID          int32  `gorm:"column:id" json:"id"`
	FileName    string `gorm:"column:file_name" json:"file_name"`
	FileType    string `gorm:"column:file_type" json:"file_type"`
	FileSize    int64  `gorm:"column:file_size" json:"file_size"`
	WorkloadUID string `gorm:"column:workload_uid" json:"workload_uid"`
	CreatedAt   string `gorm:"column:created_at" json:"created_at"`
}

// ProfilerFileInfo represents detailed profiler file metadata
type ProfilerFileInfo struct {
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

// --- List Profiler Files ---

type ListProfilerFilesRequest struct {
	WorkloadUID string `json:"workload_uid" mcp:"required,desc=The UID of the workload to list profiler files for"`
	Cluster     string `json:"cluster" mcp:"desc=Cluster name (optional, uses default if not provided)"`
}

type ListProfilerFilesResponse struct {
	Files []ProfilerFileListItem `json:"files"`
}

func handleListProfilerFiles(ctx context.Context, req *ListProfilerFilesRequest) (*ListProfilerFilesResponse, error) {
	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)
	db := facade.GetTraceLensSession().GetDB()

	var files []ProfilerFileListItem
	err = db.WithContext(ctx).
		Table("profiler_files").
		Select("id, file_name, file_type, file_size, workload_uid, created_at").
		Where("workload_uid = ?", req.WorkloadUID).
		Order("created_at DESC").
		Find(&files).Error

	if err != nil {
		return nil, errors.WrapError(err, "failed to list profiler files", errors.CodeDatabaseError)
	}

	return &ListProfilerFilesResponse{Files: files}, nil
}

// --- Get Profiler File Info ---

type GetProfilerFileInfoRequest struct {
	FileID  int32  `json:"file_id" mcp:"required,desc=The ID of the profiler file"`
	Cluster string `json:"cluster" mcp:"desc=Cluster name (optional, uses default if not provided)"`
}

type GetProfilerFileInfoResponse struct {
	FileInfo *ProfilerFileInfo `json:"file_info"`
}

func handleGetProfilerFileInfo(ctx context.Context, req *GetProfilerFileInfoRequest) (*GetProfilerFileInfoResponse, error) {
	if req.FileID <= 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid file_id")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)
	db := facade.GetTraceLensSession().GetDB()

	var fileInfo ProfilerFileInfo
	err = db.WithContext(ctx).
		Table("profiler_files").
		Where("id = ?", req.FileID).
		First(&fileInfo).Error

	if err != nil {
		return nil, errors.WrapError(err, "file not found", errors.RequestDataNotExisted)
	}

	return &GetProfilerFileInfoResponse{FileInfo: &fileInfo}, nil
}

// ============================================================================
// Unified Registration
// ============================================================================

func init() {
	// List profiler files for a workload
	unified.Register(&unified.EndpointDef[ListProfilerFilesRequest, ListProfilerFilesResponse]{
		HTTPPath:    "/profiler/files",
		HTTPMethod:  "GET",
		MCPToolName: "lens_list_profiler_files",
		Description: "List profiler files for a workload",
		Handler:     handleListProfilerFiles,
	})

	// Get profiler file metadata
	unified.Register(&unified.EndpointDef[GetProfilerFileInfoRequest, GetProfilerFileInfoResponse]{
		HTTPPath:    "/profiler/files/:id",
		HTTPMethod:  "GET",
		MCPToolName: "lens_get_profiler_file_info",
		Description: "Get metadata about a profiler file",
		Handler:     handleGetProfilerFileInfo,
	})

	// Note: /profiler/files/:id/content is NOT migrated because it returns binary content
}
