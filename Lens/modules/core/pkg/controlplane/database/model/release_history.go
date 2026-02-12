// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"time"

	"github.com/lib/pq"
)

const TableNameReleaseHistory = "release_history"

// ReleaseHistory represents a deployment history record
type ReleaseHistory struct {
	ID                int32          `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ClusterName       string         `gorm:"column:cluster_name;not null" json:"cluster_name"`
	ReleaseVersionID  int32          `gorm:"column:release_version_id;not null" json:"release_version_id"`
	Action            string         `gorm:"column:action;not null" json:"action"`
	TriggeredBy       string         `gorm:"column:triggered_by" json:"triggered_by"`
	ValuesSnapshot    ValuesJSON     `gorm:"column:values_snapshot;type:jsonb;not null" json:"values_snapshot"`
	PreviousVersionID *int32         `gorm:"column:previous_version_id" json:"previous_version_id"`
	Status            string         `gorm:"column:status;default:pending" json:"status"`
	ErrorMessage      string         `gorm:"column:error_message" json:"error_message"`
	StagesCompleted   StringArray    `gorm:"column:stages_completed;type:text[]" json:"stages_completed"`
	TaskID            *int32         `gorm:"column:task_id" json:"task_id"`
	CreatedAt         time.Time      `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	StartedAt         *time.Time     `gorm:"column:started_at" json:"started_at"`
	CompletedAt       *time.Time     `gorm:"column:completed_at" json:"completed_at"`

	// Relations
	ReleaseVersion  *ReleaseVersion `gorm:"foreignKey:ReleaseVersionID" json:"release_version,omitempty"`
	PreviousVersion *ReleaseVersion `gorm:"foreignKey:PreviousVersionID" json:"previous_version,omitempty"`
}

func (*ReleaseHistory) TableName() string {
	return TableNameReleaseHistory
}

// Release action constants
const (
	ReleaseActionInstall  = "install"
	ReleaseActionUpgrade  = "upgrade"
	ReleaseActionRollback = "rollback"
	ReleaseActionSync     = "sync"
)

// Release history status constants
const (
	ReleaseHistoryStatusPending   = "pending"
	ReleaseHistoryStatusRunning   = "running"
	ReleaseHistoryStatusCompleted = "completed"
	ReleaseHistoryStatusFailed    = "failed"
)

// StringArray is a custom type for PostgreSQL text[]
type StringArray []string

// Value implements driver.Valuer interface
func (a StringArray) Value() (driver.Value, error) {
	return pq.Array(a).Value()
}

// Scan implements sql.Scanner interface
func (a *StringArray) Scan(value interface{}) error {
	return pq.Array(a).Scan(value)
}
