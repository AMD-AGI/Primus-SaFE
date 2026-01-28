// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameDataplaneInstallTask = "dataplane_install_tasks"

// DataplaneInstallTask represents a dataplane installation task
type DataplaneInstallTask struct {
	ID            int32             `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ClusterName   string            `gorm:"column:cluster_name;not null" json:"cluster_name"`
	TaskType      string            `gorm:"column:task_type;not null;default:install" json:"task_type"`
	CurrentStage  string            `gorm:"column:current_stage;not null;default:pending" json:"current_stage"`
	StorageMode   string            `gorm:"column:storage_mode;not null;default:external" json:"storage_mode"`
	InstallConfig InstallConfigJSON `gorm:"column:install_config;not null" json:"install_config"`
	Status        string            `gorm:"column:status;not null;default:pending" json:"status"`
	ErrorMessage  string            `gorm:"column:error_message" json:"error_message"`
	RetryCount    int               `gorm:"column:retry_count;default:0" json:"retry_count"`
	MaxRetries    int               `gorm:"column:max_retries;default:3" json:"max_retries"`
	JobName       string            `gorm:"column:job_name" json:"job_name"`       // K8s Job name for tracking
	JobNamespace  string            `gorm:"column:job_namespace" json:"job_namespace"` // K8s Job namespace
	CreatedAt     time.Time         `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time         `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	StartedAt     *time.Time        `gorm:"column:started_at" json:"started_at"`
	CompletedAt   *time.Time        `gorm:"column:completed_at" json:"completed_at"`
}

// TableName returns the table name
func (*DataplaneInstallTask) TableName() string {
	return TableNameDataplaneInstallTask
}

// Stage constants
const (
	StagePending        = "pending"
	StageOperators      = "operators"
	StageWaitOperators  = "wait_operators"
	StageInfrastructure = "infrastructure"
	StageWaitInfra      = "wait_infrastructure"
	StageInit           = "init"
	StageStorageSecret  = "storage_secret"
	StageApplications   = "applications"
	StageWaitApps       = "wait_applications"
	StageCompleted      = "completed"
)

// Task type constants
const (
	TaskTypeInstall   = "install"
	TaskTypeUpgrade   = "upgrade"
	TaskTypeUninstall = "uninstall"
	TaskTypeRollback  = "rollback"
	TaskTypeSync      = "sync"
)

// Task status constants
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
)

// Storage mode constants
const (
	StorageModeLensManaged = "lens-managed"
	StorageModeExternal    = "external"
)

// InstallConfigJSON is a custom type for JSONB install config
type InstallConfigJSON struct {
	Namespace     string `json:"namespace"`
	StorageClass  string `json:"storage_class"`
	ImageRegistry string `json:"image_registry"`

	// Lens-managed storage config
	ManagedStorage *ManagedStorageConfig `json:"managed_storage,omitempty"`

	// External storage config
	ExternalStorage *ExternalStorageConfig `json:"external_storage,omitempty"`
}

// ManagedStorageConfig for lens-managed storage
type ManagedStorageConfig struct {
	StorageClass string `json:"storage_class"`

	// PostgreSQL
	PostgresEnabled bool   `json:"postgres_enabled"`
	PostgresSize    string `json:"postgres_size"`

	// OpenSearch
	OpensearchEnabled  bool   `json:"opensearch_enabled"`
	OpensearchSize     string `json:"opensearch_size"`
	OpensearchReplicas int    `json:"opensearch_replicas"`

	// VictoriaMetrics
	VictoriametricsEnabled bool   `json:"victoriametrics_enabled"`
	VictoriametricsSize    string `json:"victoriametrics_size"`
}

// ExternalStorageConfig for external storage
type ExternalStorageConfig struct {
	// PostgreSQL
	PostgresHost     string `json:"postgres_host"`
	PostgresPort     int    `json:"postgres_port"`
	PostgresUsername string `json:"postgres_username"`
	PostgresPassword string `json:"postgres_password"`
	PostgresDBName   string `json:"postgres_db_name"`
	PostgresSSLMode  string `json:"postgres_ssl_mode"`

	// OpenSearch
	OpensearchHost     string `json:"opensearch_host"`
	OpensearchPort     int    `json:"opensearch_port"`
	OpensearchUsername string `json:"opensearch_username"`
	OpensearchPassword string `json:"opensearch_password"`
	OpensearchScheme   string `json:"opensearch_scheme"`

	// VictoriaMetrics/Prometheus
	PrometheusReadHost  string `json:"prometheus_read_host"`
	PrometheusReadPort  int    `json:"prometheus_read_port"`
	PrometheusWriteHost string `json:"prometheus_write_host"`
	PrometheusWritePort int    `json:"prometheus_write_port"`
}

// Value implements driver.Valuer interface
func (c InstallConfigJSON) Value() (driver.Value, error) {
	b, err := json.Marshal(c)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (c *InstallConfigJSON) Scan(value interface{}) error {
	if value == nil {
		*c = InstallConfigJSON{}
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
