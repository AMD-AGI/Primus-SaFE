// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"fmt"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	cpmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
)

func init() {
	// TraceLens Session endpoints (GET only)
	// Session data is now centralized in Control Plane database
	unified.Register(&unified.EndpointDef[TraceLensResourceProfilesRequest, TraceLensResourceProfilesResponse]{
		Name:        "tracelens_resource_profiles",
		Description: "Get available resource profiles for TraceLens sessions (CPU/memory configurations)",
		HTTPMethod:  "GET",
		HTTPPath:    "/tracelens/resource-profiles",
		MCPToolName: "lens_tracelens_resource_profiles",
		Handler:     handleTraceLensResourceProfiles,
	})

	unified.Register(&unified.EndpointDef[TraceLensSessionsListRequest, TraceLensSessionsListResponse]{
		Name:        "tracelens_sessions_list",
		Description: "List all active TraceLens analysis sessions (from all clusters)",
		HTTPMethod:  "GET",
		HTTPPath:    "/tracelens/sessions",
		MCPToolName: "lens_tracelens_sessions_list",
		Handler:     handleTraceLensSessionsList,
	})

	unified.Register(&unified.EndpointDef[TraceLensSessionStatsRequest, TraceLensSessionStatsResponse]{
		Name:        "tracelens_session_stats",
		Description: "Get session statistics by status and cluster",
		HTTPMethod:  "GET",
		HTTPPath:    "/tracelens/sessions/stats",
		MCPToolName: "lens_tracelens_session_stats",
		Handler:     handleTraceLensSessionStats,
	})

	unified.Register(&unified.EndpointDef[TraceLensSessionGetRequest, TraceLensSessionResponse]{
		Name:        "tracelens_session_get",
		Description: "Get a specific TraceLens session by session ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/tracelens/sessions/:session_id",
		MCPToolName: "lens_tracelens_session_get",
		Handler:     handleTraceLensSessionGet,
	})

	unified.Register(&unified.EndpointDef[TraceLensWorkloadSessionsRequest, TraceLensSessionsListResponse]{
		Name:        "tracelens_workload_sessions",
		Description: "List all TraceLens sessions for a specific workload",
		HTTPMethod:  "GET",
		HTTPPath:    "/tracelens/workloads/:workload_uid/sessions",
		MCPToolName: "lens_tracelens_workload_sessions",
		Handler:     handleTraceLensWorkloadSessions,
	})
}

// ======================== Request Types ========================

type TraceLensResourceProfilesRequest struct{}

type TraceLensSessionsListRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Optional cluster filter"`
}

type TraceLensSessionStatsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Optional cluster filter"`
}

type TraceLensSessionGetRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"description=Cluster name (optional - sessions are in CP DB)"`
	SessionID string `json:"session_id" form:"session_id" param:"session_id" binding:"required" mcp:"description=Session ID,required"`
}

type TraceLensWorkloadSessionsRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"description=Optional cluster filter"`
	WorkloadUID string `json:"workload_uid" form:"workload_uid" param:"workload_uid" binding:"required" mcp:"description=Workload UID,required"`
}

// ======================== Response Types ========================

type TraceLensResourceProfilesResponse struct {
	Profiles []TraceLensResourceProfile `json:"profiles"`
}

type TraceLensResourceProfile struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Memory      string `json:"memory"`
	MemoryBytes int64  `json:"memory_bytes"`
	CPU         int    `json:"cpu"`
	IsDefault   bool   `json:"is_default"`
}

type TraceLensSessionsListResponse struct {
	Sessions []TraceLensSessionResponse `json:"sessions"`
	Total    int                        `json:"total"`
}

