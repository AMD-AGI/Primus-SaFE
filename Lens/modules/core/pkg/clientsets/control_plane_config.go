// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"fmt"
	"os"
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
