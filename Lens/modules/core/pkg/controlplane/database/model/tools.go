// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameTools = "tools"

// Tool represents a registered tool in the tools repository
type Tool struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name        string `gorm:"column:name;not null;uniqueIndex" json:"name"`
	Version     string `gorm:"column:version;not null" json:"version"`
	Description string `gorm:"column:description;not null" json:"description"`
	Category    string `gorm:"column:category" json:"category"`
	Domain      string `gorm:"column:domain" json:"domain"`
	Tags        Tags   `gorm:"column:tags;default:[]" json:"tags"`

	// Provider information
	ProviderType      string `gorm:"column:provider_type;not null" json:"provider_type"`
	ProviderEndpoint  string `gorm:"column:provider_endpoint;not null" json:"provider_endpoint"`
	ProviderTimeoutMs int    `gorm:"column:provider_timeout_ms;default:30000" json:"provider_timeout_ms"`

	// Schema information
	InputSchema  ToolSchema `gorm:"column:input_schema;default:{}" json:"input_schema"`
	OutputSchema ToolSchema `gorm:"column:output_schema;default:{}" json:"output_schema"`

	// Annotations (hints about tool behavior)
	ReadOnlyHint    bool `gorm:"column:read_only_hint;default:true" json:"read_only_hint"`
	DestructiveHint bool `gorm:"column:destructive_hint;default:false" json:"destructive_hint"`
	IdempotentHint  bool `gorm:"column:idempotent_hint;default:true" json:"idempotent_hint"`
	OpenWorldHint   bool `gorm:"column:open_world_hint;default:false" json:"open_world_hint"`

	// Access control
	AccessScope string   `gorm:"column:access_scope;default:platform" json:"access_scope"`
	AccessRoles Strings  `gorm:"column:access_roles;default:[]" json:"access_roles"`
	AccessTeams Strings  `gorm:"column:access_teams;default:[]" json:"access_teams"`
	AccessUsers Strings  `gorm:"column:access_users;default:[]" json:"access_users"`

	// Examples
	Examples ToolExamples `gorm:"column:examples;default:[]" json:"examples"`

	// Owner information
	OwnerType string `gorm:"column:owner_type" json:"owner_type"`
	OwnerID   string `gorm:"column:owner_id" json:"owner_id"`

	// Status
	Status string `gorm:"column:status;default:active" json:"status"`

	// Timestamps
	RegisteredAt time.Time `gorm:"column:registered_at;autoCreateTime" json:"registered_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (*Tool) TableName() string {
	return TableNameTools
}

// Tags is a custom type for JSONB tags array field
type Tags []string

// Value implements driver.Valuer interface
func (t Tags) Value() (driver.Value, error) {
	if t == nil {
		return "[]", nil
	}
	b, err := json.Marshal(t)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (t *Tags) Scan(value interface{}) error {
	if value == nil {
		*t = make(Tags, 0)
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

// Strings is a custom type for JSONB string array field
type Strings []string

// Value implements driver.Valuer interface
func (s Strings) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (s *Strings) Scan(value interface{}) error {
	if value == nil {
		*s = make(Strings, 0)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

// ToolSchema is a custom type for JSONB schema field
type ToolSchema map[string]interface{}

// Value implements driver.Valuer interface
func (s ToolSchema) Value() (driver.Value, error) {
	if s == nil {
		return "{}", nil
	}
	b, err := json.Marshal(s)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (s *ToolSchema) Scan(value interface{}) error {
	if value == nil {
		*s = make(ToolSchema)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

// ToolExample represents a usage example for a tool
type ToolExample struct {
	Description string                 `json:"description"`
	Input       map[string]interface{} `json:"input"`
	Output      map[string]interface{} `json:"output"`
}

// ToolExamples is a custom type for JSONB examples array field
type ToolExamples []ToolExample

// Value implements driver.Valuer interface
func (e ToolExamples) Value() (driver.Value, error) {
	if e == nil {
		return "[]", nil
	}
	b, err := json.Marshal(e)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (e *ToolExamples) Scan(value interface{}) error {
	if value == nil {
		*e = make(ToolExamples, 0)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, e)
	case string:
		return json.Unmarshal([]byte(v), e)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

// Tool scope constants
const (
	ToolScopePlatform = "platform"
	ToolScopeTeam     = "team"
	ToolScopeUser     = "user"
)

// Tool status constants
const (
	ToolStatusActive   = "active"
	ToolStatusInactive = "inactive"
	ToolStatusDisabled = "disabled"
)

// Provider type constants
const (
	ProviderTypeMCP  = "mcp"
	ProviderTypeHTTP = "http"
	ProviderTypeGRPC = "grpc"
)

// Tool category constants
const (
	ToolCategoryObservability = "observability"
	ToolCategoryDiagnosis     = "diagnosis"
	ToolCategoryManagement    = "management"
	ToolCategoryData          = "data"
	ToolCategoryWorkflow      = "workflow"
)
