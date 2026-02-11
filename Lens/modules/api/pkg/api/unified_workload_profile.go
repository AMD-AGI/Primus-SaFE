// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Request / Response types =====

// WorkloadProfileGetRequest is the request for getting a single workload profile
type WorkloadProfileGetRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID string `json:"workload_uid" param:"workload_uid" mcp:"workload_uid,description=Workload UID,required"`
}

// WorkloadProfileGetResponse returns the full workload profile (detection + intent)
type WorkloadProfileGetResponse struct {
	// Phase 1: Detection fields
	WorkloadUID      string  `json:"workload_uid"`
	Framework        string  `json:"framework,omitempty"`
	Frameworks       string  `json:"frameworks,omitempty"`
	WorkloadType     string  `json:"workload_type,omitempty"`
	WrapperFramework string  `json:"wrapper_framework,omitempty"`
	BaseFramework    string  `json:"base_framework,omitempty"`
	Confidence       float64 `json:"confidence,omitempty"`

	// Phase 2: Intent fields
	Category          string      `json:"category,omitempty"`
	ExpectedBehavior  string      `json:"expected_behavior,omitempty"`
	ModelPath         string      `json:"model_path,omitempty"`
	ModelFamily       string      `json:"model_family,omitempty"`
	ModelScale        string      `json:"model_scale,omitempty"`
	ModelVariant      string      `json:"model_variant,omitempty"`
	RuntimeFramework  string      `json:"runtime_framework,omitempty"`
	IntentDetail      interface{} `json:"intent_detail,omitempty"`
	IntentConfidence  float64     `json:"intent_confidence,omitempty"`
	IntentSource      string      `json:"intent_source,omitempty"`
	IntentReasoning   string      `json:"intent_reasoning,omitempty"`
	IntentFieldSources interface{} `json:"intent_field_sources,omitempty"`
	IntentAnalysisMode string     `json:"intent_analysis_mode,omitempty"`
	IntentMatchedRules interface{} `json:"intent_matched_rules,omitempty"`
	IntentState       string      `json:"intent_state,omitempty"`
	IntentAnalyzedAt  string      `json:"intent_analyzed_at,omitempty"`
}

// WorkloadProfileListRequest is the request for listing workload profiles
type WorkloadProfileListRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Category  string `json:"category" query:"category" mcp:"category,description=Filter by intent category (pre_training, fine_tuning, inference, etc.)"`
	ModelFamily string `json:"model_family" query:"model_family" mcp:"model_family,description=Filter by model family (llama, qwen, etc.)"`
	Framework string `json:"framework" query:"framework" mcp:"framework,description=Filter by detected framework"`
	IntentState string `json:"intent_state" query:"intent_state" mcp:"intent_state,description=Filter by intent analysis state (pending, confirmed, etc.)"`
	MinConfidence float64 `json:"min_confidence" query:"min_confidence" mcp:"min_confidence,description=Minimum intent confidence threshold"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 20)"`
}

// WorkloadProfileListResponse returns a paginated list of workload profiles
type WorkloadProfileListResponse struct {
	Data  []WorkloadProfileGetResponse `json:"data"`
	Total int64                        `json:"total"`
}

// WorkloadProfileAnalyzeRequest triggers manual intent analysis
type WorkloadProfileAnalyzeRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID string `json:"workload_uid" param:"workload_uid" mcp:"workload_uid,description=Workload UID,required"`
}

// WorkloadProfileAnalyzeResponse returns the trigger result
type WorkloadProfileAnalyzeResponse struct {
	Message string `json:"message"`
	TaskID  int64  `json:"task_id,omitempty"`
}

// WorkloadProfileEvidenceRequest gets the evidence used for analysis
type WorkloadProfileEvidenceRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID string `json:"workload_uid" param:"workload_uid" mcp:"workload_uid,description=Workload UID,required"`
}

// WorkloadProfileEvidenceResponse returns the collected evidence
type WorkloadProfileEvidenceResponse struct {
	WorkloadUID string      `json:"workload_uid"`
	Evidence    interface{} `json:"evidence"` // Full IntentEvidence structure
}

