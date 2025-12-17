# TraceLens Integration - Implementation Plan

| Field | Value |
|-------|-------|
| **Created** | 2025-12-17 |
| **Total Estimated Time** | 16-20 hours |
| **Prerequisites** | SQL migration applied ✅, DAL generated ✅ |

---

## Overview

```
Phase 1: Data Layer (2h)
    ├── TraceLens Session Facade (database layer)
    ├── Multi-cluster support (WithCluster)
    └── Register in Facade interface

Phase 2: Session Manager (3h)
    ├── Service Interface (api layer)
    ├── Create/Get/Delete Logic
    └── Lifecycle Management

Phase 3: Kubernetes Integration (3h)
    ├── Pod Template Generator
    ├── Pod Watcher
    └── Status Synchronization

Phase 4: Proxy Layer (4h)
    ├── HTTP Reverse Proxy
    ├── WebSocket Proxy
    └── Path Rewriting

Phase 5: API Integration (2h)
    ├── Route Registration
    ├── Handler Implementation
    └── E2E Testing

Phase 6: Cleanup Job (1h)
    ├── Implement Job in jobs module
    ├── Register in initManagementJobs
    └── Test cleanup logic

Phase 7: Container Image (2h)
    ├── Dockerfile
    ├── Analysis Script
    └── CI/CD Integration
```

---

## Phase 1: Data Layer (Database Facade)

### 1.1 Constants (core module)

**File**: `core/pkg/tracelens/constants.go`

```go
package tracelens

import "time"

// Session Status
const (
    StatusPending      = "pending"
    StatusCreating     = "creating"
    StatusInitializing = "initializing"
    StatusReady        = "ready"
    StatusFailed       = "failed"
    StatusExpired      = "expired"
    StatusDeleted      = "deleted"
)

// Resource Profiles
const (
    ProfileSmall  = "small"   // 2GB RAM
    ProfileMedium = "medium"  // 4GB RAM
    ProfileLarge  = "large"   // 8GB RAM
)

// Default TTL
const DefaultSessionTTL = 1 * time.Hour

// Default namespace for TraceLens pods
const DefaultPodNamespace = "primus-lens"
```

### 1.2 TraceLens Session Facade Interface

**File**: `core/pkg/database/tracelens_session_facade.go`

