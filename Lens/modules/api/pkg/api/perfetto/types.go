package perfetto

import "time"

// CreateSessionRequest is the request body for creating a Perfetto viewer session
type CreateSessionRequest struct {
	WorkloadUID    string `json:"workload_uid" binding:"required"`
	ProfilerFileID int32  `json:"profiler_file_id" binding:"required"`
	TTLMinutes     int    `json:"ttl_minutes"` // default: 30
}

// SessionResponse is the response for session operations
type SessionResponse struct {
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

// ExtendSessionRequest is the request body for extending a session TTL
type ExtendSessionRequest struct {
	ExtendMinutes int `json:"extend_minutes" binding:"required,min=1,max=60"`
}

// ListSessionsResponse is the response for listing sessions
type ListSessionsResponse struct {
	Sessions []SessionResponse `json:"sessions"`
	Total    int               `json:"total"`
}