// ===== Endpoint registration =====

func init() {
	unified.Register(&unified.EndpointDef[WorkloadProfileGetRequest, WorkloadProfileGetResponse]{
		Name:        "workload_profile_get",
		Description: "Get full workload profile including detection and intent analysis results",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-profile/:workload_uid",
		MCPToolName: "lens_workload_profile_get",
		Handler:     handleWorkloadProfileGet,
	})

	unified.Register(&unified.EndpointDef[WorkloadProfileListRequest, WorkloadProfileListResponse]{
		Name:        "workload_profile_list",
		Description: "List workload profiles with filtering by category, model family, framework, and confidence",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-profile",
		MCPToolName: "lens_workload_profile_list",
		Handler:     handleWorkloadProfileList,
	})

	unified.Register(&unified.EndpointDef[WorkloadProfileAnalyzeRequest, WorkloadProfileAnalyzeResponse]{
		Name:        "workload_profile_analyze",
		Description: "Manually trigger intent analysis for a specific workload",
		HTTPMethod:  "POST",
		HTTPPath:    "/workload-profile/:workload_uid/analyze",
		MCPToolName: "lens_workload_profile_analyze",
		Handler:     handleWorkloadProfileAnalyze,
	})

	unified.Register(&unified.EndpointDef[WorkloadProfileEvidenceRequest, WorkloadProfileEvidenceResponse]{
		Name:        "workload_profile_evidence",
		Description: "Get all collected evidence used for intent analysis of a workload",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-profile/:workload_uid/evidence",
		MCPToolName: "lens_workload_profile_evidence",
		Handler:     handleWorkloadProfileEvidence,
	})
}

// ===== Handlers =====

func handleWorkloadProfileGet(ctx context.Context, req *WorkloadProfileGetRequest) (*WorkloadProfileGetResponse, error) {
	if req.WorkloadUID == "" {
		return nil, errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("workload_uid is required")
	}

	cm := clientsets.GetClusterManager()
	clusterName, err := resolveWorkloadCluster(cm, req.WorkloadUID, req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName)
	det, err := facade.GetWorkloadDetection().GetDetection(ctx, req.WorkloadUID)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to get workload detection: " + err.Error())
	}
	if det == nil {
		return nil, errors.NewError().
			WithCode(errors.RequestDataNotExisted).
			WithMessage("workload profile not found for UID: " + req.WorkloadUID)
	}

	return convertDetectionToProfile(det), nil
}

func handleWorkloadProfileList(ctx context.Context, req *WorkloadProfileListRequest) (*WorkloadProfileListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)

	limit := pageSize
	offset := (pageNum - 1) * pageSize

	detections, total, err := facade.GetWorkloadDetection().ListByFilters(ctx,
		req.Category,
		req.ModelFamily,
		req.Framework,
		req.IntentState,
		req.MinConfidence,
		limit,
		offset,
	)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to list workload profiles: " + err.Error())
	}

	data := make([]WorkloadProfileGetResponse, 0, len(detections))
	for _, det := range detections {
		data = append(data, *convertDetectionToProfile(det))
	}

	return &WorkloadProfileListResponse{
		Data:  data,
		Total: total,
	}, nil
}

func handleWorkloadProfileAnalyze(ctx context.Context, req *WorkloadProfileAnalyzeRequest) (*WorkloadProfileAnalyzeResponse, error) {
	if req.WorkloadUID == "" {
		return nil, errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("workload_uid is required")
	}

	cm := clientsets.GetClusterManager()
	clusterName, err := resolveWorkloadCluster(cm, req.WorkloadUID, req.Cluster)
	if err != nil {
		return nil, err
	}

	// Reset intent_state to "pending" to trigger re-analysis
	facade := database.GetFacadeForCluster(clusterName)
	pending := "pending"
	err = facade.GetWorkloadDetection().UpdateIntentState(ctx, req.WorkloadUID, &pending)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to trigger analysis: " + err.Error())
	}

	return &WorkloadProfileAnalyzeResponse{
		Message: "Intent analysis triggered for workload " + req.WorkloadUID,
	}, nil
}