```go
package database

import (
    "context"
    "time"

    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/dal"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
    "gorm.io/gorm"
)

// TraceLensSessionFacadeInterface defines the TraceLens Session Facade interface
type TraceLensSessionFacadeInterface interface {
    // GetDB returns the underlying GORM database connection
    GetDB() *gorm.DB
    
    // CRUD operations
    Create(ctx context.Context, session *model.TracelensSessions) error
    GetBySessionID(ctx context.Context, sessionID string) (*model.TracelensSessions, error)
    GetByID(ctx context.Context, id int32) (*model.TracelensSessions, error)
    Update(ctx context.Context, session *model.TracelensSessions) error
    Delete(ctx context.Context, sessionID string) error
    
    // Status management
    UpdateStatus(ctx context.Context, sessionID, status, message string) error
    UpdatePodInfo(ctx context.Context, sessionID, podName, podIP string, podPort int32) error
    UpdateLastAccessed(ctx context.Context, sessionID string) error
    MarkReady(ctx context.Context, sessionID, podIP string) error
    MarkFailed(ctx context.Context, sessionID, reason string) error
    
    // Query operations
    ListByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.TracelensSessions, error)
    ListByUserID(ctx context.Context, userID string) ([]*model.TracelensSessions, error)
    ListByStatus(ctx context.Context, status string) ([]*model.TracelensSessions, error)
    ListActive(ctx context.Context) ([]*model.TracelensSessions, error)
    ListExpired(ctx context.Context) ([]*model.TracelensSessions, error)
    CountByStatus(ctx context.Context) (map[string]int, error)
    
    // Find existing session for reuse
    FindActiveSession(ctx context.Context, workloadUID string, profilerFileID int32) (*model.TracelensSessions, error)
    
    // WithCluster returns a new facade instance for the specified cluster
    WithCluster(clusterName string) TraceLensSessionFacadeInterface
}

// TraceLensSessionFacade implements TraceLensSessionFacadeInterface
type TraceLensSessionFacade struct {
    BaseFacade
}

// NewTraceLensSessionFacade creates a new TraceLens Session Facade
func NewTraceLensSessionFacade() *TraceLensSessionFacade {
    return &TraceLensSessionFacade{}
}

// GetDB returns the underlying GORM database connection
func (f *TraceLensSessionFacade) GetDB() *gorm.DB {
    return f.getDB()
}

// Create creates a new session record
func (f *TraceLensSessionFacade) Create(ctx context.Context, session *model.TracelensSessions) error {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    return q.WithContext(ctx).Create(session)
}

// GetBySessionID retrieves a session by its session_id
func (f *TraceLensSessionFacade) GetBySessionID(ctx context.Context, sessionID string) (*model.TracelensSessions, error) {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    record, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).First()
    if err != nil {
        if err == gorm.ErrRecordNotFound {
            return nil, nil
        }
        return nil, err
    }
    return record, nil
}

// GetByID retrieves a session by its ID
func (f *TraceLensSessionFacade) GetByID(ctx context.Context, id int32) (*model.TracelensSessions, error) {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    record, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
    if err != nil {
        if err == gorm.ErrRecordNotFound {
            return nil, nil
        }
        return nil, err
    }
    return record, nil
}

// Update updates an existing session record
func (f *TraceLensSessionFacade) Update(ctx context.Context, session *model.TracelensSessions) error {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    _, err := q.WithContext(ctx).Where(q.ID.Eq(session.ID)).Updates(session)
    return err
}

// Delete soft-deletes a session by session_id
func (f *TraceLensSessionFacade) Delete(ctx context.Context, sessionID string) error {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    _, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Update(
        q.Status, "deleted",
    )
    return err
}

// UpdateStatus updates the status and message of a session
func (f *TraceLensSessionFacade) UpdateStatus(ctx context.Context, sessionID, status, message string) error {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    _, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
        "status":         status,
        "status_message": message,
    })
    return err
}

// UpdatePodInfo updates the pod information of a session
func (f *TraceLensSessionFacade) UpdatePodInfo(ctx context.Context, sessionID, podName, podIP string, podPort int32) error {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    _, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
        "pod_name": podName,
        "pod_ip":   podIP,
        "pod_port": podPort,
    })
    return err
}

// UpdateLastAccessed updates the last_accessed_at timestamp
func (f *TraceLensSessionFacade) UpdateLastAccessed(ctx context.Context, sessionID string) error {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    _, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Update(
        q.LastAccessedAt, time.Now(),
    )
    return err
}

// MarkReady marks a session as ready with pod IP
func (f *TraceLensSessionFacade) MarkReady(ctx context.Context, sessionID, podIP string) error {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    now := time.Now()
    _, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
        "status":   "ready",
        "pod_ip":   podIP,
        "ready_at": now,
    })
    return err
}

// MarkFailed marks a session as failed with reason
func (f *TraceLensSessionFacade) MarkFailed(ctx context.Context, sessionID, reason string) error {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    _, err := q.WithContext(ctx).Where(q.SessionID.Eq(sessionID)).Updates(map[string]interface{}{
        "status":         "failed",
        "status_message": reason,
    })
    return err
}

// ListByWorkloadUID lists sessions for a workload
func (f *TraceLensSessionFacade) ListByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.TracelensSessions, error) {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    return q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).Find()
}

// ListByUserID lists sessions for a user
func (f *TraceLensSessionFacade) ListByUserID(ctx context.Context, userID string) ([]*model.TracelensSessions, error) {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    return q.WithContext(ctx).Where(q.UserID.Eq(userID)).Find()
}

// ListByStatus lists sessions by status
func (f *TraceLensSessionFacade) ListByStatus(ctx context.Context, status string) ([]*model.TracelensSessions, error) {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    return q.WithContext(ctx).Where(q.Status.Eq(status)).Find()
}

// ListActive lists all active sessions (pending, creating, initializing, ready)
func (f *TraceLensSessionFacade) ListActive(ctx context.Context) ([]*model.TracelensSessions, error) {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    return q.WithContext(ctx).Where(
        q.Status.In("pending", "creating", "initializing", "ready"),
    ).Find()
}

// ListExpired lists sessions that have expired but not yet cleaned up
func (f *TraceLensSessionFacade) ListExpired(ctx context.Context) ([]*model.TracelensSessions, error) {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    now := time.Now()
    return q.WithContext(ctx).Where(
        q.ExpiresAt.Lt(now),
        q.Status.NotIn("deleted", "expired"),
    ).Find()
}

// CountByStatus returns a map of status to count
func (f *TraceLensSessionFacade) CountByStatus(ctx context.Context) (map[string]int, error) {
    db := f.getDB()
    
    var results []struct {
        Status string
        Count  int
    }
    
    err := db.WithContext(ctx).
        Table("tracelens_sessions").
        Select("status, count(*) as count").
        Group("status").
        Scan(&results).Error
    
    if err != nil {
        return nil, err
    }
    
    counts := make(map[string]int)
    for _, r := range results {
        counts[r.Status] = r.Count
    }
    return counts, nil
}

// FindActiveSession finds an existing active session for reuse
func (f *TraceLensSessionFacade) FindActiveSession(ctx context.Context, workloadUID string, profilerFileID int32) (*model.TracelensSessions, error) {
    db := f.getDB()
    q := dal.Use(db).TracelensSessions
    
    record, err := q.WithContext(ctx).Where(
        q.WorkloadUID.Eq(workloadUID),
        q.ProfilerFileID.Eq(profilerFileID),
        q.Status.Eq("ready"),
    ).First()
    
    if err != nil {
        if err == gorm.ErrRecordNotFound {
            return nil, nil
        }
        return nil, err
    }
    return record, nil
}

// WithCluster returns a new facade instance for the specified cluster
func (f *TraceLensSessionFacade) WithCluster(clusterName string) TraceLensSessionFacadeInterface {
    return &TraceLensSessionFacade{
        BaseFacade: f.withCluster(clusterName),
    }
}
```

### 1.3 Register in Facade Interface

**Update**: `core/pkg/database/facade.go`

```go
// Add to FacadeInterface
type FacadeInterface interface {
    // ... existing methods ...
    
    // GetTraceLensSession returns the TraceLensSession Facade interface
    GetTraceLensSession() TraceLensSessionFacadeInterface
}

// Add to Facade struct
type Facade struct {
    // ... existing fields ...
    TraceLensSession TraceLensSessionFacadeInterface
}

// Update NewFacade
func NewFacade() *Facade {
    return &Facade{
        // ... existing ...
        TraceLensSession: NewTraceLensSessionFacade(),
    }
}

// Add getter method
func (f *Facade) GetTraceLensSession() TraceLensSessionFacadeInterface {
    return f.TraceLensSession
}

// Update WithCluster
func (f *Facade) WithCluster(clusterName string) FacadeInterface {
    return &Facade{
        // ... existing ...
        TraceLensSession: f.TraceLensSession.WithCluster(clusterName),
    }
}
```

