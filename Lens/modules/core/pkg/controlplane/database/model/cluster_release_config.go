// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameClusterReleaseConfig = "cluster_release_configs"

// ClusterReleaseConfig represents per-cluster release configuration
type ClusterReleaseConfig struct {
	ID                int32      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ClusterName       string     `gorm:"column:cluster_name;not null;uniqueIndex" json:"cluster_name"`
	ReleaseVersionID  *int32     `gorm:"column:release_version_id" json:"release_version_id"`
	ValuesOverride    ValuesJSON `gorm:"column:values_override;type:jsonb" json:"values_override"`
	DeployedVersionID *int32     `gorm:"column:deployed_version_id" json:"deployed_version_id"`
	DeployedValues    ValuesJSON `gorm:"column:deployed_values;type:jsonb" json:"deployed_values"`
	DeployedAt        *time.Time `gorm:"column:deployed_at" json:"deployed_at"`
	SyncStatus        string     `gorm:"column:sync_status;default:unknown" json:"sync_status"`
	LastSyncAt        *time.Time `gorm:"column:last_sync_at" json:"last_sync_at"`
	LastSyncError     string     `gorm:"column:last_sync_error" json:"last_sync_error"`
	AutoUpgrade       bool       `gorm:"column:auto_upgrade;default:false" json:"auto_upgrade"`
	UpgradeChannel    string     `gorm:"column:upgrade_channel;default:stable" json:"upgrade_channel"`
	CreatedAt         time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	// Relations (for eager loading)
	ReleaseVersion  *ReleaseVersion `gorm:"foreignKey:ReleaseVersionID" json:"release_version,omitempty"`
	DeployedVersion *ReleaseVersion `gorm:"foreignKey:DeployedVersionID" json:"deployed_version,omitempty"`
}

func (*ClusterReleaseConfig) TableName() string {
	return TableNameClusterReleaseConfig
}

// Sync status constants
const (
	SyncStatusUnknown   = "unknown"
	SyncStatusSynced    = "synced"
	SyncStatusOutOfSync = "out_of_sync"
	SyncStatusUpgrading = "upgrading"
	SyncStatusFailed    = "failed"
)

// IsOutOfSync checks if cluster needs deployment
func (c *ClusterReleaseConfig) IsOutOfSync() bool {
	if c.ReleaseVersionID == nil {
		return false
	}
	if c.DeployedVersionID == nil {
		return true
	}
	return *c.ReleaseVersionID != *c.DeployedVersionID
}
