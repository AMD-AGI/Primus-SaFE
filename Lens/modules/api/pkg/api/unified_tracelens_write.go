// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/tracelens"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	cpmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
)

// ============================================================================
// TraceLens Session Write Endpoints
// Session metadata is stored in Control Plane database
// ============================================================================

// --- Create TraceLens Session ---

type CreateTraceLensSessionRequest struct {
	WorkloadUID     string `json:"workload_uid" mcp:"required,desc=The UID of the workload to analyze"`
	ProfilerFileID  int32  `json:"profiler_file_id" mcp:"required,desc=The ID of the profiler file to load"`
	TTLMinutes      int    `json:"ttl_minutes" mcp:"desc=Session TTL in minutes (default: 60)"`
	ResourceProfile string `json:"resource_profile" mcp:"desc=Resource profile: small/medium/large (default: medium)"`
	Cluster         string `json:"cluster" mcp:"desc=Cluster name (optional)"`
}

type CreateTraceLensSessionResponse struct {
	SessionID       string    `json:"session_id"`
	ClusterName     string    `json:"cluster_name"`
	WorkloadUID     string    `json:"workload_uid"`
	ProfilerFileID  int32     `json:"profiler_file_id"`
	Status          string    `json:"status"`
	ResourceProfile string    `json:"resource_profile"`
	ExpiresAt       time.Time `json:"expires_at"`
	EstimatedReady  int       `json:"estimated_ready_seconds,omitempty"`
}

func handleCreateTraceLensSession(ctx context.Context, req *CreateTraceLensSessionRequest) (*CreateTraceLensSessionResponse, error) {
	if req.WorkloadUID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workload_uid is required")
	}
	if req.ProfilerFileID <= 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("profiler_file_id is required")
	}

	// Get cluster name for profiler file reference
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}
	clusterName := clients.ClusterName

	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		return nil, errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("control plane not available")
	}
	facade := cpFacade.GetTraceLensSession()

	// Check for existing active session
	existing, err := facade.FindActiveSession(ctx, clusterName, req.WorkloadUID, req.ProfilerFileID)
	if err != nil {
		log.Warnf("Failed to check existing session: %v", err)
	}
	if existing != nil && existing.ID != 0 {
		// Extend TTL and return existing session
		ttl := req.TTLMinutes
		if ttl <= 0 {
			ttl = 60
		}
		existing.ExpiresAt = time.Now().Add(time.Duration(ttl) * time.Minute)
		if err := facade.Update(ctx, existing); err != nil {
			log.Warnf("Failed to extend session TTL: %v", err)
		}
		return &CreateTraceLensSessionResponse{
			SessionID:       existing.SessionID,
			ClusterName:     existing.ClusterName,
			WorkloadUID:     existing.WorkloadUID,
			ProfilerFileID:  existing.ProfilerFileID,
			Status:          existing.Status,
			ResourceProfile: existing.ResourceProfile,
			ExpiresAt:       existing.ExpiresAt,
		}, nil
	}

	// Generate session ID
	hash := sha256.Sum256([]byte(req.WorkloadUID))
	shortHash := fmt.Sprintf("%x", hash[:4])
	sessionID := fmt.Sprintf("tl-%s-%d-%d", shortHash, req.ProfilerFileID, time.Now().Unix())

	// Set defaults
	ttl := req.TTLMinutes
	if ttl <= 0 {
		ttl = 60
	}
	resourceProfile := req.ResourceProfile
	if resourceProfile == "" {
		resourceProfile = tlconst.ProfileMedium
	}
	if !tlconst.IsValidResourceProfile(resourceProfile) {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage(fmt.Sprintf("invalid resource_profile: %s", resourceProfile))
	}

	// Create session record in Control Plane database
	session := &cpmodel.TracelensSessions{
		SessionID:       sessionID,
		ClusterName:     clusterName,
		WorkloadUID:     req.WorkloadUID,
		ProfilerFileID:  req.ProfilerFileID,
		PodNamespace:    tlconst.DefaultPodNamespace,
		PodPort:         tlconst.DefaultPodPort,
		Status:          cpmodel.SessionStatusPending,
		ResourceProfile: resourceProfile,
		ExpiresAt:       time.Now().Add(time.Duration(ttl) * time.Minute),
	}

	if err := facade.Create(ctx, session); err != nil {
		return nil, errors.WrapError(err, "failed to create session", errors.CodeDatabaseError)
	}

	log.Infof("Created TraceLens session %s for workload %s, file %d (cluster: %s)", sessionID, req.WorkloadUID, req.ProfilerFileID, clusterName)

	// Trigger pod creation asynchronously
	tracelens.CreatePodAsync(ctx, clusterName, session, "")

	return &CreateTraceLensSessionResponse{
		SessionID:       session.SessionID,
		ClusterName:     session.ClusterName,
		WorkloadUID:     session.WorkloadUID,
		ProfilerFileID:  session.ProfilerFileID,
		Status:          session.Status,
		ResourceProfile: session.ResourceProfile,
		ExpiresAt:       session.ExpiresAt,
		EstimatedReady:  30,
	}, nil
}