### 1.4 MCP Test Plan

```sql
-- Test 1: Insert a session
INSERT INTO tracelens_sessions (session_id, workload_uid, profiler_file_id, status)
VALUES ('tls-test-001', 'test-workload-uid', 1, 'pending');

-- Test 2: Query session
SELECT * FROM tracelens_sessions WHERE session_id = 'tls-test-001';

-- Test 3: Update status
UPDATE tracelens_sessions SET status = 'ready', ready_at = NOW() 
WHERE session_id = 'tls-test-001';

-- Test 4: List expired sessions
SELECT * FROM tracelens_sessions 
WHERE expires_at < NOW() AND status NOT IN ('deleted', 'expired');

-- Test 5: Cleanup
DELETE FROM tracelens_sessions WHERE session_id = 'tls-test-001';
```

**MCP Commands**:
```
mcp_postgres-x-flannel_insert_data -> tracelens_sessions
mcp_postgres-x-flannel_query_data -> SELECT * FROM tracelens_sessions
mcp_postgres-x-flannel_update_data -> status change
mcp_postgres-x-flannel_delete_data -> cleanup
```

---

## Phase 2: Session Manager Service (API Layer)

### 2.1 Types Definition

**File**: `api/pkg/tracelens/types.go`

```go
package tracelens

import "time"

// CreateSessionRequest is the request body for creating a session
type CreateSessionRequest struct {
    WorkloadUID     string `json:"workload_uid" binding:"required"`
    ProfilerFileID  int32  `json:"profiler_file_id" binding:"required"`
    TTLMinutes      int    `json:"ttl_minutes"`       // default: 60
    ResourceProfile string `json:"resource_profile"`  // default: medium
}

// SessionResponse is the response for session operations
type SessionResponse struct {
    SessionID       string    `json:"session_id"`
    Status          string    `json:"status"`
    StatusMessage   string    `json:"status_message,omitempty"`
    UIPath          string    `json:"ui_path"`
    CreatedAt       time.Time `json:"created_at"`
    ExpiresAt       time.Time `json:"expires_at"`
    ReadyAt         *time.Time `json:"ready_at,omitempty"`
    EstimatedReady  int       `json:"estimated_ready_seconds,omitempty"`
}

// ExtendSessionRequest is the request body for extending a session
type ExtendSessionRequest struct {
    ExtendMinutes int `json:"extend_minutes" binding:"required,min=1,max=120"`
}

// ListSessionsResponse is the response for listing sessions
type ListSessionsResponse struct {
    Sessions []SessionResponse `json:"sessions"`
    Total    int               `json:"total"`
}
```

### 2.2 Session Manager Interface

**File**: `api/pkg/tracelens/manager.go`

