// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/perfetto"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	pftconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/perfetto"
)

// ============================================================================
// Perfetto Session Write Endpoints
// ============================================================================

// --- Create Perfetto Session ---

type CreatePerfettoSessionRequest struct {
	WorkloadUID    string `json:"workload_uid" mcp:"required,desc=The UID of the workload"`
	ProfilerFileID int32  `json:"profiler_file_id" mcp:"required,desc=The ID of the profiler file to visualize"`
	TTLMinutes     int    `json:"ttl_minutes" mcp:"desc=Session TTL in minutes (default: 30)"`
	Cluster        string `json:"cluster" mcp:"desc=Cluster name (optional)"`
}

type CreatePerfettoSessionResponse struct {
	SessionID      string    `json:"session_id"`
	WorkloadUID    string    `json:"workload_uid"`
	ProfilerFileID int32     `json:"profiler_file_id"`
	Status         string    `json:"status"`
	ViewerType     string    `json:"viewer_type"`
	ExpiresAt      time.Time `json:"expires_at"`
	EstimatedReady int       `json:"estimated_ready_seconds,omitempty"`
}

func handleCreatePerfettoSession(ctx context.Context, req *CreatePerfettoSessionRequest) (*CreatePerfettoSessionResponse, error) {
	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}
	if req.ProfilerFileID <= 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("profiler_file_id is required")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()

	// Check for existing active Perfetto session
	existing, err := facade.FindActiveSession(ctx, req.WorkloadUID, req.ProfilerFileID)
	if err != nil {
		log.Warnf("Failed to check existing session: %v", err)
	}
	if existing != nil && existing.ID != 0 && isPerfettoViewerSession(existing) {
		// Extend TTL and return existing session
		ttl := req.TTLMinutes
		if ttl <= 0 {
			ttl = 30
		}
		existing.ExpiresAt = time.Now().Add(time.Duration(ttl) * time.Minute)
		if err := facade.Update(ctx, existing); err != nil {
			log.Warnf("Failed to extend session TTL: %v", err)
		}
		return &CreatePerfettoSessionResponse{
			SessionID:      existing.SessionID,
			WorkloadUID:    existing.WorkloadUID,
			ProfilerFileID: existing.ProfilerFileID,
			Status:         existing.Status,
			ViewerType:     perfettoViewerType,
			ExpiresAt:      existing.ExpiresAt,
		}, nil
	}

	// Generate session ID
	hash := sha256.Sum256([]byte(req.WorkloadUID))
	shortHash := fmt.Sprintf("%x", hash[:4])
	sessionID := fmt.Sprintf("%s-%s-%d-%d", pftconst.SessionIDPrefix, shortHash, req.ProfilerFileID, time.Now().Unix())

	// Set defaults
	ttl := req.TTLMinutes
	if ttl <= 0 {
		ttl = 30
	}

	// Create session record with viewer_type in config
	config := model.ExtType{"viewer_type": perfettoViewerType}
	session := &model.TracelensSessions{
		SessionID:       sessionID,
		WorkloadUID:     req.WorkloadUID,
		ProfilerFileID:  req.ProfilerFileID,
		PodNamespace:    pftconst.DefaultPodNamespace,
		PodPort:         int32(pftconst.DefaultPodPort),
		Status:          pftconst.StatusPending,
		ResourceProfile: "perfetto",
		ExpiresAt:       time.Now().Add(time.Duration(ttl) * time.Minute),
		Config:          config,
	}

	if err := facade.Create(ctx, session); err != nil {
		return nil, errors.WrapError(err, "failed to create session", errors.CodeDatabaseError)
	}

	log.Infof("Created Perfetto session %s for workload %s, file %d", sessionID, req.WorkloadUID, req.ProfilerFileID)

	// Trigger pod creation asynchronously
	perfetto.CreatePodAsync(ctx, clients.ClusterName, session)

	return &CreatePerfettoSessionResponse{
		SessionID:      session.SessionID,
		WorkloadUID:    session.WorkloadUID,
		ProfilerFileID: session.ProfilerFileID,
		Status:         session.Status,
		ViewerType:     perfettoViewerType,
		ExpiresAt:      session.ExpiresAt,
		EstimatedReady: 15,
	}, nil
}