// --- Extend TraceLens Session ---

type ExtendTraceLensSessionRequest struct {
	SessionID     string `json:"session_id" mcp:"required,desc=The session ID to extend"`
	ExtendMinutes int    `json:"extend_minutes" mcp:"required,desc=Minutes to extend (1-60)"`
	Cluster       string `json:"cluster" mcp:"desc=Cluster name (optional, ignored - sessions are in CP DB)"`
}

type ExtendTraceLensSessionResponse struct {
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Message   string    `json:"message"`
}

func handleExtendTraceLensSession(ctx context.Context, req *ExtendTraceLensSessionRequest) (*ExtendTraceLensSessionResponse, error) {
	if req.SessionID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("session_id is required")
	}
	if req.ExtendMinutes < 1 || req.ExtendMinutes > 60 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("extend_minutes must be between 1 and 60")
	}

	// Get Control Plane facade
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

	if session.Status == cpmodel.SessionStatusDeleted || session.Status == cpmodel.SessionStatusExpired {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("cannot extend deleted or expired session")
	}

	session.ExpiresAt = session.ExpiresAt.Add(time.Duration(req.ExtendMinutes) * time.Minute)
	if err := facade.Update(ctx, session); err != nil {
		return nil, errors.WrapError(err, "failed to extend session", errors.CodeDatabaseError)
	}

	return &ExtendTraceLensSessionResponse{
		SessionID: req.SessionID,
		ExpiresAt: session.ExpiresAt,
		Message:   fmt.Sprintf("Session extended by %d minutes", req.ExtendMinutes),
	}, nil
}

// --- Delete TraceLens Session ---

type DeleteTraceLensSessionRequest struct {
	SessionID string `json:"session_id" mcp:"required,desc=The session ID to delete"`
	Cluster   string `json:"cluster" mcp:"desc=Cluster name (optional, ignored - sessions are in CP DB)"`
}

type DeleteTraceLensSessionResponse struct {
	Message string `json:"message"`
}

func handleDeleteTraceLensSession(ctx context.Context, req *DeleteTraceLensSessionRequest) (*DeleteTraceLensSessionResponse, error) {
	if req.SessionID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("session_id is required")
	}

	// Get Control Plane facade
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

	// Delete pod asynchronously
	if session.PodName != "" && session.PodNamespace != "" {
		go tracelens.DeletePod(ctx, session.PodName, session.PodNamespace)
	}

	// Mark session as deleted
	if err := facade.UpdateStatus(ctx, req.SessionID, cpmodel.SessionStatusDeleted, "Deleted by user"); err != nil {
		log.Errorf("Failed to mark session as deleted: %v", err)
	}

	return &DeleteTraceLensSessionResponse{
		Message: "Session deleted successfully",
	}, nil
}

// ============================================================================
// Unified Registration
// ============================================================================

func init() {
	// Create TraceLens Session
	unified.Register(&unified.EndpointDef[CreateTraceLensSessionRequest, CreateTraceLensSessionResponse]{
		HTTPPath:    "/tracelens/sessions",
		HTTPMethod:  "POST",
		MCPToolName: "lens_create_tracelens_session",
		Description: "Create a new TraceLens analysis session for a profiler file",
		Handler:     handleCreateTraceLensSession,
	})

	// Extend TraceLens Session
	unified.Register(&unified.EndpointDef[ExtendTraceLensSessionRequest, ExtendTraceLensSessionResponse]{
		HTTPPath:    "/tracelens/sessions/:session_id",
		HTTPMethod:  "PATCH",
		MCPToolName: "lens_extend_tracelens_session",
		Description: "Extend the TTL of a TraceLens session",
		Handler:     handleExtendTraceLensSession,
	})

	// Delete TraceLens Session
	unified.Register(&unified.EndpointDef[DeleteTraceLensSessionRequest, DeleteTraceLensSessionResponse]{
		HTTPPath:    "/tracelens/sessions/:session_id",
		HTTPMethod:  "DELETE",
		MCPToolName: "lens_delete_tracelens_session",
		Description: "Delete a TraceLens session and its associated pod",
		Handler:     handleDeleteTraceLensSession,
	})
}