func handleWorkloadProfileEvidence(ctx context.Context, req *WorkloadProfileEvidenceRequest) (*WorkloadProfileEvidenceResponse, error) {
	if req.WorkloadUID == "" {
		return nil, errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("workload_uid is required")
	}

	cm := clientsets.GetClusterManager()
	clusterName, err := resolveWorkloadCluster(cm, req.WorkloadUID, req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName)

	// Collect evidence from multiple sources
	evidence := make(map[string]interface{})

	// Detection evidence records
	evidenceRecords, err := facade.GetWorkloadDetectionEvidence().ListByWorkloadUID(ctx, req.WorkloadUID)
	if err == nil && len(evidenceRecords) > 0 {
		evidence["detection_evidence"] = evidenceRecords
	}

	// Code snapshot
	snapshot, err := facade.GetWorkloadCodeSnapshot().GetByWorkloadUID(ctx, req.WorkloadUID)
	if err == nil && snapshot != nil {
		evidence["code_snapshot"] = snapshot
	}

	return &WorkloadProfileEvidenceResponse{
		WorkloadUID: req.WorkloadUID,
		Evidence:    evidence,
	}, nil
}

// ===== Conversion helpers =====

func convertDetectionToProfile(det *dbModel.WorkloadDetection) *WorkloadProfileGetResponse {
	resp := &WorkloadProfileGetResponse{
		WorkloadUID: det.WorkloadUID,
	}

	// Phase 1 fields
	if det.Framework != nil {
		resp.Framework = *det.Framework
	}
	if det.Frameworks != nil {
		resp.Frameworks = *det.Frameworks
	}
	if det.WorkloadType != nil {
		resp.WorkloadType = *det.WorkloadType
	}
	if det.WrapperFramework != nil {
		resp.WrapperFramework = *det.WrapperFramework
	}
	if det.BaseFramework != nil {
		resp.BaseFramework = *det.BaseFramework
	}
	if det.Confidence != nil {
		resp.Confidence = *det.Confidence
	}

	// Phase 2: Intent fields
	if det.Category != nil {
		resp.Category = *det.Category
	}
	if det.ExpectedBehavior != nil {
		resp.ExpectedBehavior = *det.ExpectedBehavior
	}
	if det.ModelPath != nil {
		resp.ModelPath = *det.ModelPath
	}
	if det.ModelFamily != nil {
		resp.ModelFamily = *det.ModelFamily
	}
	if det.ModelScale != nil {
		resp.ModelScale = *det.ModelScale
	}
	if det.ModelVariant != nil {
		resp.ModelVariant = *det.ModelVariant
	}
	if det.RuntimeFramework != nil {
		resp.RuntimeFramework = *det.RuntimeFramework
	}
	if det.IntentDetail != nil {
		resp.IntentDetail = det.IntentDetail
	}
	if det.IntentConfidence != nil {
		resp.IntentConfidence = *det.IntentConfidence
	}
	if det.IntentSource != nil {
		resp.IntentSource = *det.IntentSource
	}
	if det.IntentReasoning != nil {
		resp.IntentReasoning = *det.IntentReasoning
	}
	if det.IntentFieldSources != nil {
		resp.IntentFieldSources = det.IntentFieldSources
	}
	if det.IntentAnalysisMode != nil {
		resp.IntentAnalysisMode = *det.IntentAnalysisMode
	}
	if det.IntentMatchedRules != nil {
		resp.IntentMatchedRules = det.IntentMatchedRules
	}
	if det.IntentState != nil {
		resp.IntentState = *det.IntentState
	}
	if det.IntentAnalyzedAt != nil {
		resp.IntentAnalyzedAt = det.IntentAnalyzedAt.Format("2006-01-02T15:04:05Z")
	}

	return resp
}
