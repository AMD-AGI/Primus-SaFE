// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	pftconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/perfetto"
)

// ============================================================================
// Perfetto Session GET Endpoint
// ============================================================================

const perfettoViewerType = "perfetto"

// PerfettoSessionResponse represents a Perfetto session response
type PerfettoSessionResponse struct {
	SessionID      string     `json:"session_id"`
	WorkloadUID    string     `json:"workload_uid"`
	ProfilerFileID int32      `json:"profiler_file_id"`
	Status         string     `json:"status"`
	StatusMessage  string     `json:"status_message,omitempty"`
	UIPath         string     `json:"ui_path,omitempty"`
	PodName        string     `json:"pod_name,omitempty"`
	PodIP          string     `json:"pod_ip,omitempty"`
	ViewerType     string     `json:"viewer_type"`
	CreatedAt      time.Time  `json:"created_at"`
	ReadyAt        *time.Time `json:"ready_at,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	EstimatedReady int        `json:"estimated_ready_seconds,omitempty"`
}

// --- Get Perfetto Session ---

type GetPerfettoSessionRequest struct {
	SessionID string `json:"session_id" mcp:"required,desc=The ID of the Perfetto session"`
	Cluster   string `json:"cluster" mcp:"desc=Cluster name (optional, uses default if not provided)"`
}

type GetPerfettoSessionResponse struct {
	Session *PerfettoSessionResponse `json:"session"`
}

func handleGetPerfettoSession(ctx context.Context, req *GetPerfettoSessionRequest) (*GetPerfettoSessionResponse, error) {
	if req.SessionID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("session_id is required")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
	session, err := facade.GetBySessionID(ctx, req.SessionID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get session", errors.CodeDatabaseError)
	}
	if session == nil || session.ID == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("session not found")
	}

	resp := &PerfettoSessionResponse{
		SessionID:      session.SessionID,
		WorkloadUID:    session.WorkloadUID,
		ProfilerFileID: session.ProfilerFileID,
		Status:         session.Status,
		StatusMessage:  session.StatusMessage,
		PodName:        session.PodName,
		PodIP:          session.PodIP,
		ViewerType:     perfettoViewerType,
		CreatedAt:      session.CreatedAt,
		ExpiresAt:      session.ExpiresAt,
	}

	// Set UI path if ready
	if session.Status == pftconst.StatusReady && session.PodIP != "" {
		resp.UIPath = fmt.Sprintf("/perfetto/sessions/%s/ui/", session.SessionID)
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
	if session.Status == pftconst.StatusPending || session.Status == pftconst.StatusCreating {
		resp.EstimatedReady = 15 // Perfetto pods are faster to start
	}

	return &GetPerfettoSessionResponse{Session: resp}, nil
}

// ============================================================================
// Unified Registration
// ============================================================================

func init() {
	// Get Perfetto session by ID
	unified.Register(&unified.EndpointDef[GetPerfettoSessionRequest, GetPerfettoSessionResponse]{
		HTTPPath:    "/perfetto/sessions/:session_id",
		HTTPMethod:  "GET",
		MCPToolName: "lens_get_perfetto_session",
		Description: "Get details of a Perfetto viewer session by session ID",
		Handler:     handleGetPerfettoSession,
	})

	// Note: POST/PATCH/DELETE/Proxy endpoints are NOT migrated
	// - POST /perfetto/sessions - CreateSession (write operation)
	// - PATCH /perfetto/sessions/:session_id - ExtendSession (write operation)
	// - DELETE /perfetto/sessions/:session_id - DeleteSession (write operation)
	// - Any /perfetto/sessions/:session_id/ui/* - ProxyUI (proxy)
}
