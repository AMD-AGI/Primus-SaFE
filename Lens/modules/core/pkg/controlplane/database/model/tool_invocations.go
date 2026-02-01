// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameToolInvocations = "tool_invocations"

// ToolInvocation represents a tool invocation log entry
type ToolInvocation struct {
	ID         int64          `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ToolName   string         `gorm:"column:tool_name;not null;index" json:"tool_name"`
	UserID     string         `gorm:"column:user_id;index" json:"user_id"`
	Input      InvocationData `gorm:"column:input" json:"input"`
	Output     InvocationData `gorm:"column:output" json:"output"`
	Error      string         `gorm:"column:error" json:"error"`
	DurationMs int            `gorm:"column:duration_ms" json:"duration_ms"`
	Success    bool           `gorm:"column:success;index" json:"success"`
	InvokedAt  time.Time      `gorm:"column:invoked_at;autoCreateTime;index" json:"invoked_at"`
}

// TableName returns the table name
func (*ToolInvocation) TableName() string {
	return TableNameToolInvocations
}

// InvocationData is a custom type for JSONB input/output field
type InvocationData map[string]interface{}

// Value implements driver.Valuer interface
func (d InvocationData) Value() (driver.Value, error) {
	if d == nil {
		return nil, nil
	}
	b, err := json.Marshal(d)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (d *InvocationData) Scan(value interface{}) error {
	if value == nil {
		*d = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, d)
	case string:
		return json.Unmarshal([]byte(v), d)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}
