// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

const TableNameTrainingLogPattern = "training_log_pattern"

// TrainingLogPattern represents a global log parsing regex pattern.
// Patterns are matched against all incoming logs regardless of framework.
type TrainingLogPattern struct {
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Pattern           string     `gorm:"column:pattern;not null" json:"pattern"`
	PatternType       string     `gorm:"column:pattern_type;not null;default:performance" json:"pattern_type"` // performance, blacklist, training_event, checkpoint_event
	EventSubtype      *string    `gorm:"column:event_subtype" json:"event_subtype,omitempty"`                  // start_training, end_training, start_saving, end_saving, loading
	Source            string     `gorm:"column:source;not null;default:manual" json:"source"`                  // manual, autodiscovered, migration
	SourceWorkloadUID *string    `gorm:"column:source_workload_uid" json:"source_workload_uid,omitempty"`
	Framework         *string    `gorm:"column:framework" json:"framework,omitempty"` // informational only
	Name              *string    `gorm:"column:name" json:"name,omitempty"`
	Description       *string    `gorm:"column:description" json:"description,omitempty"`
	SampleLine        *string    `gorm:"column:sample_line" json:"sample_line,omitempty"`
	Enabled           bool       `gorm:"column:enabled;not null;default:false" json:"enabled"`
	Priority          int        `gorm:"column:priority;not null;default:50" json:"priority"`
	Confidence        float64    `gorm:"column:confidence;not null;default:0.5" json:"confidence"`
	HitCount          int64      `gorm:"column:hit_count;not null;default:0" json:"hit_count"`
	LastHitAt         *time.Time `gorm:"column:last_hit_at" json:"last_hit_at,omitempty"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (*TrainingLogPattern) TableName() string {
	return TableNameTrainingLogPattern
}