```go
package tracelens

import (
    "context"
    "fmt"
    "time"

    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
    tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
)

// SessionManager manages TraceLens sessions
type SessionManager struct {
    podManager *PodManager
}

// NewSessionManager creates a new session manager
func NewSessionManager(podManager *PodManager) *SessionManager {
    return &SessionManager{
        podManager: podManager,
    }
}

// CreateSession creates a new TraceLens session
func (m *SessionManager) CreateSession(ctx context.Context, clusterName string, req *CreateSessionRequest, userID, userEmail string) (*SessionResponse, error) {
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    
    // 1. Check for existing active session (reuse if available)
    existing, err := facade.FindActiveSession(ctx, req.WorkloadUID, req.ProfilerFileID)
    if err != nil {
        return nil, fmt.Errorf("failed to check existing session: %w", err)
    }
    if existing != nil {
        // Extend TTL and return existing session
        existing.ExpiresAt = time.Now().Add(time.Duration(req.TTLMinutes) * time.Minute)
        facade.Update(ctx, existing)
        return m.toResponse(existing), nil
    }
    
    // 2. Generate session ID
    sessionID := generateSessionID(req.WorkloadUID, req.ProfilerFileID)
    
    // 3. Set defaults
    ttl := req.TTLMinutes
    if ttl <= 0 {
        ttl = 60 // default 60 minutes
    }
    resourceProfile := req.ResourceProfile
    if resourceProfile == "" {
        resourceProfile = tlconst.ProfileMedium
    }
    
    // 4. Create session record
    session := &model.TracelensSessions{
        SessionID:       sessionID,
        WorkloadUID:     req.WorkloadUID,
        ProfilerFileID:  req.ProfilerFileID,
        UserID:          userID,
        UserEmail:       userEmail,
        PodNamespace:    tlconst.DefaultPodNamespace,
        Status:          tlconst.StatusPending,
        ResourceProfile: resourceProfile,
        ExpiresAt:       time.Now().Add(time.Duration(ttl) * time.Minute),
    }
    
    if err := facade.Create(ctx, session); err != nil {
        return nil, fmt.Errorf("failed to create session: %w", err)
    }
    
    // 5. Trigger pod creation (async)
    go m.podManager.CreatePod(context.Background(), clusterName, session)
    
    return m.toResponse(session), nil
}

// GetSession retrieves a session by ID
func (m *SessionManager) GetSession(ctx context.Context, clusterName, sessionID string) (*SessionResponse, error) {
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    
    session, err := facade.GetBySessionID(ctx, sessionID)
    if err != nil {
        return nil, err
    }
    if session == nil {
        return nil, fmt.Errorf("session not found")
    }
    
    return m.toResponse(session), nil
}

// DeleteSession deletes a session and its pod
func (m *SessionManager) DeleteSession(ctx context.Context, clusterName, sessionID string) error {
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    
    session, err := facade.GetBySessionID(ctx, sessionID)
    if err != nil {
        return err
    }
    if session == nil {
        return fmt.Errorf("session not found")
    }
    
    // Delete pod if exists
    if session.PodName != "" {
        go m.podManager.DeletePod(context.Background(), session.PodName, session.PodNamespace)
    }
    
    // Mark session as deleted
    return facade.Delete(ctx, sessionID)
}

// ExtendSession extends the TTL of a session
func (m *SessionManager) ExtendSession(ctx context.Context, clusterName, sessionID string, extendMinutes int) (*SessionResponse, error) {
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    
    session, err := facade.GetBySessionID(ctx, sessionID)
    if err != nil {
        return nil, err
    }
    if session == nil {
        return nil, fmt.Errorf("session not found")
    }
    
    session.ExpiresAt = session.ExpiresAt.Add(time.Duration(extendMinutes) * time.Minute)
    if err := facade.Update(ctx, session); err != nil {
        return nil, err
    }
    
    return m.toResponse(session), nil
}

// ListWorkloadSessions lists sessions for a workload
func (m *SessionManager) ListWorkloadSessions(ctx context.Context, clusterName, workloadUID string) (*ListSessionsResponse, error) {
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    
    sessions, err := facade.ListByWorkloadUID(ctx, workloadUID)
    if err != nil {
        return nil, err
    }
    
    resp := &ListSessionsResponse{
        Sessions: make([]SessionResponse, 0, len(sessions)),
        Total:    len(sessions),
    }
    for _, s := range sessions {
        resp.Sessions = append(resp.Sessions, *m.toResponse(s))
    }
    return resp, nil
}

// MarkReady marks a session as ready (called by pod watcher)
func (m *SessionManager) MarkReady(ctx context.Context, clusterName, sessionID, podIP string) error {
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    return facade.MarkReady(ctx, sessionID, podIP)
}

// MarkFailed marks a session as failed (called by pod watcher)
func (m *SessionManager) MarkFailed(ctx context.Context, clusterName, sessionID, reason string) error {
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    return facade.MarkFailed(ctx, sessionID, reason)
}

func (m *SessionManager) toResponse(session *model.TracelensSessions) *SessionResponse {
    resp := &SessionResponse{
        SessionID:     session.SessionID,
        Status:        session.Status,
        StatusMessage: session.StatusMessage,
        UIPath:        fmt.Sprintf("/api/v1/tracelens/sessions/%s/ui/", session.SessionID),
        CreatedAt:     session.CreatedAt,
        ExpiresAt:     session.ExpiresAt,
    }
    
    if !session.ReadyAt.IsZero() {
        resp.ReadyAt = &session.ReadyAt
    }
    
    if session.Status == tlconst.StatusPending || session.Status == tlconst.StatusCreating {
        resp.EstimatedReady = 30 // estimated 30 seconds
    }
    
    return resp
}

func generateSessionID(workloadUID string, fileID int32) string {
    prefix := workloadUID
    if len(workloadUID) > 8 {
        prefix = workloadUID[:8]
    }
    return fmt.Sprintf("tls-%s-%d-%d", prefix, fileID, time.Now().Unix())
}
```

### 2.3 MCP Test Plan

```sql
-- Test session creation flow
-- 1. Check profiler file exists
SELECT id, workload_uid, file_name FROM profiler_files LIMIT 1;

-- 2. Create session
INSERT INTO tracelens_sessions (
    session_id, workload_uid, profiler_file_id, 
    status, expires_at, resource_profile
) VALUES (
    'tls-test-flow-001', 
    '0512be05-0c7f-4a39-b577-624b13c8533f',
    369,
    'pending',
    NOW() + INTERVAL '1 hour',
    'medium'
);

-- 3. Simulate pod creation -> status update
UPDATE tracelens_sessions 
SET status = 'creating', pod_name = 'tracelens-session-tls-test-flow-001'
WHERE session_id = 'tls-test-flow-001';

-- 4. Simulate pod ready
UPDATE tracelens_sessions 
SET status = 'ready', pod_ip = '172.16.57.100', ready_at = NOW()
WHERE session_id = 'tls-test-flow-001';

-- 5. Verify
SELECT * FROM tracelens_sessions WHERE session_id = 'tls-test-flow-001';

-- 6. Cleanup
DELETE FROM tracelens_sessions WHERE session_id LIKE 'tls-test%';
```

---

## Phase 3: Kubernetes Pod Management

### 3.1 Pod Manager

**File**: `api/pkg/tracelens/pod_manager.go`

