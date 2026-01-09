// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Note: This file is manually created as a placeholder.
// After running `go run gen/main.go`, this should be replaced by ai_agent_registrations.gen.go

package model

import (
	"time"
)

const TableNameAIAgentRegistration = "ai_agent_registrations"

// AIAgentRegistration mapped from table <ai_agent_registrations>
type AIAgentRegistration struct {
	Name            string            `gorm:"column:name;primaryKey;size:128" json:"name"`
	Endpoint        string            `gorm:"column:endpoint;not null;size:512" json:"endpoint"`
	TopicsJSON      string            `gorm:"column:topics;type:jsonb;default:'[]'" json:"-"`
	Topics          []string          `gorm:"-" json:"topics"`
	HealthCheckPath string            `gorm:"column:health_check_path;size:256;default:'/health'" json:"health_check_path"`
	TimeoutSecs     int               `gorm:"column:timeout_secs;default:60" json:"timeout_secs"`
	Timeout         time.Duration     `gorm:"-" json:"timeout"`
	Status          string            `gorm:"column:status;size:32;default:'unknown'" json:"status"`
	LastHealthCheck *time.Time        `gorm:"column:last_health_check" json:"last_health_check"`
	FailureCount    int               `gorm:"column:failure_count;default:0" json:"failure_count"`
	MetadataJSON    string            `gorm:"column:metadata;type:jsonb;default:'{}'" json:"-"`
	Metadata        map[string]string `gorm:"-" json:"metadata"`
	RegisteredAt    time.Time         `gorm:"column:registered_at;not null;default:now()" json:"registered_at"`
	UpdatedAt       time.Time         `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

// TableName AIAgentRegistration's table name
func (*AIAgentRegistration) TableName() string {
	return TableNameAIAgentRegistration
}

