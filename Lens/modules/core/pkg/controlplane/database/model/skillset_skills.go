// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameSkillsetSkills = "skillset_skills"

// SkillsetSkill represents the many-to-many relationship between skillsets and skills
type SkillsetSkill struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	SkillsetID int64     `gorm:"column:skillset_id;not null;uniqueIndex:idx_skillset_skill" json:"skillset_id"`
	SkillID    int64     `gorm:"column:skill_id;not null;uniqueIndex:idx_skillset_skill" json:"skill_id"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName returns the table name
func (*SkillsetSkill) TableName() string {
	return TableNameSkillsetSkills
}