```go
package tracelens

import (
    "context"
    "fmt"

    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
    tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/watch"
    "k8s.io/client-go/kubernetes"
)

type PodManagerConfig struct {
    TraceLensImage    string
    ProfilerStorePath string
}

type PodManager struct {
    config         *PodManagerConfig
    sessionManager *SessionManager // Set after creation to avoid circular dependency
}

func NewPodManager(config *PodManagerConfig) *PodManager {
    return &PodManager{config: config}
}

func (m *PodManager) SetSessionManager(sm *SessionManager) {
    m.sessionManager = sm
}

// CreatePod creates a TraceLens pod for the session
func (m *PodManager) CreatePod(ctx context.Context, clusterName string, session *model.TracelensSessions) error {
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    
    // Update status to creating
    if err := facade.UpdateStatus(ctx, session.SessionID, tlconst.StatusCreating, "Creating pod"); err != nil {
        return err
    }
    
    // Get kubernetes client for the cluster
    cm := clientsets.GetClusterManager()
    clientSet, err := cm.GetClientSetByClusterName(clusterName)
    if err != nil {
        facade.MarkFailed(ctx, session.SessionID, fmt.Sprintf("Failed to get k8s client: %v", err))
        return err
    }
    
    // Build pod spec
    pod := m.buildPodSpec(session)
    
    // Create pod
    _, err = clientSet.K8S.CoreV1().Pods(session.PodNamespace).Create(ctx, pod, metav1.CreateOptions{})
    if err != nil {
        facade.MarkFailed(ctx, session.SessionID, fmt.Sprintf("Failed to create pod: %v", err))
        return err
    }
    
    // Update pod name in session
    if err := facade.UpdatePodInfo(ctx, session.SessionID, pod.Name, "", session.PodPort); err != nil {
        return err
    }
    
    log.Infof("Created TraceLens pod %s for session %s", pod.Name, session.SessionID)
    return nil
}

// DeletePod deletes a TraceLens pod
func (m *PodManager) DeletePod(ctx context.Context, podName, namespace string) error {
    cm := clientsets.GetClusterManager()
    clientSet := cm.GetCurrentClusterClients()
    
    err := clientSet.K8S.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
    if err != nil {
        log.Errorf("Failed to delete pod %s: %v", podName, err)
        return err
    }
    
    log.Infof("Deleted TraceLens pod %s", podName)
    return nil
}

func (m *PodManager) buildPodSpec(session *model.TracelensSessions) *corev1.Pod {
    podName := fmt.Sprintf("tracelens-session-%s", session.SessionID)
    baseURLPath := fmt.Sprintf("/api/v1/tracelens/sessions/%s/ui", session.SessionID)
    
    resources := m.getResourceRequirements(session.ResourceProfile)
    
    return &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      podName,
            Namespace: session.PodNamespace,
            Labels: map[string]string{
                "app":        "tracelens-session",
                "session-id": session.SessionID,
                "managed-by": "lens-api",
            },
        },
        Spec: corev1.PodSpec{
            RestartPolicy: corev1.RestartPolicyNever,
            Containers: []corev1.Container{{
                Name:  "tracelens",
                Image: m.config.TraceLensImage,
                Env: []corev1.EnvVar{
                    {Name: "SESSION_ID", Value: session.SessionID},
                    {Name: "PROFILER_FILE_ID", Value: fmt.Sprintf("%d", session.ProfilerFileID)},
                    {Name: "BASE_URL_PATH", Value: baseURLPath},
                },
                Ports: []corev1.ContainerPort{{
                    Name:          "http",
                    ContainerPort: 8501,
                }},
                Resources: resources,
                ReadinessProbe: &corev1.Probe{
                    ProbeHandler: corev1.ProbeHandler{
                        HTTPGet: &corev1.HTTPGetAction{
                            Path: "/_stcore/health",
                            Port: intstr.FromInt(8501),
                        },
                    },
                    InitialDelaySeconds: 10,
                    PeriodSeconds:       5,
                },
            }},
        },
    }
}

func (m *PodManager) getResourceRequirements(profile string) corev1.ResourceRequirements {
    switch profile {
    case tlconst.ProfileSmall:
        return corev1.ResourceRequirements{
            Requests: corev1.ResourceList{
                corev1.ResourceCPU:    resource.MustParse("500m"),
                corev1.ResourceMemory: resource.MustParse("1Gi"),
            },
            Limits: corev1.ResourceList{
                corev1.ResourceCPU:    resource.MustParse("1"),
                corev1.ResourceMemory: resource.MustParse("2Gi"),
            },
        }
    case tlconst.ProfileLarge:
        return corev1.ResourceRequirements{
            Requests: corev1.ResourceList{
                corev1.ResourceCPU:    resource.MustParse("2"),
                corev1.ResourceMemory: resource.MustParse("4Gi"),
            },
            Limits: corev1.ResourceList{
                corev1.ResourceCPU:    resource.MustParse("4"),
                corev1.ResourceMemory: resource.MustParse("8Gi"),
            },
        }
    default: // ProfileMedium
        return corev1.ResourceRequirements{
            Requests: corev1.ResourceList{
                corev1.ResourceCPU:    resource.MustParse("1"),
                corev1.ResourceMemory: resource.MustParse("2Gi"),
            },
            Limits: corev1.ResourceList{
                corev1.ResourceCPU:    resource.MustParse("2"),
                corev1.ResourceMemory: resource.MustParse("4Gi"),
            },
        }
    }
}

// StartWatcher starts watching for TraceLens pod events
func (m *PodManager) StartWatcher(ctx context.Context, clusterName string) error {
    cm := clientsets.GetClusterManager()
    clientSet, err := cm.GetClientSetByClusterName(clusterName)
    if err != nil {
        return err
    }
    
    go m.watchPods(ctx, clusterName, clientSet.K8S)
    return nil
}

func (m *PodManager) watchPods(ctx context.Context, clusterName string, clientset kubernetes.Interface) {
    for {
        watcher, err := clientset.CoreV1().Pods(tlconst.DefaultPodNamespace).Watch(ctx, metav1.ListOptions{
            LabelSelector: "app=tracelens-session",
        })
        if err != nil {
            log.Errorf("Failed to watch pods: %v", err)
            continue
        }
        
        for event := range watcher.ResultChan() {
            pod, ok := event.Object.(*corev1.Pod)
            if !ok {
                continue
            }
            
            sessionID := pod.Labels["session-id"]
            if sessionID == "" {
                continue
            }
            
            switch event.Type {
            case watch.Modified:
                if pod.Status.Phase == corev1.PodRunning && isPodReady(pod) {
                    m.sessionManager.MarkReady(ctx, clusterName, sessionID, pod.Status.PodIP)
                } else if pod.Status.Phase == corev1.PodFailed {
                    reason := getPodFailReason(pod)
                    m.sessionManager.MarkFailed(ctx, clusterName, sessionID, reason)
                }
            case watch.Deleted:
                facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
                facade.UpdateStatus(ctx, sessionID, tlconst.StatusDeleted, "Pod deleted")
            }
        }
        
        // Watcher closed, restart after a delay
        select {
        case <-ctx.Done():
            return
        default:
            continue
        }
    }
}

func isPodReady(pod *corev1.Pod) bool {
    for _, cond := range pod.Status.Conditions {
        if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
            return true
        }
    }
    return false
}

func getPodFailReason(pod *corev1.Pod) string {
    for _, cs := range pod.Status.ContainerStatuses {
        if cs.State.Terminated != nil {
            return cs.State.Terminated.Reason
        }
        if cs.State.Waiting != nil {
            return cs.State.Waiting.Reason
        }
    }
    return "Unknown"
}
```

