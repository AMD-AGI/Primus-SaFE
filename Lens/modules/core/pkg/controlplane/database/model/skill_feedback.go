// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameSkillFeedback = "skill_feedback"

// SkillFeedback represents user feedback for a skill execution
type SkillFeedback struct {
	ID          int64        `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ExecutionID int64        `gorm:"column:execution_id;index" json:"execution_id"`
	SkillName   string       `gorm:"column:skill_name;not null;index" json:"skill_name"`
	UserID      string       `gorm:"column:user_id" json:"user_id"`
	Rating      int          `gorm:"column:rating;index" json:"rating"` // 1-5
	Comment     string       `gorm:"column:comment" json:"comment"`
	Tags        FeedbackTags `gorm:"column:tags;default:[]" json:"tags"`
	CreatedAt   time.Time    `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName returns the table name
func (*SkillFeedback) TableName() string {
	return TableNameSkillFeedback
}

// FeedbackTags is a custom type for JSONB tags array field
type FeedbackTags []string

// Value implements driver.Valuer interface
func (t FeedbackTags) Value() (driver.Value, error) {
	if t == nil {
		return "[]", nil
	}
	b, err := json.Marshal(t)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (t *FeedbackTags) Scan(value interface{}) error {
	if value == nil {
		*t = make(FeedbackTags, 0)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, t)
	case string:
		return json.Unmarshal([]byte(v), t)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}
