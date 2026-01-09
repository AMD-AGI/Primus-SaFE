// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"fmt"
	"os"
	"strconv"
)

// ControlPlaneConfig contains Control Plane configuration
type ControlPlaneConfig struct {
	Postgres *ControlPlanePostgresConfig `yaml:"postgres" json:"postgres"`
}

// ControlPlanePostgresConfig contains Control Plane database configuration
type ControlPlanePostgresConfig struct {
	// Direct connection parameters
	Host     string `yaml:"host" json:"host"`
	Port     int32  `yaml:"port" json:"port"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	DBName   string `yaml:"dbName" json:"db_name"`
	SSLMode  string `yaml:"sslMode" json:"ssl_mode"`

	// Alternative: read from environment variable
	PasswordEnv string `yaml:"passwordEnv" json:"password_env,omitempty"`

	// Alternative: use K8S Service name (for in-cluster deployment)
	Service   string `yaml:"service" json:"service,omitempty"`
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`

	// Connection pool settings
	MaxIdleConn int `yaml:"maxIdleConn" json:"max_idle_conn,omitempty"`
	MaxOpenConn int `yaml:"maxOpenConn" json:"max_open_conn,omitempty"`
}

// GetHost returns the database host, preferring Service name if in-cluster
func (c *ControlPlanePostgresConfig) GetHost() string {
	if c.Service != "" && c.Namespace != "" {
		return fmt.Sprintf("%s.%s.svc.cluster.local", c.Service, c.Namespace)
	}
	return c.Host
}

// GetPassword returns the password from direct config or environment variable
func (c *ControlPlanePostgresConfig) GetPassword() string {
	if c.Password != "" {
		return c.Password
	}
	if c.PasswordEnv != "" {
		return os.Getenv(c.PasswordEnv)
	}
	return ""
}

// GetPort returns the database port, defaulting to 5432
func (c *ControlPlanePostgresConfig) GetPort() int {
	if c.Port == 0 {
		return 5432
	}
	return int(c.Port)
}

// GetSSLMode returns the SSL mode, defaulting to "require"
func (c *ControlPlanePostgresConfig) GetSSLMode() string {
	if c.SSLMode == "" {
		return "require"
	}
	return c.SSLMode
}

// GetMaxIdleConn returns the max idle connections, defaulting to 5
func (c *ControlPlanePostgresConfig) GetMaxIdleConn() int {
	if c.MaxIdleConn == 0 {
		return 5
	}
	return c.MaxIdleConn
}

// GetMaxOpenConn returns the max open connections, defaulting to 20
func (c *ControlPlanePostgresConfig) GetMaxOpenConn() int {
	if c.MaxOpenConn == 0 {
		return 20
	}
	return c.MaxOpenConn
}

// Validate validates the Control Plane configuration
func (c *ControlPlaneConfig) Validate() error {
	if c.Postgres == nil {
		return fmt.Errorf("control plane postgres config is required")
	}
	if c.Postgres.GetHost() == "" {
		return fmt.Errorf("control plane postgres host is required")
	}
	if c.Postgres.DBName == "" {
		return fmt.Errorf("control plane postgres dbName is required")
	}
	if c.Postgres.Username == "" {
		return fmt.Errorf("control plane postgres username is required")
	}
	return nil
}

// Environment variable names for Control Plane database configuration
const (
	EnvControlPlaneDBHost     = "CONTROL_PLANE_DB_HOST"
	EnvControlPlaneDBPort     = "CONTROL_PLANE_DB_PORT"
	EnvControlPlaneDBName     = "CONTROL_PLANE_DB_NAME"
	EnvControlPlaneDBUser     = "CONTROL_PLANE_DB_USER"
	EnvControlPlaneDBPassword = "CONTROL_PLANE_DB_PASSWORD"
	EnvControlPlaneDBSSLMode  = "CONTROL_PLANE_DB_SSL_MODE"
)

// NewControlPlaneConfigFromEnv creates a ControlPlaneConfig from environment variables
// This is the preferred method for Kubernetes deployments where secrets are mounted as env vars
func NewControlPlaneConfigFromEnv() (*ControlPlaneConfig, error) {
	host := os.Getenv(EnvControlPlaneDBHost)
	if host == "" {
		return nil, fmt.Errorf("environment variable %s is required", EnvControlPlaneDBHost)
	}

	dbName := os.Getenv(EnvControlPlaneDBName)
	if dbName == "" {
		return nil, fmt.Errorf("environment variable %s is required", EnvControlPlaneDBName)
	}

	user := os.Getenv(EnvControlPlaneDBUser)
	if user == "" {
		return nil, fmt.Errorf("environment variable %s is required", EnvControlPlaneDBUser)
	}

	password := os.Getenv(EnvControlPlaneDBPassword)
	if password == "" {
		return nil, fmt.Errorf("environment variable %s is required", EnvControlPlaneDBPassword)
	}

	portStr := os.Getenv(EnvControlPlaneDBPort)
	var port int32 = 5432
	if portStr != "" {
		p, err := strconv.ParseInt(portStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid port value: %s", portStr)
		}
		port = int32(p)
	}

	sslMode := os.Getenv(EnvControlPlaneDBSSLMode)
	if sslMode == "" {
		sslMode = "require"
	}

	return &ControlPlaneConfig{
		Postgres: &ControlPlanePostgresConfig{
			Host:     host,
			Port:     port,
			Username: user,
			Password: password,
			DBName:   dbName,
			SSLMode:  sslMode,
		},
	}, nil
}
