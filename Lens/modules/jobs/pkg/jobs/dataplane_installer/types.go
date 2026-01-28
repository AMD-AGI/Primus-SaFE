// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dataplane_installer

import "context"

// InstallConfig contains all installation parameters
type InstallConfig struct {
	ClusterName   string
	Kubeconfig    []byte
	Namespace     string
	StorageClass  string
	StorageMode   string // "lens-managed" or "external"
	ImageRegistry string

	// Lens-managed storage config
	ManagedStorage *ManagedStorageConfig

	// External storage config
	ExternalStorage *ExternalStorageConfig
}

// ManagedStorageConfig for lens-managed storage
type ManagedStorageConfig struct {
	StorageClass string

	// PostgreSQL
	PostgresEnabled bool
	PostgresSize    string

	// OpenSearch
	OpensearchEnabled  bool
	OpensearchSize     string
	OpensearchReplicas int

	// VictoriaMetrics
	VictoriametricsEnabled bool
	VictoriametricsSize    string
}

// ExternalStorageConfig for external storage
type ExternalStorageConfig struct {
	// PostgreSQL
	PostgresHost     string
	PostgresPort     int
	PostgresUsername string
	PostgresPassword string
	PostgresDBName   string
	PostgresSSLMode  string

	// OpenSearch
	OpensearchHost     string
	OpensearchPort     int
	OpensearchUsername string
	OpensearchPassword string
	OpensearchScheme   string

	// VictoriaMetrics/Prometheus
	PrometheusReadHost  string
	PrometheusReadPort  int
	PrometheusWriteHost string
	PrometheusWritePort int
}

// Stage interface for each installation stage
type Stage interface {
	Name() string
	Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error
	IsIdempotent() bool
}

// StorageConfig for storage secret
type StorageConfig struct {
	Postgres   PostgresConfig   `json:"postgres"`
	OpenSearch OpenSearchConfig `json:"opensearch"`
	Prometheus PrometheusConfig `json:"prometheus"`
}

// PostgresConfig for postgres connection
type PostgresConfig struct {
	Service   string `json:"service"`
	Port      int    `json:"port"`
	Namespace string `json:"namespace"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	DBName    string `json:"db_name"`
	SSLMode   string `json:"ssl_mode"`
}

// OpenSearchConfig for opensearch connection
type OpenSearchConfig struct {
	Service     string `json:"service"`
	Port        int    `json:"port"`
	Namespace   string `json:"namespace"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	TLSEnabled  bool   `json:"tls_enabled"`
	TLSInsecure bool   `json:"tls_insecure"`
	Scheme      string `json:"scheme"`
}

// PrometheusConfig for prometheus/victoriametrics connection
type PrometheusConfig struct {
	ReadEndpoint  string `json:"read_endpoint"`
	WriteEndpoint string `json:"write_endpoint"`
}

// Default values
const (
	DefaultNamespace     = "primus-lens"
	DefaultStorageClass  = "local-path"
	DefaultImageRegistry = "docker.io"
	DefaultPostgresSize  = "10Gi"
	DefaultOSSize        = "10Gi"
	DefaultVMSize        = "10Gi"
	DefaultOSReplicas    = 1
)

// Helm chart names
const (
	ChartOperators      = "primus-lens-operators"
	ChartInfrastructure = "primus-lens-infrastructure"
	ChartInit           = "primus-lens-init"
	ChartApplications   = "primus-lens-apps-dataplane"
)

// Release names
const (
	ReleaseOperators      = "plo"
	ReleaseInfrastructure = "pli"
	ReleaseInit           = "pli-init"
	ReleaseApplications   = "pla"
)
