// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameToolDomains = "tool_domains"

// ToolDomain represents a domain grouping of tools
type ToolDomain struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Domain      string    `gorm:"column:domain;not null;uniqueIndex" json:"domain"`
	Description string    `gorm:"column:description" json:"description"`
	ToolNames   Strings   `gorm:"column:tool_names;default:[]" json:"tool_names"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (*ToolDomain) TableName() string {
	return TableNameToolDomains
}

// Tool domain constants
const (
	ToolDomainTraining = "training"
	ToolDomainCluster  = "cluster"
	ToolDomainWorkflow = "workflow"
	ToolDomainLogging  = "logging"
)
