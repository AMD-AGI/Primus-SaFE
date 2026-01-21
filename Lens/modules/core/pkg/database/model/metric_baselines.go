// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameMetricBaselines = "metric_baselines"

// BaselineType constants
const (
	BaselineTypeRolling7D  = "rolling_7d"
	BaselineTypeRolling30D = "rolling_30d"
	BaselineTypeFixed      = "fixed"
)

// MetricBaselines represents historical baseline values for each metric
type MetricBaselines struct {
	ID            int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ConfigID      int64      `gorm:"column:config_id;not null" json:"config_id"`
	MetricName    string     `gorm:"column:metric_name;not null" json:"metric_name"`
	DimensionKey  string     `gorm:"column:dimension_key" json:"dimension_key"`
	Dimensions    ExtType    `gorm:"column:dimensions" json:"dimensions"`
	BaselineValue *float64   `gorm:"column:baseline_value;type:decimal(20,6)" json:"baseline_value"`
	BaselineType  string     `gorm:"column:baseline_type;not null;default:rolling_7d" json:"baseline_type"`
	AvgValue      *float64   `gorm:"column:avg_value;type:decimal(20,6)" json:"avg_value"`
	MinValue      *float64   `gorm:"column:min_value;type:decimal(20,6)" json:"min_value"`
	MaxValue      *float64   `gorm:"column:max_value;type:decimal(20,6)" json:"max_value"`
	StddevValue   *float64   `gorm:"column:stddev_value;type:decimal(20,6)" json:"stddev_value"`
	SampleCount   int        `gorm:"column:sample_count;not null;default:0" json:"sample_count"`
	StartDate     *time.Time `gorm:"column:start_date;type:date" json:"start_date"`
	EndDate       *time.Time `gorm:"column:end_date;type:date" json:"end_date"`
	LastUpdatedAt time.Time  `gorm:"column:last_updated_at;not null;default:now()" json:"last_updated_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (*MetricBaselines) TableName() string {
	return TableNameMetricBaselines
}

// BaselineStats represents baseline statistics
type BaselineStats struct {
	Avg    float64 `json:"avg"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Stddev float64 `json:"stddev"`
	Count  int     `json:"count"`
}