### 3.2 MCP/Kubectl Test Plan

```bash
# Test 1: Create test pod manually
kubectl --context=x-flannel -n primus-lens apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: tracelens-test-pod
  labels:
    app: tracelens-session
    session-id: test-session
spec:
  restartPolicy: Never
  containers:
  - name: tracelens
    image: python:3.10-slim
    command: ["sleep", "300"]
EOF

# Test 2: Check pod status
kubectl --context=x-flannel -n primus-lens get pod tracelens-test-pod -o wide

# Test 3: Delete pod
kubectl --context=x-flannel -n primus-lens delete pod tracelens-test-pod
```

**MCP Kubernetes Commands**:
```
mcp_kubernetes_kubectl_apply -> create test pod
mcp_kubernetes_kubectl_get -> check status
mcp_kubernetes_kubectl_delete -> cleanup
```

---

## Phase 4: HTTP/WebSocket Proxy

### 4.1 Proxy Handler

**File**: `api/pkg/tracelens/proxy.go`

```go
package tracelens

import (
    "fmt"
    "net/http"
    "net/http/httputil"
    "net/url"

    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
    tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
    "github.com/gin-gonic/gin"
    "github.com/gorilla/websocket"
)

type ProxyHandler struct{}

func NewProxyHandler() *ProxyHandler {
    return &ProxyHandler{}
}

func (h *ProxyHandler) ServeHTTP(c *gin.Context) {
    sessionID := c.Param("session_id")
    
    // Get cluster from context
    cm := clientsets.GetClusterManager()
    clusterName := cm.GetCurrentClusterName()
    
    // Get session
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    session, err := facade.GetBySessionID(c, sessionID)
    if err != nil || session == nil {
        c.JSON(404, gin.H{"error": "Session not found"})
        return
    }
    
    if session.Status != tlconst.StatusReady {
        c.JSON(503, gin.H{
            "error":  "Session not ready",
            "status": session.Status,
        })
        return
    }
    
    // Update last accessed
    facade.UpdateLastAccessed(c, sessionID)
    
    // Check WebSocket upgrade
    if isWebSocketUpgrade(c.Request) {
        h.proxyWebSocket(c, session)
        return
    }
    
    // Proxy HTTP
    h.proxyHTTP(c, session)
}

func (h *ProxyHandler) proxyHTTP(c *gin.Context, session *model.TracelensSessions) {
    targetURL := fmt.Sprintf("http://%s:%d", session.PodIP, session.PodPort)
    target, _ := url.Parse(targetURL)
    
    proxy := httputil.NewSingleHostReverseProxy(target)
    proxy.ServeHTTP(c.Writer, c.Request)
}

func (h *ProxyHandler) proxyWebSocket(c *gin.Context, session *model.TracelensSessions) {
    upgrader := websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool { return true },
    }
    
    clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        return
    }
    defer clientConn.Close()
    
    backendURL := fmt.Sprintf("ws://%s:%d%s", session.PodIP, session.PodPort, c.Request.URL.Path)
    backendConn, _, err := websocket.DefaultDialer.Dial(backendURL, nil)
    if err != nil {
        return
    }
    defer backendConn.Close()
    
    errChan := make(chan error, 2)
    go copyWS(clientConn, backendConn, errChan)
    go copyWS(backendConn, clientConn, errChan)
    <-errChan
}

func isWebSocketUpgrade(r *http.Request) bool {
    return r.Header.Get("Upgrade") == "websocket"
}

func copyWS(dst, src *websocket.Conn, errChan chan error) {
    for {
        msgType, msg, err := src.ReadMessage()
        if err != nil {
            errChan <- err
            return
        }
        if err := dst.WriteMessage(msgType, msg); err != nil {
            errChan <- err
            return
        }
    }
}
```

---

## Phase 5: API Routes Integration

### 5.1 Route Registration

