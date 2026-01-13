// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"
	"gorm.io/gorm"
)

// ControlPlaneClientSet contains clients for Control Plane operations
// Unlike Data Plane, Control Plane only needs database access
type ControlPlaneClientSet struct {
	// Database connection for Control Plane data
	// Stores: users, sessions, auth_providers, system_configs, clusters metadata
	DB *gorm.DB

	// Configuration used to create this client (for debugging)
	Config *ControlPlaneConfig
}

// InitOptions contains initialization options for ClusterManager
type InitOptions struct {
	// Control Plane options
	LoadControlPlane   bool
	ControlPlaneConfig *ControlPlaneConfig

	// Data Plane options
	MultiCluster      bool
	LoadK8SClient     bool
	LoadStorageClient bool
}

// initializeControlPlane initializes Control Plane database connection
func (cm *ClusterManager) initializeControlPlane(ctx context.Context, cfg *ControlPlaneConfig) error {
	if cfg == nil || cfg.Postgres == nil {
		return fmt.Errorf("control plane postgres config is required")
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("control plane config validation failed: %w", err)
	}

	sqlConfig := sql.DatabaseConfig{
		Host:        cfg.Postgres.GetHost(),
		Port:        cfg.Postgres.GetPort(),
		UserName:    cfg.Postgres.Username,
		Password:    cfg.Postgres.GetPassword(),
		DBName:      cfg.Postgres.DBName,
		LogMode:     false,
		MaxIdleConn: cfg.Postgres.GetMaxIdleConn(),
		MaxOpenConn: cfg.Postgres.GetMaxOpenConn(),
		SSLMode:     cfg.Postgres.GetSSLMode(),
		Driver:      sql.DriverNamePostgres,
	}

	db, err := sql.InitGormDB("control-plane", sqlConfig,
		sql.WithTracingCallback(),
		sql.WithErrorStackCallback(),
		sql.WithReconnectCallback(),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize control plane DB: %w", err)
	}

	cm.controlPlane = &ControlPlaneClientSet{
		DB:     db,
		Config: cfg,
	}

	log.Infof("Control Plane database initialized successfully: host=%s, db=%s",
		cfg.Postgres.GetHost(), cfg.Postgres.DBName)

	return nil
}

// GetControlPlaneDB returns the Control Plane database connection
// Used for authentication, user management, system configuration
func (cm *ClusterManager) GetControlPlaneDB() *gorm.DB {
	if cm.controlPlane == nil {
		panic("control plane not initialized, please enable loadControlPlane option")
	}
	return cm.controlPlane.DB
}

// GetControlPlaneClientSet returns the Control Plane client set
func (cm *ClusterManager) GetControlPlaneClientSet() *ControlPlaneClientSet {
	if cm.controlPlane == nil {
		panic("control plane not initialized, please enable loadControlPlane option")
	}
	return cm.controlPlane
}

// IsControlPlaneEnabled returns whether Control Plane is enabled
func (cm *ClusterManager) IsControlPlaneEnabled() bool {
	return cm.loadControlPlane && cm.controlPlane != nil
}

// InitControlPlane initializes Control Plane after ClusterManager has been initialized
// This allows reading DB config from K8s Secret using the already-initialized K8s client
func (cm *ClusterManager) InitControlPlane(ctx context.Context, cfg *ControlPlaneConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.controlPlane != nil {
		log.Info("Control Plane already initialized, skipping")
		return nil
	}

	if err := cm.initializeControlPlane(ctx, cfg); err != nil {
		return err
	}

	cm.loadControlPlane = true
	return nil
}
