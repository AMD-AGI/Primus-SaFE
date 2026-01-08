// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Service Pod Reference model for gateway-exporter
// This file stores the relationship between Kubernetes Services and their backend Pods

package model

import (
	"time"
)

const TableNameServicePodReference = "service_pod_references"

// ServicePodReference represents the relationship between a Service and its backend Pods
type ServicePodReference struct {
	ID               int64   `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ServiceUID       string  `gorm:"column:service_uid;type:varchar(64);index" json:"service_uid"`
	ServiceName      string  `gorm:"column:service_name;type:varchar(253);index" json:"service_name"`
	ServiceNamespace string  `gorm:"column:service_namespace;type:varchar(253);index" json:"service_namespace"`
	PodUID           string  `gorm:"column:pod_uid;type:varchar(64);index" json:"pod_uid"`
	PodName          string  `gorm:"column:pod_name;type:varchar(253)" json:"pod_name"`
	PodIP            string  `gorm:"column:pod_ip;type:varchar(45)" json:"pod_ip"`
	PodLabels        ExtType `gorm:"column:pod_labels;type:jsonb;default:'{}'" json:"pod_labels"`
	WorkloadID       string  `gorm:"column:workload_id;type:varchar(253);index" json:"workload_id"`
	WorkloadOwner    string  `gorm:"column:workload_owner;type:varchar(253)" json:"workload_owner"`
	WorkloadType     string  `gorm:"column:workload_type;type:varchar(64)" json:"workload_type"`
	NodeName         string  `gorm:"column:node_name;type:varchar(253)" json:"node_name"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// TableName returns the table name for ServicePodReference
func (*ServicePodReference) TableName() string {
	return TableNameServicePodReference
}

