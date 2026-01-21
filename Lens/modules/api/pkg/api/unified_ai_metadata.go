// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// AI Workload Metadata (GET only)
	unified.Register(&unified.EndpointDef[AiMetadataListRequest, AiMetadataListResponse]{
		Name:        "ai_metadata_list",
		Description: "List AI workload metadata with filtering options",
		HTTPMethod:  "GET",
		HTTPPath:    "/ai-workload-metadata",
		MCPToolName: "lens_ai_metadata_list",
		Handler:     handleAiMetadataList,
	})

	unified.Register(&unified.EndpointDef[AiMetadataDetailRequest, AiWorkloadMetadataResponse]{
		Name:        "ai_metadata_detail",
		Description: "Get AI workload metadata by workload UID with conflict information",
		HTTPMethod:  "GET",
		HTTPPath:    "/ai-workload-metadata/:workload_uid",
		MCPToolName: "lens_ai_metadata_detail",
		Handler:     handleAiMetadataDetail,
	})

	unified.Register(&unified.EndpointDef[AiMetadataConflictsRequest, AiMetadataConflictsResponse]{
		Name:        "ai_metadata_conflicts",
		Description: "Get detection conflict logs for a specific workload",
		HTTPMethod:  "GET",
		HTTPPath:    "/ai-workload-metadata/:workload_uid/conflicts",
		MCPToolName: "lens_ai_metadata_conflicts",
		Handler:     handleAiMetadataConflicts,
	})

	// Detection Conflicts (all recent conflicts)
	unified.Register(&unified.EndpointDef[DetectionConflictsRequest, DetectionConflictsResponse]{
		Name:        "detection_conflicts",
		Description: "List all recent detection conflicts across workloads",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-conflicts",
		MCPToolName: "lens_detection_conflicts",
		Handler:     handleDetectionConflicts,
	})
}

// ======================== Request Types ========================

type AiMetadataListRequest struct {
	Cluster          string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	Framework        string `json:"framework" form:"framework" mcp:"description=Search in wrapper_framework and base_framework"`
	WrapperFramework string `json:"wrapper_framework" form:"wrapper_framework" mcp:"description=Specific wrapper framework filter"`
	BaseFramework    string `json:"base_framework" form:"base_framework" mcp:"description=Specific base framework filter"`
	Type             string `json:"type" form:"type" mcp:"description=Workload type filter"`
	HasConflict      string `json:"has_conflict" form:"has_conflict" mcp:"description=Filter by conflict status (true/false)"`
	Page             int    `json:"page" form:"page" mcp:"description=Page number"`
	PageSize         int    `json:"page_size" form:"page_size" mcp:"description=Page size"`
}

type AiMetadataDetailRequest struct {
	Cluster     string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	WorkloadUID string `json:"workload_uid" form:"workload_uid" uri:"workload_uid" binding:"required" mcp:"description=Workload UID,required"`
}

type AiMetadataConflictsRequest struct {
	Cluster     string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	WorkloadUID string `json:"workload_uid" form:"workload_uid" uri:"workload_uid" binding:"required" mcp:"description=Workload UID,required"`
	Page        int    `json:"page" form:"page" mcp:"description=Page number (default 1)"`
	PageSize    int    `json:"page_size" form:"page_size" mcp:"description=Page size (default 20 max 100)"`
}

type DetectionConflictsRequest struct {
	Cluster  string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	Page     int    `json:"page" form:"page" mcp:"description=Page number (default 1)"`
	PageSize int    `json:"page_size" form:"page_size" mcp:"description=Page size (default 20 max 100)"`
}

// ======================== Response Types ========================

type AiMetadataListResponse struct {
	Data  []AiWorkloadMetadataResponse `json:"data"`
	Total int                          `json:"total"`
}

type AiMetadataConflictsResponse struct {
	Data     []DetectionConflictLogDetail `json:"data"`
	Total    int64                        `json:"total"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
}

type DetectionConflictsResponse struct {
	Data     []DetectionConflictLogDetail `json:"data"`
	Total    int64                        `json:"total"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
}

// ======================== Handler Implementations ========================

