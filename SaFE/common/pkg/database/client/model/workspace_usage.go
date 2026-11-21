/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model

import (
	"time"
)

const (
	TableNameWorkspaceUsageSample = "workspace_usage_sample"
	TableNameWorkspaceUsageHourly = "workspace_usage_hourly"
)

// WorkspaceUsageSample mapped from table <workspace_usage_sample>.
// Busy and UtilCoverage are nil when the utilization is unknown.
type WorkspaceUsageSample struct {
	ID               int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	SampleTime       time.Time `gorm:"column:sample_time;not null" json:"sample_time"`
	Cluster          string    `gorm:"column:cluster;not null" json:"cluster"`
	Workspace        string    `gorm:"column:workspace;not null" json:"workspace"`
	Resource         string    `gorm:"column:resource;not null" json:"resource"`
	Quota            float64   `gorm:"column:quota;not null" json:"quota"`
	Allocated        float64   `gorm:"column:allocated;not null" json:"allocated"`
	Busy             *float64  `gorm:"column:busy" json:"busy"`
	UtilCoverage     *float64  `gorm:"column:util_coverage" json:"util_coverage"`
	QueuedQuota      float64   `gorm:"column:queued_quota;not null" json:"queued_quota"`
	QueuedUserQos    float64   `gorm:"column:queued_user_qos;not null" json:"queued_user_qos"`
	QueuedExternal   float64   `gorm:"column:queued_external;not null" json:"queued_external"`
	QueuedOther      float64   `gorm:"column:queued_other;not null" json:"queued_other"`
	RunningWorkloads int32     `gorm:"column:running_workloads;not null" json:"running_workloads"`
	QueuedWorkloads  int32     `gorm:"column:queued_workloads;not null" json:"queued_workloads"`
}

// TableName WorkspaceUsageSample's table name
func (*WorkspaceUsageSample) TableName() string {
	return TableNameWorkspaceUsageSample
}

// WorkspaceUsageHourly mapped from table <workspace_usage_hourly>.
type WorkspaceUsageHourly struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Hour            time.Time `gorm:"column:hour;not null" json:"hour"`
	Cluster         string    `gorm:"column:cluster;not null" json:"cluster"`
	Workspace       string    `gorm:"column:workspace;not null" json:"workspace"`
	Resource        string    `gorm:"column:resource;not null" json:"resource"`
	QuotaAvg        float64   `gorm:"column:quota_avg;not null" json:"quota_avg"`
	AllocatedAvg    float64   `gorm:"column:allocated_avg;not null" json:"allocated_avg"`
	AllocatedMax    float64   `gorm:"column:allocated_max;not null" json:"allocated_max"`
	BusyAvg         *float64  `gorm:"column:busy_avg" json:"busy_avg"`
	UtilCoverageAvg *float64  `gorm:"column:util_coverage_avg" json:"util_coverage_avg"`
	QueuedAvg       float64   `gorm:"column:queued_avg;not null" json:"queued_avg"`
	QueuedMax       float64   `gorm:"column:queued_max;not null" json:"queued_max"`
	AllocatedHours  float64   `gorm:"column:allocated_hours;not null" json:"allocated_hours"`
	BusyHours       *float64  `gorm:"column:busy_hours" json:"busy_hours"`
	Samples         int32     `gorm:"column:samples;not null" json:"samples"`
}

// TableName WorkspaceUsageHourly's table name
func (*WorkspaceUsageHourly) TableName() string {
	return TableNameWorkspaceUsageHourly
}
