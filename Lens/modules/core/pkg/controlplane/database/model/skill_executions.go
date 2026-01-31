// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameSkillExecutions = "skill_executions"

// SkillExecution represents an execution log entry for a skill
type SkillExecution struct {
	ID          int64              `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	SkillName   string             `gorm:"column:skill_name;not null;index" json:"skill_name"`
	UserID      string             `gorm:"column:user_id;index" json:"user_id"`
	SessionID   string             `gorm:"column:session_id" json:"session_id"`
	StartedAt   time.Time          `gorm:"column:started_at;not null" json:"started_at"`
	CompletedAt *time.Time         `gorm:"column:completed_at" json:"completed_at"`
	DurationMs  int                `gorm:"column:duration_ms" json:"duration_ms"`
	Status      string             `gorm:"column:status;index" json:"status"`
	Variables   ExecutionVariables `gorm:"column:variables;default:{}" json:"variables"`
	CreatedAt   time.Time          `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName returns the table name
func (*SkillExecution) TableName() string {
	return TableNameSkillExecutions
}

// ExecutionVariables is a custom type for JSONB variables field
type ExecutionVariables map[string]string

// Value implements driver.Valuer interface
func (v ExecutionVariables) Value() (driver.Value, error) {
	if v == nil {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (v *ExecutionVariables) Scan(value interface{}) error {
	if value == nil {
		*v = make(ExecutionVariables)
		return nil
	}
	switch val := value.(type) {
	case []byte:
		return json.Unmarshal(val, v)
	case string:
		return json.Unmarshal([]byte(val), v)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

// Execution status constants
const (
	ExecutionStatusPending   = "pending"
	ExecutionStatusRunning   = "running"
	ExecutionStatusCompleted = "completed"
	ExecutionStatusFailed    = "failed"
)
