// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameClusterDeploymentHistory = "cluster_deployment_history"

// ClusterDeploymentHistory tracks dataplane deployment operations
type ClusterDeploymentHistory struct {
	ID          int32  `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ClusterName string `gorm:"column:cluster_name;not null" json:"cluster_name"`

	// Deployment Info
	DeploymentType string `gorm:"column:deployment_type;not null" json:"deployment_type"` // 'install', 'upgrade', 'uninstall'
	Version        string `gorm:"column:version" json:"version"`
	ValuesYAML     string `gorm:"column:values_yaml" json:"values_yaml"`

	// Status
	Status  string `gorm:"column:status;not null" json:"status"` // 'started', 'success', 'failed'
	Message string `gorm:"column:message" json:"message"`
	Logs    string `gorm:"column:logs" json:"logs"`

	// Timing
	StartedAt  time.Time  `gorm:"column:started_at;autoCreateTime" json:"started_at"`
	FinishedAt *time.Time `gorm:"column:finished_at" json:"finished_at"`
}

// TableName returns the table name
func (*ClusterDeploymentHistory) TableName() string {
	return TableNameClusterDeploymentHistory
}

// Deployment type constants
const (
	DeploymentTypeInstall   = "install"
	DeploymentTypeUpgrade   = "upgrade"
	DeploymentTypeUninstall = "uninstall"
)

// Deployment status constants
const (
	DeploymentStatusStarted = "started"
	DeploymentStatusSuccess = "success"
	DeploymentStatusFailed  = "failed"
)
