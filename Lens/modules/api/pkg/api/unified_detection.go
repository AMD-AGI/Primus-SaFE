// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// Detection Status endpoints
	unified.Register(&unified.EndpointDef[DetectionSummaryRequest, DetectionSummaryResponse]{
		Name:        "detection_summary",
		Description: "Get summary of all detection statuses across workloads",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-status/summary",
		MCPToolName: "lens_detection_summary",
		Handler:     handleDetectionSummary,
	})

	unified.Register(&unified.EndpointDef[DetectionStatusesListRequest, DetectionStatusesListResponse]{
		Name:        "detection_statuses",
		Description: "List detection statuses with filtering",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-status",
		MCPToolName: "lens_detection_statuses",
		Handler:     handleDetectionStatusesList,
	})

	unified.Register(&unified.EndpointDef[DetectionStatusDetailRequest, DetectionStatusResponse]{
		Name:        "detection_status",
		Description: "Get full detection status for a specific workload",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-status/:workload_uid",
		MCPToolName: "lens_detection_status",
		Handler:     handleDetectionStatusDetail,
	})

	unified.Register(&unified.EndpointDef[DetectionCoverageRequest, DetectionCoverageResponse]{
		Name:        "detection_coverage",
		Description: "Get detection coverage for a workload",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-status/:workload_uid/coverage",
		MCPToolName: "lens_detection_coverage",
		Handler:     handleDetectionCoverageGet,
	})

	unified.Register(&unified.EndpointDef[DetectionLogGapRequest, DetectionLogGapResponse]{
		Name:        "detection_log_gap",
		Description: "Get uncovered log time window for a workload",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-status/:workload_uid/coverage/log-gap",
		MCPToolName: "lens_detection_log_gap",
		Handler:     handleDetectionLogGap,
	})

	unified.Register(&unified.EndpointDef[DetectionTasksRequest, DetectionTasksResponse]{
		Name:        "detection_tasks",
		Description: "Get detection-related tasks for a workload",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-status/:workload_uid/tasks",
		MCPToolName: "lens_detection_tasks",
		Handler:     handleDetectionTasks,
	})

	unified.Register(&unified.EndpointDef[DetectionEvidenceRequest, DetectionEvidenceResponse]{
		Name:        "detection_evidence",
		Description: "Get evidence records for a workload",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-status/:workload_uid/evidence",
		MCPToolName: "lens_detection_evidence",
		Handler:     handleDetectionEvidence,
	})

	// Detection Config endpoints (GET only)
	unified.Register(&unified.EndpointDef[DetectionFrameworksListRequest, DetectionFrameworksListResponse]{
		Name:        "detection_frameworks",
		Description: "List all enabled framework names for detection",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-configs/frameworks",
		MCPToolName: "lens_detection_frameworks",
		Handler:     handleDetectionFrameworksList,
	})

	unified.Register(&unified.EndpointDef[DetectionFrameworkConfigRequest, FrameworkLogPatterns]{
		Name:        "detection_framework_config",
		Description: "Get configuration for a specific framework",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-configs/frameworks/:name",
		MCPToolName: "lens_detection_framework_config",
		Handler:     handleDetectionFrameworkConfig,
	})

	unified.Register(&unified.EndpointDef[DetectionCacheTTLRequest, CacheTTLResponse]{
		Name:        "detection_cache_ttl",
		Description: "Get the cache TTL configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/detection-configs/cache/ttl",
		MCPToolName: "lens_detection_cache_ttl",
		Handler:     handleDetectionCacheTTL,
	})
}

// ======================== Request Types ========================

type DetectionSummaryRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
}

type DetectionStatusesListRequest struct {
	Cluster        string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Status         string `json:"status" form:"status" mcp:"description=Filter by detection status"`
	DetectionState string `json:"state" form:"state" mcp:"description=Filter by detection state"`
	Page           int    `json:"page" form:"page" mcp:"description=Page number (default 1)"`
	PageSize       int    `json:"page_size" form:"page_size" mcp:"description=Page size (default 20 max 100)"`
}

type DetectionStatusDetailRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	WorkloadUID string `json:"workload_uid" form:"workload_uid" param:"workload_uid" binding:"required" mcp:"description=Workload UID,required"`
}

type DetectionCoverageRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	WorkloadUID string `json:"workload_uid" form:"workload_uid" param:"workload_uid" binding:"required" mcp:"description=Workload UID,required"`
}

type DetectionLogGapRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	WorkloadUID string `json:"workload_uid" form:"workload_uid" param:"workload_uid" binding:"required" mcp:"description=Workload UID,required"`
}

type DetectionTasksRequest struct {
	WorkloadUID string `json:"workload_uid" form:"workload_uid" param:"workload_uid" binding:"required" mcp:"description=Workload UID,required"`
}

type DetectionEvidenceRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	WorkloadUID string `json:"workload_uid" form:"workload_uid" param:"workload_uid" binding:"required" mcp:"description=Workload UID,required"`
	Source      string `json:"source" form:"source" mcp:"description=Filter by evidence source"`
}

type DetectionFrameworksListRequest struct{}

type DetectionFrameworkConfigRequest struct {
	Name string `json:"name" form:"name" param:"name" binding:"required" mcp:"description=Framework name,required"`
}

type DetectionCacheTTLRequest struct{}

// ======================== Response Types ========================

type DetectionStatusesListResponse struct {
	Data     []DetectionStatusResponse `json:"data"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

type DetectionCoverageResponse struct {
	WorkloadUID string                  `json:"workload_uid"`
	Coverage    []DetectionCoverageItem `json:"coverage"`
	Total       int                     `json:"total"`
}

type DetectionLogGapResponse struct {
	WorkloadUID        string     `json:"workload_uid"`
	HasGap             bool       `json:"has_gap"`
	GapFrom            *time.Time `json:"gap_from,omitempty"`
	GapTo              *time.Time `json:"gap_to,omitempty"`
	GapDurationSeconds float64    `json:"gap_duration_seconds,omitempty"`
}

type DetectionTasksResponse struct {
	WorkloadUID string              `json:"workload_uid"`
	Tasks       []DetectionTaskItem `json:"tasks"`
	Total       int                 `json:"total"`
}

type DetectionEvidenceResponse struct {
	WorkloadUID string                  `json:"workload_uid"`
	Evidence    []DetectionEvidenceItem `json:"evidence"`
	Total       int                     `json:"total"`
}

type DetectionFrameworksListResponse struct {
	Frameworks []string `json:"frameworks"`
	Total      int      `json:"total"`
}

// ======================== Handler Implementations ========================

func handleDetectionSummary(ctx context.Context, req *DetectionSummaryRequest) (*DetectionSummaryResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	db := database.GetFacadeForCluster(clients.ClusterName).GetSystemConfig().GetDB()

	// Count total workloads with detection
	var totalWorkloads int64
	if err := db.WithContext(context.Background()).
		Table(dbmodel.TableNameWorkloadDetection).
		Count(&totalWorkloads).Error; err != nil {
		return nil, errors.WrapError(err, "failed to count detections", errors.CodeDatabaseError)
	}

	// Count by status
	type StatusCount struct {
		Status string `gorm:"column:status"`
		Count  int64  `gorm:"column:count"`
	}
	var statusCounts []StatusCount
	db.WithContext(context.Background()).
		Table(dbmodel.TableNameWorkloadDetection).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&statusCounts)

	statusCountMap := make(map[string]int64)
	for _, sc := range statusCounts {
		statusCountMap[sc.Status] = sc.Count
	}

	// Count by detection state
	var stateCounts []StatusCount
	db.WithContext(context.Background()).
		Table(dbmodel.TableNameWorkloadDetection).
		Select("detection_state as status, COUNT(*) as count").
		Group("detection_state").
		Scan(&stateCounts)

	stateCountMap := make(map[string]int64)
	for _, sc := range stateCounts {
		stateCountMap[sc.Status] = sc.Count
	}

	// Get recent detections
	var recentDetections []*dbmodel.WorkloadDetection
	db.WithContext(context.Background()).
		Table(dbmodel.TableNameWorkloadDetection).
		Order("updated_at DESC").
		Limit(10).
		Find(&recentDetections)

	recentResponses := make([]DetectionStatusResponse, 0, len(recentDetections))
	for _, d := range recentDetections {
		recentResponses = append(recentResponses, buildDetectionStatusResponse(d, nil, nil))
	}

	return &DetectionSummaryResponse{
		TotalWorkloads:       totalWorkloads,
		StatusCounts:         statusCountMap,
		DetectionStateCounts: stateCountMap,
		RecentDetections:     recentResponses,
	}, nil
}

func handleDetectionStatusesList(ctx context.Context, req *DetectionStatusesListRequest) (*DetectionStatusesListResponse, error) {
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

	facade := database.GetFacadeForCluster(clients.ClusterName)
	detectionFacade := facade.GetWorkloadDetection()

	var detections []*dbmodel.WorkloadDetection
	var total int64

	if req.Status != "" {
		detections, total, err = detectionFacade.ListDetectionsByStatus(context.Background(), req.Status, pageSize, offset)
	} else if req.DetectionState != "" {
		allDetections, err2 := detectionFacade.ListDetectionsByDetectionState(context.Background(), req.DetectionState)
		if err2 != nil {
			return nil, errors.WrapError(err2, "failed to list detections", errors.CodeDatabaseError)
		}
		total = int64(len(allDetections))
		start := offset
		end := offset + pageSize
		if start > len(allDetections) {
			start = len(allDetections)
		}
		if end > len(allDetections) {
			end = len(allDetections)
		}
		detections = allDetections[start:end]
	} else {
		db := facade.GetSystemConfig().GetDB()
		err = db.WithContext(context.Background()).
			Table(dbmodel.TableNameWorkloadDetection).
			Count(&total).Error
		if err == nil {
			err = db.WithContext(context.Background()).
				Table(dbmodel.TableNameWorkloadDetection).
				Order("updated_at DESC").
				Limit(pageSize).
				Offset(offset).
				Find(&detections).Error
		}
	}

	if err != nil {
		return nil, errors.WrapError(err, "failed to list detections", errors.CodeDatabaseError)
	}

	responses := make([]DetectionStatusResponse, 0, len(detections))
	for _, d := range detections {
		responses = append(responses, buildDetectionStatusResponse(d, nil, nil))
	}

	return &DetectionStatusesListResponse{
		Data:     responses,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func handleDetectionStatusDetail(ctx context.Context, req *DetectionStatusDetailRequest) (*DetectionStatusResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)

	detection, err := facade.GetWorkloadDetection().GetDetection(context.Background(), req.WorkloadUID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get detection", errors.CodeDatabaseError)
	}

	if detection == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("detection not found")
	}

	coverages, _ := facade.GetDetectionCoverage().ListCoverageByWorkload(context.Background(), req.WorkloadUID)
	taskFacade := database.NewWorkloadTaskFacade()
	tasks, _ := taskFacade.ListTasksByWorkload(context.Background(), req.WorkloadUID)
	detectionTasks := filterDetectionTasks(tasks)

	response := buildDetectionStatusResponse(detection, coverages, detectionTasks)
	return &response, nil
}

func handleDetectionCoverageGet(ctx context.Context, req *DetectionCoverageRequest) (*DetectionCoverageResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	coverages, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionCoverage().
		ListCoverageByWorkload(context.Background(), req.WorkloadUID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get coverage", errors.CodeDatabaseError)
	}

	items := make([]DetectionCoverageItem, 0, len(coverages))
	for _, c := range coverages {
		items = append(items, buildCoverageItem(c))
	}

	return &DetectionCoverageResponse{
		WorkloadUID: req.WorkloadUID,
		Coverage:    items,
		Total:       len(items),
	}, nil
}

func handleDetectionLogGap(ctx context.Context, req *DetectionLogGapRequest) (*DetectionLogGapResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	coverageFacade := database.GetFacadeForCluster(clients.ClusterName).GetDetectionCoverage()
	from, to, err := coverageFacade.GetUncoveredLogWindow(context.Background(), req.WorkloadUID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get log window", errors.CodeDatabaseError)
	}

	response := &DetectionLogGapResponse{
		WorkloadUID: req.WorkloadUID,
		HasGap:      from != nil && to != nil,
	}

	if from != nil && to != nil {
		response.GapFrom = from
		response.GapTo = to
		response.GapDurationSeconds = to.Sub(*from).Seconds()
	}

	return response, nil
}

func handleDetectionTasks(ctx context.Context, req *DetectionTasksRequest) (*DetectionTasksResponse, error) {
	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	taskFacade := database.NewWorkloadTaskFacade()
	tasks, err := taskFacade.ListTasksByWorkload(context.Background(), req.WorkloadUID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get tasks", errors.CodeDatabaseError)
	}

	detectionTasks := filterDetectionTasks(tasks)

	items := make([]DetectionTaskItem, 0, len(detectionTasks))
	for _, t := range detectionTasks {
		items = append(items, buildTaskItem(t))
	}

	return &DetectionTasksResponse{
		WorkloadUID: req.WorkloadUID,
		Tasks:       items,
		Total:       len(items),
	}, nil
}

func handleDetectionEvidence(ctx context.Context, req *DetectionEvidenceRequest) (*DetectionEvidenceResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	db := database.GetFacadeForCluster(clients.ClusterName).GetSystemConfig().GetDB()

	var evidenceRecords []*dbmodel.WorkloadDetectionEvidence
	query := db.WithContext(context.Background()).
		Table(dbmodel.TableNameWorkloadDetectionEvidence).
		Where("workload_uid = ?", req.WorkloadUID).
		Order("detected_at DESC")

	if req.Source != "" {
		query = query.Where("source = ?", req.Source)
	}

	if err := query.Find(&evidenceRecords).Error; err != nil {
		return nil, errors.WrapError(err, "failed to get evidence", errors.CodeDatabaseError)
	}

	items := make([]DetectionEvidenceItem, 0, len(evidenceRecords))
	for _, e := range evidenceRecords {
		item := DetectionEvidenceItem{
			ID:               e.ID,
			WorkloadUID:      e.WorkloadUID,
			Source:           e.Source,
			SourceType:       e.SourceType,
			Framework:        e.Framework,
			WorkloadType:     e.WorkloadType,
			Confidence:       e.Confidence,
			FrameworkLayer:   e.FrameworkLayer,
			WrapperFramework: e.WrapperFramework,
			BaseFramework:    e.BaseFramework,
			Evidence:         e.Evidence,
			DetectedAt:       e.DetectedAt,
			CreatedAt:        e.CreatedAt,
		}
		if e.BaseFramework != "" {
			item.OrchestrationFramework = e.BaseFramework
		}
		items = append(items, item)
	}

	return &DetectionEvidenceResponse{
		WorkloadUID: req.WorkloadUID,
		Evidence:    items,
		Total:       len(items),
	}, nil
}

func handleDetectionFrameworksList(ctx context.Context, req *DetectionFrameworksListRequest) (*DetectionFrameworksListResponse, error) {
	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())

	configs, err := configMgr.List(context.Background(), config.WithKeyPrefixFilter(ConfigKeyPrefix+"."))
	if err != nil {
		return nil, errors.WrapError(err, "failed to list framework configs", errors.CodeDatabaseError)
	}

	var enabledFrameworks []string
	prefix := ConfigKeyPrefix + "."

	for _, cfg := range configs {
		if len(cfg.Key) <= len(prefix) {
			continue
		}
		name := cfg.Key[len(prefix):]
		if len(name) == 0 || name[0] == '.' {
			continue
		}

		var patterns FrameworkLogPatterns
		if err := configMgr.Get(context.Background(), cfg.Key, &patterns); err != nil {
			log.Debugf("Failed to parse framework config %s: %v", cfg.Key, err)
			continue
		}

		if patterns.Enabled {
			enabledFrameworks = append(enabledFrameworks, name)
		}
	}

	return &DetectionFrameworksListResponse{
		Frameworks: enabledFrameworks,
		Total:      len(enabledFrameworks),
	}, nil
}

func handleDetectionFrameworkConfig(ctx context.Context, req *DetectionFrameworkConfigRequest) (*FrameworkLogPatterns, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("framework name is required")
	}

	configMgr := config.NewManager(database.GetFacade().GetSystemConfig().GetDB())
	configKey := ConfigKeyPrefix + "." + req.Name

	var patterns FrameworkLogPatterns
	err := configMgr.Get(context.Background(), configKey, &patterns)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get framework config", errors.CodeDatabaseError)
	}

	return &patterns, nil
}

func handleDetectionCacheTTL(ctx context.Context, req *DetectionCacheTTLRequest) (*CacheTTLResponse, error) {
	defaultTTL := 5 * time.Minute

	return &CacheTTLResponse{
		TTLSeconds:  int(defaultTTL.Seconds()),
		LastRefresh: time.Now(),
		IsExpired:   false,
	}, nil
}
