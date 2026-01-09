// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package perfetto

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	pftconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/perfetto"
	"github.com/gin-gonic/gin"
)

const viewerTypePerfetto = "perfetto"

// CreateSession creates a new Perfetto viewer session
func CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get cluster
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()

	// Check for existing active Perfetto session (reuse if available)
	existing, err := facade.FindActiveSession(c, req.WorkloadUID, req.ProfilerFileID)
	if err != nil {
		log.Warnf("Failed to check existing session: %v", err)
	}
	// Check if it's a Perfetto session
	if existing != nil && existing.ID != 0 && isPerfettoSession(existing) {
		// Extend TTL and return existing session
		ttl := req.TTLMinutes
		if ttl <= 0 {
			ttl = 30
		}
		existing.ExpiresAt = time.Now().Add(time.Duration(ttl) * time.Minute)
		if err := facade.Update(c, existing); err != nil {
			log.Warnf("Failed to extend session TTL: %v", err)
		}
		c.JSON(http.StatusOK, rest.SuccessResp(c, toSessionResponse(existing)))
		return
	}

	// Generate session ID
	sessionID := generateSessionID(req.WorkloadUID, req.ProfilerFileID)

	// Set defaults
	ttl := req.TTLMinutes
	if ttl <= 0 {
		ttl = 30 // Perfetto sessions are shorter by default
	}

	// Create session record with viewer_type in config
	config := model.ExtType{"viewer_type": viewerTypePerfetto}
	session := &model.TracelensSessions{
		SessionID:       sessionID,
		WorkloadUID:     req.WorkloadUID,
		ProfilerFileID:  req.ProfilerFileID,
		PodNamespace:    pftconst.DefaultPodNamespace,
		PodPort:         int32(pftconst.DefaultPodPort),
		Status:          pftconst.StatusPending,
		ResourceProfile: "perfetto", // Mark as perfetto for resource allocation
		ExpiresAt:       time.Now().Add(time.Duration(ttl) * time.Minute),
		Config:          config,
	}

	if err := facade.Create(c, session); err != nil {
		log.Errorf("Failed to create session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	log.Infof("Created Perfetto session %s for workload %s, file %d", sessionID, req.WorkloadUID, req.ProfilerFileID)

	// Trigger pod creation asynchronously
	CreatePodAsync(c, clients.ClusterName, session)

	c.JSON(http.StatusCreated, rest.SuccessResp(c, toSessionResponse(session)))
}

// GetSession retrieves a session by ID
func GetSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		log.Errorf("Failed to get session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, toSessionResponse(session)))
}

// ExtendSession extends the TTL of a session
func ExtendSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	var req ExtendSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		log.Errorf("Failed to get session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Extend expiration
	newExpiry := session.ExpiresAt.Add(time.Duration(req.ExtendMinutes) * time.Minute)
	maxExpiry := session.CreatedAt.Add(pftconst.MaxSessionTTL)
	if newExpiry.After(maxExpiry) {
		newExpiry = maxExpiry
	}
	session.ExpiresAt = newExpiry

	if err := facade.Update(c, session); err != nil {
		log.Errorf("Failed to extend session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to extend session"})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, toSessionResponse(session)))
}

// DeleteSession deletes a session and its associated pod
func DeleteSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		log.Errorf("Failed to get session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Delete pod asynchronously
	DeletePodAsync(session)

	// Mark session as deleted
	if err := facade.UpdateStatus(c, sessionID, pftconst.StatusDeleted, "Deleted by user"); err != nil {
		log.Errorf("Failed to mark session as deleted: %v", err)
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"message": "session deleted"}))
}

// Helper functions

func generateSessionID(workloadUID string, profilerFileID int32) string {
	// Create a short hash from workload UID
	hash := sha256.Sum256([]byte(workloadUID))
	shortHash := fmt.Sprintf("%x", hash[:4])

	// Format: pft-{workload_hash}-{file_id}-{timestamp}
	return fmt.Sprintf("%s-%s-%d-%d", pftconst.SessionIDPrefix, shortHash, profilerFileID, time.Now().Unix())
}

func isPerfettoSession(session *model.TracelensSessions) bool {
	if session.Config == nil {
		return false
	}
	viewerType, ok := session.Config["viewer_type"]
	if !ok {
		return false
	}
	return viewerType == viewerTypePerfetto
}

func toSessionResponse(session *model.TracelensSessions) *SessionResponse {
	resp := &SessionResponse{
		SessionID:      session.SessionID,
		WorkloadUID:    session.WorkloadUID,
		ProfilerFileID: session.ProfilerFileID,
		Status:         session.Status,
		StatusMessage:  session.StatusMessage,
		PodName:        session.PodName,
		PodIP:          session.PodIP,
		ViewerType:     viewerTypePerfetto,
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

	return resp
}