**File**: `api/pkg/tracelens/routes.go`

```go
package tracelens

import (
    "net/http"

    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
    "github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, manager *SessionManager, proxy *ProxyHandler) {
    tl := router.Group("/tracelens")
    {
        sessions := tl.Group("/sessions")
        {
            sessions.POST("", createSessionHandler(manager))
            sessions.GET("/:session_id", getSessionHandler(manager))
            sessions.PATCH("/:session_id", extendSessionHandler(manager))
            sessions.DELETE("/:session_id", deleteSessionHandler(manager))
            
            // Proxy - catch all paths under /ui/
            sessions.Any("/:session_id/ui/*path", proxy.ServeHTTP)
        }
    }
}

func createSessionHandler(manager *SessionManager) gin.HandlerFunc {
    return func(c *gin.Context) {
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
        
        // Get user info (from auth middleware)
        userID := c.GetString("user_id")
        userEmail := c.GetString("user_email")
        
        resp, err := manager.CreateSession(c, clients.ClusterName, &req, userID, userEmail)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(http.StatusCreated, rest.SuccessResp(c, resp))
    }
}

func getSessionHandler(manager *SessionManager) gin.HandlerFunc {
    return func(c *gin.Context) {
        sessionID := c.Param("session_id")
        
        cm := clientsets.GetClusterManager()
        clusterName := c.Query("cluster")
        clients, err := cm.GetClusterClientsOrDefault(clusterName)
        if err != nil {
            _ = c.Error(err)
            return
        }
        
        resp, err := manager.GetSession(c, clients.ClusterName, sessionID)
        if err != nil {
            c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
    }
}

func extendSessionHandler(manager *SessionManager) gin.HandlerFunc {
    return func(c *gin.Context) {
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
        
        resp, err := manager.ExtendSession(c, clients.ClusterName, sessionID, req.ExtendMinutes)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
    }
}

func deleteSessionHandler(manager *SessionManager) gin.HandlerFunc {
    return func(c *gin.Context) {
        sessionID := c.Param("session_id")
        
        cm := clientsets.GetClusterManager()
        clusterName := c.Query("cluster")
        clients, err := cm.GetClusterClientsOrDefault(clusterName)
        if err != nil {
            _ = c.Error(err)
            return
        }
        
        err = manager.DeleteSession(c, clients.ClusterName, sessionID)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(http.StatusNoContent, nil)
    }
}
```

---

## Phase 6: Cleanup Job (jobs module)

### 6.1 TraceLens Session Cleanup Job

**File**: `jobs/pkg/jobs/tracelens_cleanup/cleanup.go`

```go
package tracelens_cleanup

import (
    "context"
    "fmt"
    "time"

    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
    "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
    tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
    "github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TraceLensCleanupJob cleans up expired TraceLens sessions
type TraceLensCleanupJob struct{}

func NewTraceLensCleanupJob() *TraceLensCleanupJob {
    return &TraceLensCleanupJob{}
}

func (j *TraceLensCleanupJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
    stats := common.NewExecutionStats()
    
    clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
    facade := database.GetFacadeForCluster(clusterName).GetTraceLensSession()
    
    // Get expired sessions
    expiredSessions, err := facade.ListExpired(ctx)
    if err != nil {
        return stats, fmt.Errorf("failed to list expired sessions: %w", err)
    }
    
    if len(expiredSessions) == 0 {
        stats.AddMessage("No expired sessions found")
        return stats, nil
    }
    
    log.Infof("Found %d expired TraceLens sessions to cleanup", len(expiredSessions))
    
    for _, session := range expiredSessions {
        // Delete pod if exists
        if session.PodName != "" {
            err := clientSets.K8S.CoreV1().Pods(session.PodNamespace).Delete(
                ctx,
                session.PodName,
                metav1.DeleteOptions{},
            )
            if err != nil {
                log.Warnf("Failed to delete pod %s: %v", session.PodName, err)
                stats.ErrorCount++
            } else {
                log.Infof("Deleted expired pod %s", session.PodName)
            }
        }
        
        // Mark session as expired
        err := facade.UpdateStatus(ctx, session.SessionID, tlconst.StatusExpired, "Session expired")
        if err != nil {
            log.Errorf("Failed to update session status: %v", err)
            stats.ErrorCount++
        } else {
            stats.ItemsCreated++ // reusing for items processed
        }
    }
    
    stats.RecordsProcessed = int64(len(expiredSessions))
    stats.AddMessage(fmt.Sprintf("Cleaned up %d expired sessions", len(expiredSessions)))
    
    return stats, nil
}

func (j *TraceLensCleanupJob) Schedule() string {
    return "@every 5m" // Run every 5 minutes
}
```

### 6.2 Register in initManagementJobs

**Update**: `jobs/pkg/jobs/interface.go`

```go
import (
    // ... existing imports ...
    "github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/tracelens_cleanup"
)

// initManagementJobs initializes all management jobs
func initManagementJobs(cfg *config.JobsConfig) []Job {
    var jobs []Job

    // Add TraceLens cleanup job
    jobs = append(jobs, tracelens_cleanup.NewTraceLensCleanupJob())
    log.Info("TraceLens cleanup job registered")

    // Add weekly report job if configured
    if cfg != nil && cfg.WeeklyReport != nil && cfg.WeeklyReport.Enabled {
        jobs = append(jobs, gpu_usage_weekly_report.NewGpuUsageWeeklyReportJob(cfg.WeeklyReport))
        log.Info("Weekly report job registered")
    }

    return jobs
}
```

