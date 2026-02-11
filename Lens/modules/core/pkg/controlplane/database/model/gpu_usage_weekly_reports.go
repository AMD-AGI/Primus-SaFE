// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

const TableNameGpuUsageWeeklyReports = "gpu_usage_weekly_reports"

// JSONMap is a custom type for JSONB fields
type JSONMap map[string]interface{}

// Value implements the driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// GpuUsageWeeklyReports represents a GPU usage weekly report stored in control plane
type GpuUsageWeeklyReports struct {
	ID           string    `gorm:"column:id;primaryKey" json:"id"`
	ClusterName  string    `gorm:"column:cluster_name;not null" json:"cluster_name"`
	PeriodStart  time.Time `gorm:"column:period_start;not null" json:"period_start"`
	PeriodEnd    time.Time `gorm:"column:period_end;not null" json:"period_end"`
	GeneratedAt  time.Time `gorm:"column:generated_at;not null" json:"generated_at"`
	Status       string    `gorm:"column:status;not null;default:pending" json:"status"`
	HTMLContent  []byte    `gorm:"column:html_content" json:"html_content,omitempty"`
	PdfContent   []byte    `gorm:"column:pdf_content" json:"pdf_content,omitempty"`
	JSONContent  JSONMap   `gorm:"column:json_content;type:jsonb" json:"json_content,omitempty"`
	Metadata     JSONMap   `gorm:"column:metadata;type:jsonb" json:"metadata,omitempty"`
	ErrorMessage string    `gorm:"column:error_message" json:"error_message,omitempty"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName returns the table name
func (*GpuUsageWeeklyReports) TableName() string {
	return TableNameGpuUsageWeeklyReports
}

// ReportStatus constants
const (
	ReportStatusPending   = "pending"
	ReportStatusGenerated = "generated"
	ReportStatusSent      = "sent"
	ReportStatusFailed    = "failed"
)
