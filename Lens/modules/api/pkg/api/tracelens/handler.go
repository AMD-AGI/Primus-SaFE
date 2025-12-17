package tracelens

import (
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
	"github.com/gin-gonic/gin"
)

// CreateSession creates a new TraceLens analysis session
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

	// Check for existing active session (reuse if available)
	existing, err := facade.FindActiveSession(c, req.WorkloadUID, req.ProfilerFileID)
	if err != nil {
		log.Warnf("Failed to check existing session: %v", err)
	}
	if existing != nil {
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

	// Get user info from context (if available from auth middleware)
	userID := c.GetString("user_id")
	userEmail := c.GetString("user_email")

	// Create session record
	session := &model.TracelensSessions{
		SessionID:       sessionID,
		WorkloadUID:     req.WorkloadUID,
		ProfilerFileID:  req.ProfilerFileID,
		UserID:          userID,
		UserEmail:       userEmail,
		PodNamespace:    tlconst.DefaultPodNamespace,
		PodPort:         tlconst.DefaultPodPort,
		Status:          tlconst.StatusPending,
		ResourceProfile: resourceProfile,
		ExpiresAt:       time.Now().Add(time.Duration(ttl) * time.Minute),
	}

	if err := facade.Create(c, session); err != nil {
		log.Errorf("Failed to create session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	log.Infof("Created TraceLens session %s for workload %s, file %d", sessionID, req.WorkloadUID, req.ProfilerFileID)

	// TODO: In Phase 3, trigger pod creation here
	// go podManager.CreatePod(context.Background(), clients.ClusterName, session)

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
	if session == nil {
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
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Check if session is in a state that can be extended
	if session.Status == tlconst.StatusDeleted || session.Status == tlconst.StatusExpired {
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
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// TODO: In Phase 3, delete pod here if exists
	// if session.PodName != "" {
	//     go podManager.DeletePod(context.Background(), session.PodName, session.PodNamespace)
	// }

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

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
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
func ListActiveSessions(c *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
	sessions, err := facade.ListActive(c)
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
func GetSessionStats(c *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
	counts, err := facade.CountByStatus(c)
	if err != nil {
		log.Errorf("Failed to get session stats: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get statistics"})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, counts))
}

// Helper functions

func generateSessionID(workloadUID string, fileID int32) string {
	prefix := workloadUID
	if len(workloadUID) > 8 {
		prefix = workloadUID[:8]
	}
	return fmt.Sprintf("%s-%s-%d-%d", tlconst.SessionIDPrefix, prefix, fileID, time.Now().Unix())
}

func toSessionResponse(session *model.TracelensSessions) *SessionResponse {
	resp := &SessionResponse{
		SessionID:       session.SessionID,
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
	if session.Status == tlconst.StatusReady {
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
	if session.Status == tlconst.StatusPending || session.Status == tlconst.StatusCreating {
		resp.EstimatedReady = 30 // estimated 30 seconds
	}

	return resp
}

