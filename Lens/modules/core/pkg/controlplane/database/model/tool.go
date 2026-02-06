// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/pgvector/pgvector-go"
)

const TableNameTools = "tools"

// Tool represents a unified tool (skill or mcp)
type Tool struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Type        string    `gorm:"column:type;not null" json:"type"` // skill, mcp
	Name        string    `gorm:"column:name;not null" json:"name"`
	DisplayName string    `gorm:"column:display_name" json:"display_name"`
	Description string    `gorm:"column:description;not null" json:"description"`
	Tags        AppTags   `gorm:"column:tags;default:[]" json:"tags"`
	IconURL     string    `gorm:"column:icon_url" json:"icon_url"`
	Author      string    `gorm:"column:author" json:"author"`
	Config      AppConfig `gorm:"column:config;not null;default:{}" json:"config"`

	// Source tracking (for skill only)
	SkillSource    string `gorm:"column:skill_source;default:manual" json:"skill_source"` // manual, github, zip
	SkillSourceURL string `gorm:"column:skill_source_url" json:"skill_source_url"`

	// Access control
	OwnerUserID string `gorm:"column:owner_user_id" json:"owner_user_id"`
	IsPublic    bool   `gorm:"column:is_public;default:true" json:"is_public"`
	Status      string `gorm:"column:status;default:active" json:"status"`

	// Statistics
	RunCount      int `gorm:"column:run_count;default:0" json:"run_count"`
	DownloadCount int `gorm:"column:download_count;default:0" json:"download_count"`

	// Semantic search (not exposed in JSON response)
	Embedding pgvector.Vector `gorm:"type:vector(1024)" json:"-"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (*Tool) TableName() string {
	return TableNameTools
}

// AppConfig is the unified config field for different app types
type AppConfig map[string]interface{}

// Value implements driver.Valuer interface
func (c AppConfig) Value() (driver.Value, error) {
	if c == nil {
		return "{}", nil
	}
	b, err := json.Marshal(c)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (c *AppConfig) Scan(value interface{}) error {
	if value == nil {
		*c = make(AppConfig)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, c)
	case string:
		return json.Unmarshal([]byte(v), c)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

// AppTags is a custom type for JSONB tags array field
type AppTags []string

// Value implements driver.Valuer interface
func (t AppTags) Value() (driver.Value, error) {
	if t == nil {
		return "[]", nil
	}
	b, err := json.Marshal(t)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (t *AppTags) Scan(value interface{}) error {
	if value == nil {
		*t = make(AppTags, 0)
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

// App type constants
const (
	AppTypeSkill = "skill"
	AppTypeMCP   = "mcp"
)

// Skill source constants
const (
	SkillSourceUpload = "upload" // Single SKILL.md file upload
	SkillSourceGitHub = "github" // GitHub import
	SkillSourceZIP    = "zip"    // ZIP import
)

// App status constants
const (
	AppStatusActive   = "active"
	AppStatusInactive = "inactive"
)

// GetMCPServerConfig extracts MCP server config from Config field
func (t *Tool) GetMCPServerConfig() (command string, args []string, env map[string]string) {
	if t.Type != AppTypeMCP {
		return
	}

	// Extract server config from mcpServers format
	var serverConfig map[string]interface{}

	if mcpServers, ok := t.Config["mcpServers"].(map[string]interface{}); ok {
		// Get the first (and usually only) server config
		for _, cfg := range mcpServers {
			if cfgMap, ok := cfg.(map[string]interface{}); ok {
				serverConfig = cfgMap
				break
			}
		}
	} else {
		// Fallback: direct format (command, args, env at root level)
		serverConfig = t.Config
	}

	if serverConfig == nil {
		return
	}

	if cmd, ok := serverConfig["command"].(string); ok {
		command = cmd
	}
	if argsList, ok := serverConfig["args"].([]interface{}); ok {
		for _, arg := range argsList {
			if s, ok := arg.(string); ok {
				args = append(args, s)
			}
		}
	}
	if envMap, ok := serverConfig["env"].(map[string]interface{}); ok {
		env = make(map[string]string)
		for k, v := range envMap {
			if s, ok := v.(string); ok {
				env[k] = s
			}
		}
	}
	return
}

// GetSkillS3Key extracts skill S3 key from Config field
func (t *Tool) GetSkillS3Key() string {
	if t.Type != AppTypeSkill {
		return ""
	}
	if key, ok := t.Config["s3_key"].(string); ok {
		return key
	}
	return ""
}