func handleAiMetadataList(ctx context.Context, req *AiMetadataListRequest) (*AiMetadataListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	db := database.GetFacadeForCluster(clients.ClusterName).GetAiWorkloadMetadata()

	// Get all metadata
	allMetadata, err := db.ListAiWorkloadMetadataByUIDs(context.Background(), []string{})
	if err != nil {
		return nil, errors.WrapError(err, "failed to list metadata", errors.CodeDatabaseError)
	}

	// Parse has_conflict filter
	var hasConflictFilter *bool
	if req.HasConflict != "" {
		if req.HasConflict == "true" {
			v := true
			hasConflictFilter = &v
		} else if req.HasConflict == "false" {
			v := false
			hasConflictFilter = &v
		}
	}

	// Build responses with conflict information
	responses := []AiWorkloadMetadataResponse{}
	for _, metadata := range allMetadata {
		// Get conflict logs
		conflicts, _, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionConflictLog().
			ListDetectionConflictLogsByWorkloadUID(context.Background(), metadata.WorkloadUID, 100, 0)
		if err != nil {
			log.Warnf("Failed to get conflict logs for workload %s: %v", metadata.WorkloadUID, err)
			conflicts = []*dbmodel.DetectionConflictLog{}
		}

		response := buildMetadataResponseWithConflicts(metadata, conflicts)

		// Apply filters
		if req.Type != "" && response.Type != req.Type {
			continue
		}

		// Framework filter
		if req.Framework != "" {
			matched := false
			if response.Framework == req.Framework ||
				response.WrapperFramework == req.Framework ||
				response.BaseFramework == req.Framework {
				matched = true
			}
			if !matched {
				continue
			}
		}

		if req.WrapperFramework != "" && response.WrapperFramework != req.WrapperFramework {
			continue
		}

		if req.BaseFramework != "" && response.BaseFramework != req.BaseFramework {
			continue
		}

		if hasConflictFilter != nil && *hasConflictFilter != response.HasConflicts {
			continue
		}

		responses = append(responses, response)
	}

	return &AiMetadataListResponse{
		Data:  responses,
		Total: len(responses),
	}, nil
}

func handleAiMetadataDetail(ctx context.Context, req *AiMetadataDetailRequest) (*AiWorkloadMetadataResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	// Get metadata
	metadata, err := database.GetFacadeForCluster(clients.ClusterName).GetAiWorkloadMetadata().
		GetAiWorkloadMetadata(context.Background(), req.WorkloadUID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get metadata", errors.CodeDatabaseError)
	}

	if metadata == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("metadata not found")
	}

	// Get conflict logs
	conflicts, _, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionConflictLog().
		ListDetectionConflictLogsByWorkloadUID(context.Background(), req.WorkloadUID, 100, 0)
	if err != nil {
		log.Warnf("Failed to get conflict logs: %v", err)
		conflicts = []*dbmodel.DetectionConflictLog{}
	}

	response := buildMetadataResponseWithConflicts(metadata, conflicts)
	return &response, nil
}

func handleAiMetadataConflicts(ctx context.Context, req *AiMetadataConflictsRequest) (*AiMetadataConflictsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	conflicts, total, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionConflictLog().
		ListDetectionConflictLogsByWorkloadUID(context.Background(), req.WorkloadUID, pageSize, offset)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get conflict logs", errors.CodeDatabaseError)
	}

	details := make([]DetectionConflictLogDetail, 0, len(conflicts))
	for _, conflict := range conflicts {
		details = append(details, convertConflictToDetail(conflict))
	}

	return &AiMetadataConflictsResponse{
		Data:     details,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func handleDetectionConflicts(ctx context.Context, req *DetectionConflictsRequest) (*DetectionConflictsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	conflicts, total, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionConflictLog().
		ListRecentConflicts(context.Background(), pageSize, offset)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list conflict logs", errors.CodeDatabaseError)
	}

	details := make([]DetectionConflictLogDetail, 0, len(conflicts))
	for _, conflict := range conflicts {
		details = append(details, convertConflictToDetail(conflict))
	}

	return &DetectionConflictsResponse{
		Data:     details,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
