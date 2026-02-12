// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	cpmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
	"github.com/gin-gonic/gin"
)

// CreateSession creates a new TraceLens analysis session
// Session metadata is stored in Control Plane database
func CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get cluster name for profiler file lookup
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	// Use the resolved cluster name
	clusterName = clients.ClusterName

	// Get Control Plane facade for session management
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	// Check for existing active session (reuse if available)
	existing, err := facade.FindActiveSession(c, clusterName, req.WorkloadUID, req.ProfilerFileID)
	if err != nil {
		log.Warnf("Failed to check existing session: %v", err)
	}
	// Note: gorm may return empty struct with ID=0 instead of nil
	if existing != nil && existing.ID != 0 {
		// Extend TTL and return existing session
		ttl := req.TTLMinutes
		if ttl <= 0 {
			ttl = 60
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
		ttl = 60
	}
	resourceProfile := req.ResourceProfile
	if resourceProfile == "" {
		resourceProfile = tlconst.ProfileMedium
	}
	if !tlconst.IsValidResourceProfile(resourceProfile) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid resource_profile: %s", resourceProfile)})
		return
	}

	// Create session record in Control Plane database
	session := &cpmodel.TracelensSessions{
		SessionID:       sessionID,
		ClusterName:     clusterName, // Store cluster name for reference
		WorkloadUID:     req.WorkloadUID,
		ProfilerFileID:  req.ProfilerFileID,
		PodNamespace:    tlconst.DefaultPodNamespace,
		PodPort:         tlconst.DefaultPodPort,
		Status:          cpmodel.SessionStatusPending,
		ResourceProfile: resourceProfile,
		ExpiresAt:       time.Now().Add(time.Duration(ttl) * time.Minute),
	}

	if err := facade.Create(c, session); err != nil {
		log.Errorf("Failed to create session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	log.Infof("Created TraceLens session %s for workload %s, file %d (cluster: %s)", sessionID, req.WorkloadUID, req.ProfilerFileID, clusterName)

	// In the new architecture, profiler files are stored in database (profiler_file_content table)
	// file_path is optional and used only for legacy/backward compatibility
	// Trigger pod creation asynchronously - pod will fetch content via API
	CreatePodAsync(c, clusterName, session, "")

	c.JSON(http.StatusCreated, rest.SuccessResp(c, toSessionResponse(session)))
}

// GetSession retrieves a session by ID
// Session data is now in Control Plane database (cluster parameter is optional for backward compatibility)
func GetSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		log.Errorf("Failed to get session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	// Note: gorm may return empty struct with ID=0 instead of nil
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

	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		log.Errorf("Failed to get session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	// Note: gorm may return empty struct with ID=0 instead of nil
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Check if session is in a state that can be extended
	if session.Status == cpmodel.SessionStatusDeleted || session.Status == cpmodel.SessionStatusExpired {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot extend deleted or expired session"})
		return
	}

	session.ExpiresAt = session.ExpiresAt.Add(time.Duration(req.ExtendMinutes) * time.Minute)
	if err := facade.Update(c, session); err != nil {
		log.Errorf("Failed to update session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to extend session"})
		return
	}

	log.Infof("Extended session %s by %d minutes", sessionID, req.ExtendMinutes)
	c.JSON(http.StatusOK, rest.SuccessResp(c, toSessionResponse(session)))
}

// DeleteSession deletes a session and its associated pod
func DeleteSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		log.Errorf("Failed to get session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	// Note: gorm may return empty struct with ID=0 instead of nil
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Delete pod if exists (pods are in management cluster)
	if session.PodName != "" {
		podName := session.PodName
		podNamespace := session.PodNamespace
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := DeletePod(ctx, podName, podNamespace); err != nil {
				log.Warnf("Failed to delete pod %s: %v", podName, err)
			}
		}()
	}

	// Mark session as deleted
	if err := facade.Delete(c, sessionID); err != nil {
		log.Errorf("Failed to delete session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete session"})
		return
	}

	log.Infof("Deleted session %s", sessionID)
	c.JSON(http.StatusNoContent, nil)
}

