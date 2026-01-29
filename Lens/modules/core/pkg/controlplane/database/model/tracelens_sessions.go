// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"

	"gorm.io/gorm"
)

const TableNameTracelensSessions = "tracelens_sessions"

// TracelensSessions represents a TraceLens session stored in control plane
type TracelensSessions struct {
	ID              int32          `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	SessionID       string         `gorm:"column:session_id;not null;uniqueIndex" json:"session_id"`
	ClusterName     string         `gorm:"column:cluster_name;not null" json:"cluster_name"`
	WorkloadUID     string         `gorm:"column:workload_uid;not null" json:"workload_uid"`
	ProfilerFileID  int32          `gorm:"column:profiler_file_id;not null" json:"profiler_file_id"`
	UserID          string         `gorm:"column:user_id" json:"user_id"`
	UserEmail       string         `gorm:"column:user_email" json:"user_email"`
	PodName         string         `gorm:"column:pod_name" json:"pod_name"`
	PodNamespace    string         `gorm:"column:pod_namespace;default:primus-lens" json:"pod_namespace"`
	PodIP           string         `gorm:"column:pod_ip" json:"pod_ip"`
	PodPort         int32          `gorm:"column:pod_port;default:8501" json:"pod_port"`
	Status          string         `gorm:"column:status;not null;default:pending" json:"status"`
	StatusMessage   string         `gorm:"column:status_message" json:"status_message"`
	ResourceProfile string         `gorm:"column:resource_profile;default:medium" json:"resource_profile"`
	Config          JSONMap        `gorm:"column:config;type:jsonb;default:'{}'" json:"config"`
	CreatedAt       time.Time      `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	ReadyAt         time.Time      `gorm:"column:ready_at" json:"ready_at"`
	ExpiresAt       time.Time      `gorm:"column:expires_at;not null" json:"expires_at"`
	LastAccessedAt  time.Time      `gorm:"column:last_accessed_at" json:"last_accessed_at"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at" json:"deleted_at"`
}

// TableName returns the table name
func (*TracelensSessions) TableName() string {
	return TableNameTracelensSessions
}

// Session status constants
const (
	SessionStatusPending      = "pending"
	SessionStatusCreating     = "creating"
	SessionStatusInitializing = "initializing"
	SessionStatusReady        = "ready"
	SessionStatusFailed       = "failed"
	SessionStatusExpired      = "expired"
	SessionStatusDeleted      = "deleted"
)

// ActiveStatuses returns all statuses considered "active"
func ActiveStatuses() []string {
	return []string{
		SessionStatusPending,
		SessionStatusCreating,
		SessionStatusInitializing,
		SessionStatusReady,
	}
}
