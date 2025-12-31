// Note: This file is manually created as a placeholder.
// After running `go run gen/main.go`, this should be replaced by ai_tasks.gen.go

package model

import (
	"encoding/json"
	"time"
)

const TableNameAITask = "ai_tasks"

// AITask mapped from table <ai_tasks>
type AITask struct {
	ID            string                 `gorm:"column:id;primaryKey;size:64" json:"id"`
	Topic         string                 `gorm:"column:topic;not null;size:128" json:"topic"`
	Status        string                 `gorm:"column:status;not null;size:32;default:'pending'" json:"status"`
	Priority      int                    `gorm:"column:priority;default:0" json:"priority"`
	InputPayload  json.RawMessage        `gorm:"column:input_payload;type:jsonb;not null" json:"input_payload"`
	OutputPayload json.RawMessage        `gorm:"column:output_payload;type:jsonb" json:"output_payload,omitempty"`
	ErrorMessage  string                 `gorm:"column:error_message;size:1024" json:"error_message,omitempty"`
	ErrorCode     int                    `gorm:"column:error_code" json:"error_code,omitempty"`
	RetryCount    int                    `gorm:"column:retry_count;default:0" json:"retry_count"`
	MaxRetries    int                    `gorm:"column:max_retries;default:3" json:"max_retries"`
	AgentID       string                 `gorm:"column:agent_id;size:128" json:"agent_id,omitempty"`
	ContextJSON   string                 `gorm:"column:context;type:jsonb;default:'{}'" json:"-"`
	Context       map[string]interface{} `gorm:"-" json:"context"`
	CreatedAt     time.Time              `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	StartedAt     *time.Time             `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt   *time.Time             `gorm:"column:completed_at" json:"completed_at,omitempty"`
	TimeoutAt     time.Time              `gorm:"column:timeout_at;not null" json:"timeout_at"`
}

// TableName AITask's table name
func (*AITask) TableName() string {
	return TableNameAITask
}