// --- Extend Perfetto Session ---

type ExtendPerfettoSessionRequest struct {
	SessionID     string `json:"session_id" mcp:"required,desc=The session ID to extend"`
	ExtendMinutes int    `json:"extend_minutes" mcp:"required,desc=Minutes to extend (1-60)"`
	Cluster       string `json:"cluster" mcp:"desc=Cluster name (optional)"`
}

type ExtendPerfettoSessionResponse struct {
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Message   string    `json:"message"`
}

func handleExtendPerfettoSession(ctx context.Context, req *ExtendPerfettoSessionRequest) (*ExtendPerfettoSessionResponse, error) {
	if req.SessionID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("session_id is required")
	}
	if req.ExtendMinutes < 1 || req.ExtendMinutes > 60 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("extend_minutes must be between 1 and 60")
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

	// Extend expiration with max limit
	newExpiry := session.ExpiresAt.Add(time.Duration(req.ExtendMinutes) * time.Minute)
	maxExpiry := session.CreatedAt.Add(pftconst.MaxSessionTTL)
	if newExpiry.After(maxExpiry) {
		newExpiry = maxExpiry
	}
	session.ExpiresAt = newExpiry

	if err := facade.Update(ctx, session); err != nil {
		return nil, errors.WrapError(err, "failed to extend session", errors.CodeDatabaseError)
	}

	return &ExtendPerfettoSessionResponse{
		SessionID: req.SessionID,
		ExpiresAt: session.ExpiresAt,
		Message:   fmt.Sprintf("Session extended by %d minutes", req.ExtendMinutes),
	}, nil
}

// --- Delete Perfetto Session ---

type DeletePerfettoSessionRequest struct {
	SessionID string `json:"session_id" mcp:"required,desc=The session ID to delete"`
	Cluster   string `json:"cluster" mcp:"desc=Cluster name (optional)"`
}

type DeletePerfettoSessionResponse struct {
	Message string `json:"message"`
}

func handleDeletePerfettoSession(ctx context.Context, req *DeletePerfettoSessionRequest) (*DeletePerfettoSessionResponse, error) {
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

	// Delete pod asynchronously
	go perfetto.DeletePod(ctx, session)

	// Mark session as deleted
	if err := facade.UpdateStatus(ctx, req.SessionID, pftconst.StatusDeleted, "Deleted by user"); err != nil {
		log.Errorf("Failed to mark session as deleted: %v", err)
	}

	return &DeletePerfettoSessionResponse{
		Message: "Session deleted successfully",
	}, nil
}

// Helper function
func isPerfettoViewerSession(session *model.TracelensSessions) bool {
	if session.Config == nil {
		return false
	}
	viewerType, ok := session.Config["viewer_type"]
	if !ok {
		return false
	}
	return viewerType == perfettoViewerType
}

// ============================================================================
// Unified Registration
// ============================================================================

func init() {
	// Create Perfetto Session
	unified.Register(&unified.EndpointDef[CreatePerfettoSessionRequest, CreatePerfettoSessionResponse]{
		HTTPPath:    "/perfetto/sessions",
		HTTPMethod:  "POST",
		MCPToolName: "lens_create_perfetto_session",
		Description: "Create a new Perfetto viewer session for a profiler file",
		Handler:     handleCreatePerfettoSession,
	})

	// Extend Perfetto Session
	unified.Register(&unified.EndpointDef[ExtendPerfettoSessionRequest, ExtendPerfettoSessionResponse]{
		HTTPPath:    "/perfetto/sessions/:session_id",
		HTTPMethod:  "PATCH",
		MCPToolName: "lens_extend_perfetto_session",
		Description: "Extend the TTL of a Perfetto session",
		Handler:     handleExtendPerfettoSession,
	})

	// Delete Perfetto Session
	unified.Register(&unified.EndpointDef[DeletePerfettoSessionRequest, DeletePerfettoSessionResponse]{
		HTTPPath:    "/perfetto/sessions/:session_id",
		HTTPMethod:  "DELETE",
		MCPToolName: "lens_delete_perfetto_session",
		Description: "Delete a Perfetto session and its associated pod",
		Handler:     handleDeletePerfettoSession,
	})
}
