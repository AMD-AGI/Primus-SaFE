// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameClusterConfig = "cluster_config"

// ClusterConfig stores cluster connection configuration
type ClusterConfig struct {
	ID            int32      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ClusterName   string     `gorm:"column:cluster_name;not null;uniqueIndex" json:"cluster_name"`
	DisplayName   string     `gorm:"column:display_name" json:"display_name"`
	Description   string     `gorm:"column:description" json:"description"`
	Source        string     `gorm:"column:source;not null;default:manual" json:"source"` // 'manual' or 'primus-safe'
	PrimusSafeID  string     `gorm:"column:primus_safe_id" json:"primus_safe_id"`

	// K8S Connection Config
	K8SEndpoint           string `gorm:"column:k8s_endpoint" json:"k8s_endpoint"`
	K8SCAData             string `gorm:"column:k8s_ca_data" json:"k8s_ca_data"`
	K8SCertData           string `gorm:"column:k8s_cert_data" json:"k8s_cert_data"`
	K8SKeyData            string `gorm:"column:k8s_key_data" json:"k8s_key_data"`
	K8SToken              string `gorm:"column:k8s_token" json:"k8s_token"`
	K8SInsecureSkipVerify bool   `gorm:"column:k8s_insecure_skip_verify;default:false" json:"k8s_insecure_skip_verify"`

	// Storage Config
	PostgresHost     string `gorm:"column:postgres_host" json:"postgres_host"`
	PostgresPort     int    `gorm:"column:postgres_port;default:5432" json:"postgres_port"`
	PostgresUsername string `gorm:"column:postgres_username" json:"postgres_username"`
	PostgresPassword string `gorm:"column:postgres_password" json:"postgres_password"`
	PostgresDBName   string `gorm:"column:postgres_db_name" json:"postgres_db_name"`
	PostgresSSLMode  string `gorm:"column:postgres_ssl_mode;default:require" json:"postgres_ssl_mode"`

	OpensearchHost     string `gorm:"column:opensearch_host" json:"opensearch_host"`
	OpensearchPort     int    `gorm:"column:opensearch_port;default:9200" json:"opensearch_port"`
	OpensearchUsername string `gorm:"column:opensearch_username" json:"opensearch_username"`
	OpensearchPassword string `gorm:"column:opensearch_password" json:"opensearch_password"`
	OpensearchScheme   string `gorm:"column:opensearch_scheme;default:https" json:"opensearch_scheme"`

	PrometheusReadHost  string `gorm:"column:prometheus_read_host" json:"prometheus_read_host"`
	PrometheusReadPort  int    `gorm:"column:prometheus_read_port;default:8481" json:"prometheus_read_port"`
	PrometheusWriteHost string `gorm:"column:prometheus_write_host" json:"prometheus_write_host"`
	PrometheusWritePort int    `gorm:"column:prometheus_write_port;default:8480" json:"prometheus_write_port"`

	// Dataplane Status
	DataplaneStatus  string     `gorm:"column:dataplane_status;default:pending" json:"dataplane_status"`
	DataplaneVersion string     `gorm:"column:dataplane_version" json:"dataplane_version"`
	DataplaneMessage string     `gorm:"column:dataplane_message" json:"dataplane_message"`
	LastDeployTime   *time.Time `gorm:"column:last_deploy_time" json:"last_deploy_time"`

	// Storage Mode
	StorageMode          string              `gorm:"column:storage_mode;default:external" json:"storage_mode"`
	ManagedStorageConfig ManagedStorageJSON  `gorm:"column:managed_storage_config;default:{}" json:"managed_storage_config"`

	// Metadata
	Status    string        `gorm:"column:status;default:active" json:"status"`
	IsDefault bool          `gorm:"column:is_default;default:false" json:"is_default"`
	Labels    ClusterLabels `gorm:"column:labels;default:{}" json:"labels"`
	CreatedAt time.Time     `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time     `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt *time.Time    `gorm:"column:deleted_at" json:"deleted_at"`
}

// TableName returns the table name
func (*ClusterConfig) TableName() string {
	return TableNameClusterConfig
}

// ClusterLabels is a custom type for JSONB labels field
type ClusterLabels map[string]string

// Value implements driver.Valuer interface
func (l ClusterLabels) Value() (driver.Value, error) {
	if l == nil {
		return "{}", nil
	}
	b, err := json.Marshal(l)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (l *ClusterLabels) Scan(value interface{}) error {
	if value == nil {
		*l = make(ClusterLabels)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, l)
	case string:
		return json.Unmarshal([]byte(v), l)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

// Dataplane status constants
const (
	DataplaneStatusPending   = "pending"
	DataplaneStatusDeploying = "deploying"
	DataplaneStatusDeployed  = "deployed"
	DataplaneStatusFailed    = "failed"
)

// ManagedStorageJSON is a custom type for JSONB managed storage config
type ManagedStorageJSON struct {
	StorageClass           string `json:"storage_class"`
	PostgresEnabled        bool   `json:"postgres_enabled"`
	PostgresSize           string `json:"postgres_size"`
	OpensearchEnabled      bool   `json:"opensearch_enabled"`
	OpensearchSize         string `json:"opensearch_size"`
	OpensearchReplicas     int    `json:"opensearch_replicas"`
	VictoriametricsEnabled bool   `json:"victoriametrics_enabled"`
	VictoriametricsSize    string `json:"victoriametrics_size"`
}

// Value implements driver.Valuer interface
func (m ManagedStorageJSON) Value() (driver.Value, error) {
	b, err := json.Marshal(m)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (m *ManagedStorageJSON) Scan(value interface{}) error {
	if value == nil {
		*m = ManagedStorageJSON{}
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, m)
	case string:
		return json.Unmarshal([]byte(v), m)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

// Cluster source constants
const (
	ClusterSourceManual     = "manual"
	ClusterSourcePrimusSafe = "primus-safe"
)

// Cluster status constants
const (
	ClusterStatusActive   = "active"
	ClusterStatusInactive = "inactive"
	ClusterStatusDeleted  = "deleted"
)
