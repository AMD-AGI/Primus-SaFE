// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameWorkspaceSkills = "workspace_skills"

// WorkspaceSkill represents the many-to-many relationship between workspaces and skills
type WorkspaceSkill struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	WorkspaceID int64     `gorm:"column:workspace_id;not null;uniqueIndex:idx_workspace_skill" json:"workspace_id"`
	SkillID     int64     `gorm:"column:skill_id;not null;uniqueIndex:idx_workspace_skill" json:"skill_id"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName returns the table name
func (*WorkspaceSkill) TableName() string {
	return TableNameWorkspaceSkills
}