type TraceLensSessionResponse struct {
	SessionID       string     `json:"session_id"`
	ClusterName     string     `json:"cluster_name"`
	WorkloadUID     string     `json:"workload_uid"`
	ProfilerFileID  int32      `json:"profiler_file_id"`
	Status          string     `json:"status"`
	StatusMessage   string     `json:"status_message,omitempty"`
	PodName         string     `json:"pod_name,omitempty"`
	PodIP           string     `json:"pod_ip,omitempty"`
	ResourceProfile string     `json:"resource_profile"`
	UIPath          string     `json:"ui_path,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ExpiresAt       time.Time  `json:"expires_at"`
	ReadyAt         *time.Time `json:"ready_at,omitempty"`
	LastAccessedAt  *time.Time `json:"last_accessed_at,omitempty"`
	EstimatedReady  int        `json:"estimated_ready,omitempty"`
}

type TraceLensSessionStatsResponse struct {
	ByStatus  map[string]int `json:"by_status"`
	ByCluster map[string]int `json:"by_cluster"`
}

// ======================== Handler Implementations ========================

func handleTraceLensResourceProfiles(ctx context.Context, req *TraceLensResourceProfilesRequest) (*TraceLensResourceProfilesResponse, error) {
	profiles := make([]TraceLensResourceProfile, 0, len(tlconst.ResourceProfiles))
	for _, p := range tlconst.ResourceProfiles {
		profiles = append(profiles, TraceLensResourceProfile{
			Value:       p.Value,
			Label:       p.Label,
			Description: p.Description,
			Memory:      p.Memory,
			MemoryBytes: p.MemoryBytes,
			CPU:         p.CPU,
			IsDefault:   p.IsDefault,
		})
	}

	return &TraceLensResourceProfilesResponse{
		Profiles: profiles,
	}, nil
}

func handleTraceLensSessionsList(ctx context.Context, req *TraceLensSessionsListRequest) (*TraceLensSessionsListResponse, error) {
	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		return nil, errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("control plane not available")
	}
	facade := cpFacade.GetTraceLensSession()

	var sessions []*cpmodel.TracelensSessions
	var err error

	if req.Cluster != "" {
		// Filter by cluster - get all sessions for the cluster and filter active ones
		allSessions, err := facade.ListByCluster(ctx, req.Cluster)
		if err != nil {
			return nil, errors.WrapError(err, "failed to list sessions", errors.CodeDatabaseError)
		}
		sessions = make([]*cpmodel.TracelensSessions, 0)
		for _, s := range allSessions {
			if isActiveSessionStatus(s.Status) {
				sessions = append(sessions, s)
			}
		}
	} else {
		// List all active sessions across all clusters
		sessions, err = facade.ListActive(ctx)
		if err != nil {
			return nil, errors.WrapError(err, "failed to list sessions", errors.CodeDatabaseError)
		}
	}

	resp := &TraceLensSessionsListResponse{
		Sessions: make([]TraceLensSessionResponse, 0, len(sessions)),
		Total:    len(sessions),
	}
	for _, s := range sessions {
		resp.Sessions = append(resp.Sessions, *toTraceLensSessionResponse(s))
	}

	return resp, nil
}

func handleTraceLensSessionStats(ctx context.Context, req *TraceLensSessionStatsRequest) (*TraceLensSessionStatsResponse, error) {
	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		return nil, errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("control plane not available")
	}
	facade := cpFacade.GetTraceLensSession()

	byStatus, err := facade.CountByStatus(ctx)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get session stats", errors.CodeDatabaseError)
	}

	byCluster, err := facade.CountByCluster(ctx)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get cluster stats", errors.CodeDatabaseError)
	}

	return &TraceLensSessionStatsResponse{
		ByStatus:  byStatus,
		ByCluster: byCluster,
	}, nil
}

func handleTraceLensSessionGet(ctx context.Context, req *TraceLensSessionGetRequest) (*TraceLensSessionResponse, error) {
	if req.SessionID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("session_id is required")
	}

	// Get Control Plane facade - sessions are centralized
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		return nil, errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("control plane not available")
	}
	facade := cpFacade.GetTraceLensSession()

	session, err := facade.GetBySessionID(ctx, req.SessionID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get session", errors.CodeDatabaseError)
	}

	if session == nil || session.ID == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("session not found")
	}

	return toTraceLensSessionResponse(session), nil
}

func handleTraceLensWorkloadSessions(ctx context.Context, req *TraceLensWorkloadSessionsRequest) (*TraceLensSessionsListResponse, error) {
	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}

	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		return nil, errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("control plane not available")
	}
	facade := cpFacade.GetTraceLensSession()

	sessions, err := facade.ListByWorkloadUID(ctx, req.WorkloadUID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list sessions", errors.CodeDatabaseError)
	}

	resp := &TraceLensSessionsListResponse{
		Sessions: make([]TraceLensSessionResponse, 0, len(sessions)),
		Total:    len(sessions),
	}
	for _, s := range sessions {
		resp.Sessions = append(resp.Sessions, *toTraceLensSessionResponse(s))
	}

	return resp, nil
}

// Helper function to check if status is active
func isActiveSessionStatus(status string) bool {
	for _, s := range cpmodel.ActiveStatuses() {
		if status == s {
			return true
		}
	}
	return false
}

// Helper function to convert session model to response
func toTraceLensSessionResponse(session *cpmodel.TracelensSessions) *TraceLensSessionResponse {
	resp := &TraceLensSessionResponse{
		SessionID:       session.SessionID,
		ClusterName:     session.ClusterName,
		WorkloadUID:     session.WorkloadUID,
		ProfilerFileID:  session.ProfilerFileID,
		Status:          session.Status,
		StatusMessage:   session.StatusMessage,
		PodName:         session.PodName,
		PodIP:           session.PodIP,
		ResourceProfile: session.ResourceProfile,
		CreatedAt:       session.CreatedAt,
		ExpiresAt:       session.ExpiresAt,
	}

	// Set UI path only if session is ready
	if session.Status == cpmodel.SessionStatusReady {
		resp.UIPath = fmt.Sprintf("/api/v1/tracelens/sessions/%s/ui/", session.SessionID)
	}

	// Set ready_at if not zero
	if !session.ReadyAt.IsZero() {
		resp.ReadyAt = &session.ReadyAt
	}

	// Set last_accessed_at if not zero
	if !session.LastAccessedAt.IsZero() {
		resp.LastAccessedAt = &session.LastAccessedAt
	}

	// Set estimated ready time for pending/creating sessions
	if session.Status == cpmodel.SessionStatusPending || session.Status == cpmodel.SessionStatusCreating {
		resp.EstimatedReady = 30 // estimated 30 seconds
	}

	return resp
}