### 6.3 MCP Test Plan

```sql
-- Create expired test session
INSERT INTO tracelens_sessions (
    session_id, workload_uid, profiler_file_id,
    status, expires_at, pod_name
) VALUES (
    'tls-expired-test',
    'test-workload',
    1,
    'ready',
    NOW() - INTERVAL '1 hour',
    'tracelens-session-tls-expired-test'
);

-- Verify it appears in expired list
SELECT * FROM tracelens_sessions 
WHERE expires_at < NOW() AND status NOT IN ('deleted', 'expired');

-- After job runs, verify status updated
SELECT * FROM tracelens_sessions WHERE session_id = 'tls-expired-test';
```

---

## Phase 7: TraceLens Container Image

### 7.1 Dockerfile

**File**: `Lens/docker/tracelens/Dockerfile`

```dockerfile
FROM python:3.10-slim

LABEL maintainer="AMD AGI Team"

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1000 tracelens
WORKDIR /app

RUN pip install --no-cache-dir \
    git+https://github.com/AMD-AGI/TraceLens.git \
    streamlit openpyxl plotly requests

COPY analyze_trace.py /app/

RUN chown -R tracelens:tracelens /app
USER tracelens

EXPOSE 8501

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s \
    CMD curl -f http://localhost:8501/_stcore/health || exit 1

CMD ["streamlit", "run", "/app/analyze_trace.py", "--server.port=8501"]
```

### 7.2 Analysis Script

**File**: `Lens/docker/tracelens/analyze_trace.py`

```python
import os
import streamlit as st
import requests
from io import BytesIO

# Get configuration from environment
SESSION_ID = os.getenv("SESSION_ID", "unknown")
PROFILER_FILE_ID = os.getenv("PROFILER_FILE_ID")
BASE_URL_PATH = os.getenv("BASE_URL_PATH", "")
API_BASE_URL = os.getenv("API_BASE_URL", "http://lens-api:8080")

st.set_page_config(
    page_title=f"TraceLens - {SESSION_ID}",
    layout="wide"
)

@st.cache_data
def load_trace_file(file_id):
    """Load trace file from API"""
    response = requests.get(f"{API_BASE_URL}/api/v1/profiler/files/{file_id}/content")
    response.raise_for_status()
    return BytesIO(response.content)

def main():
    st.title("TraceLens Trace Analyzer")
    st.write(f"Session: `{SESSION_ID}`")
    
    if PROFILER_FILE_ID:
        try:
            trace_data = load_trace_file(PROFILER_FILE_ID)
            # Use TraceLens to analyze
            from TraceLens.UI.trace_analyser import analyze_trace
            analyze_trace(trace_data)
        except Exception as e:
            st.error(f"Failed to load trace file: {e}")
    else:
        st.warning("No profiler file ID provided")

if __name__ == "__main__":
    main()
```

### 7.3 CI/CD Integration

**Add to**: `.github/workflows/lens-build.yml`

```yaml
  build-tracelens:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    - name: Build TraceLens image
      run: |
        docker build -t $HARBOR_REGISTRY/primussafe/tracelens:$VERSION \
          -f Lens/docker/tracelens/Dockerfile \
          Lens/docker/tracelens/
    - name: Push image
      run: docker push $HARBOR_REGISTRY/primussafe/tracelens:$VERSION
```

---

## File Structure Summary

```
core/pkg/
├── tracelens/
│   └── constants.go              # Status, Profile constants
└── database/
    ├── tracelens_session_facade.go  # Database operations
    └── facade.go                    # Add GetTraceLensSession()

api/pkg/tracelens/
├── types.go                      # Request/Response types
├── manager.go                    # SessionManager service
├── pod_manager.go                # Kubernetes pod management
├── proxy.go                      # HTTP/WS proxy handler
└── routes.go                     # API route registration

jobs/pkg/jobs/
└── tracelens_cleanup/
    └── cleanup.go                # Expired session cleanup job

Lens/docker/tracelens/
├── Dockerfile
└── analyze_trace.py
```

---

## Testing Checklist

### Phase 1 Tests (MCP - Database)
- [ ] Insert session record via MCP
- [ ] Query by session_id
- [ ] Query by workload_uid  
- [ ] Update status
- [ ] List expired sessions
- [ ] Delete session

### Phase 2 Tests (MCP + Code - Session Manager)
- [ ] Generate session ID correctly
- [ ] Create session with valid profiler file
- [ ] Reuse existing ready session
- [ ] Multi-cluster operations work

### Phase 3 Tests (kubectl - Pod Management)
- [ ] Create pod with correct labels
- [ ] Pod reaches Running state
- [ ] Pod IP is captured
- [ ] Pod deletion triggers status update

### Phase 4 Tests (Browser - Proxy)
- [ ] HTTP proxy works
- [ ] WebSocket connects
- [ ] Streamlit UI loads

### Phase 5 Tests (curl - API)
- [ ] POST /sessions creates session
- [ ] GET /sessions/:id returns status
- [ ] PATCH extends TTL
- [ ] DELETE removes session and pod

### Phase 6 Tests (MCP - Cleanup Job)
- [ ] Expired sessions are detected
- [ ] Pods are deleted
- [ ] Session status updated to expired

### Phase 7 Tests (Docker - Image)
- [ ] Image builds successfully
- [ ] Container starts with health check
- [ ] TraceLens loads trace file

---

## Next Steps

Ready to start **Phase 1: Create TraceLens Session Facade**?
