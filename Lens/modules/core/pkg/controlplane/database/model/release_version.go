// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameReleaseVersion = "release_versions"

// ReleaseVersion represents a release version definition
type ReleaseVersion struct {
	ID           int32     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	VersionName  string    `gorm:"column:version_name;not null;uniqueIndex" json:"version_name"`
	Channel      string    `gorm:"column:channel;default:stable" json:"channel"`
	ChartRepo    string    `gorm:"column:chart_repo;not null" json:"chart_repo"`
	ChartVersion string    `gorm:"column:chart_version;not null" json:"chart_version"`
	ImageRegistry string   `gorm:"column:image_registry;not null" json:"image_registry"`
	ImageTag     string    `gorm:"column:image_tag;not null" json:"image_tag"`
	DefaultValues ValuesJSON `gorm:"column:default_values;type:jsonb;not null" json:"default_values"`
	ValuesSchema ValuesJSON `gorm:"column:values_schema;type:jsonb" json:"values_schema"`
	Status       string    `gorm:"column:status;default:draft" json:"status"`
	ReleaseNotes string    `gorm:"column:release_notes" json:"release_notes"`
	CreatedBy    string    `gorm:"column:created_by" json:"created_by"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (*ReleaseVersion) TableName() string {
	return TableNameReleaseVersion
}

// Channel constants
const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"
	ChannelCanary = "canary"
)

// Release version status constants
const (
	ReleaseStatusDraft      = "draft"
	ReleaseStatusActive     = "active"
	ReleaseStatusDeprecated = "deprecated"
)

// ValuesJSON is a custom type for JSONB values
type ValuesJSON map[string]interface{}

// Value implements driver.Valuer interface
func (v ValuesJSON) Value() (driver.Value, error) {
	if v == nil {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (v *ValuesJSON) Scan(value interface{}) error {
	if value == nil {
		*v = ValuesJSON{}
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

// MergeValues merges base values with override values
func MergeValues(base, override ValuesJSON) ValuesJSON {
	result := make(ValuesJSON)
	
	// Copy base values
	for k, v := range base {
		result[k] = deepCopy(v)
	}
	
	// Merge override values
	for k, v := range override {
		if existing, ok := result[k]; ok {
			// If both are maps, merge recursively
			if existingMap, ok1 := existing.(map[string]interface{}); ok1 {
				if overrideMap, ok2 := v.(map[string]interface{}); ok2 {
					result[k] = mergeMap(existingMap, overrideMap)
					continue
				}
			}
		}
		result[k] = deepCopy(v)
	}
	
	return result
}

func mergeMap(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range base {
		result[k] = deepCopy(v)
	}
	for k, v := range override {
		if existing, ok := result[k]; ok {
			if existingMap, ok1 := existing.(map[string]interface{}); ok1 {
				if overrideMap, ok2 := v.(map[string]interface{}); ok2 {
					result[k] = mergeMap(existingMap, overrideMap)
					continue
				}
			}
		}
		result[k] = deepCopy(v)
	}
	return result
}

func deepCopy(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	var result interface{}
	json.Unmarshal(b, &result)
	return result
}
