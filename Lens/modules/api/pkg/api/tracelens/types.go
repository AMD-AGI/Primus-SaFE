// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens

import "time"

// CreateSessionRequest is the request body for creating a TraceLens session
type CreateSessionRequest struct {
	WorkloadUID     string `json:"workload_uid" binding:"required"`
	ProfilerFileID  int32  `json:"profiler_file_id" binding:"required"`
	TTLMinutes      int    `json:"ttl_minutes"`       // default: 60
	ResourceProfile string `json:"resource_profile"`  // default: medium
}

// SessionResponse is the response for session operations
type SessionResponse struct {
	SessionID       string     `json:"session_id"`
	ClusterName     string     `json:"cluster_name"`
	WorkloadUID     string     `json:"workload_uid"`
	ProfilerFileID  int32      `json:"profiler_file_id"`
	Status          string     `json:"status"`
	StatusMessage   string     `json:"status_message,omitempty"`
	UIPath          string     `json:"ui_path,omitempty"`
	PodName         string     `json:"pod_name,omitempty"`
	PodIP           string     `json:"pod_ip,omitempty"`
	ResourceProfile string     `json:"resource_profile"`
	CreatedAt       time.Time  `json:"created_at"`
	ReadyAt         *time.Time `json:"ready_at,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at"`
	LastAccessedAt  *time.Time `json:"last_accessed_at,omitempty"`
	EstimatedReady  int        `json:"estimated_ready_seconds,omitempty"`
}

// ExtendSessionRequest is the request body for extending a session TTL
type ExtendSessionRequest struct {
	ExtendMinutes int `json:"extend_minutes" binding:"required,min=1,max=120"`
}

// ListSessionsResponse is the response for listing sessions
type ListSessionsResponse struct {
	Sessions []SessionResponse `json:"sessions"`
	Total    int               `json:"total"`
}

// SessionStatusResponse is a simplified status response
type SessionStatusResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// ResourceProfileResponse is the response for resource profiles
type ResourceProfileResponse struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Memory      string `json:"memory"`
	MemoryBytes int64  `json:"memory_bytes"`
	CPU         int    `json:"cpu"`
	IsDefault   bool   `json:"is_default,omitempty"`
}

// ResourceProfilesResponse is the response for listing resource profiles
type ResourceProfilesResponse struct {
	Profiles []ResourceProfileResponse `json:"profiles"`
}