// ListWorkloadSessions lists all sessions for a workload
func ListWorkloadSessions(c *gin.Context) {
	workloadUID := c.Param("workload_uid")

	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	sessions, err := facade.ListByWorkloadUID(c, workloadUID)
	if err != nil {
		log.Errorf("Failed to list sessions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	resp := ListSessionsResponse{
		Sessions: make([]SessionResponse, 0, len(sessions)),
		Total:    len(sessions),
	}
	for _, s := range sessions {
		resp.Sessions = append(resp.Sessions, *toSessionResponse(s))
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// ListActiveSessions lists all active sessions
// Now returns sessions from all clusters (centralized in CP DB)
func ListActiveSessions(c *gin.Context) {
	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	// Optional cluster filter
	clusterName := c.Query("cluster")
	var sessions []*cpmodel.TracelensSessions
	var err error

	if clusterName != "" {
		// Filter by cluster
		sessions, err = facade.ListByCluster(c, clusterName)
		if err == nil {
			// Further filter to only active ones
			activeSessions := make([]*cpmodel.TracelensSessions, 0)
			for _, s := range sessions {
				if isActiveStatus(s.Status) {
					activeSessions = append(activeSessions, s)
				}
			}
			sessions = activeSessions
		}
	} else {
		// All active sessions across all clusters
		sessions, err = facade.ListActive(c)
	}

	if err != nil {
		log.Errorf("Failed to list active sessions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	resp := ListSessionsResponse{
		Sessions: make([]SessionResponse, 0, len(sessions)),
		Total:    len(sessions),
	}
	for _, s := range sessions {
		resp.Sessions = append(resp.Sessions, *toSessionResponse(s))
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// GetSessionStats returns session statistics
// Now returns aggregated stats from Control Plane DB
func GetSessionStats(c *gin.Context) {
	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	counts, err := facade.CountByStatus(c)
	if err != nil {
		log.Errorf("Failed to get session stats: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get statistics"})
		return
	}

	// Also get per-cluster counts
	clusterCounts, err := facade.CountByCluster(c)
	if err != nil {
		log.Warnf("Failed to get cluster stats: %v", err)
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"by_status":  counts,
		"by_cluster": clusterCounts,
	}))
}

// Helper functions

func isActiveStatus(status string) bool {
	for _, s := range cpmodel.ActiveStatuses() {
		if status == s {
			return true
		}
	}
	return false
}

func getProfilerFilePath(c *gin.Context, clusterName string, profilerFileID int32) (string, error) {
	// Query profiler_files table from Data Plane database to get file path
	facade := database.GetFacadeForCluster(clusterName)
	db := facade.GetTraceLensSession().GetDB()

	var filePath string
	err := db.WithContext(c).
		Table("profiler_files").
		Select("file_path").
		Where("id = ?", profilerFileID).
		Scan(&filePath).Error

	if err != nil {
		return "", fmt.Errorf("failed to query profiler file: %w", err)
	}
	if filePath == "" {
		return "", fmt.Errorf("profiler file not found: %d", profilerFileID)
	}
	return filePath, nil
}

func generateSessionID(workloadUID string, fileID int32) string {
	prefix := workloadUID
	if len(workloadUID) > 8 {
		prefix = workloadUID[:8]
	}
	return fmt.Sprintf("%s-%s-%d-%d", tlconst.SessionIDPrefix, prefix, fileID, time.Now().Unix())
}

func toSessionResponse(session *cpmodel.TracelensSessions) *SessionResponse {
	resp := &SessionResponse{
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

// GetResourceProfiles returns the available resource profiles
func GetResourceProfiles(c *gin.Context) {
	profiles := make([]ResourceProfileResponse, 0, len(tlconst.ResourceProfiles))
	for _, p := range tlconst.ResourceProfiles {
		profiles = append(profiles, ResourceProfileResponse{
			Value:       p.Value,
			Label:       p.Label,
			Description: p.Description,
			Memory:      p.Memory,
			MemoryBytes: p.MemoryBytes,
			CPU:         p.CPU,
			IsDefault:   p.IsDefault,
		})
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, ResourceProfilesResponse{
		Profiles: profiles,
	}))
}
