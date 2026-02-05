// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ControlPlaneClientSet contains clients for control plane operations
type ControlPlaneClientSet struct {
	DB     *gorm.DB
	Facade *cpdb.ControlPlaneFacade
}

var (
	controlPlaneClientSet     *ControlPlaneClientSet
	controlPlaneClientSetOnce sync.Once
	isControlPlaneMode        bool
)

// ControlPlaneDBConfig contains database connection for control plane (parsed from secret)
type ControlPlaneDBConfig struct {
	Host     string
	Port     int
	UserName string
	Password string
	DBName   string
	SSLMode  string
}

// InitControlPlaneClient initializes the control plane client
func InitControlPlaneClient(ctx context.Context, cfg *config.Config) error {
	var initErr error
	controlPlaneClientSetOnce.Do(func() {
		isControlPlaneMode = true

		// Load DB config from secret
		dbCfg, err := loadControlPlaneDBConfigFromSecret(ctx, cfg.ControlPlane)
		if err != nil {
			initErr = errors.NewError().
				WithCode(errors.CodeInitializeError).
				WithMessage("failed to load control plane DB config from secret").
				WithError(err)
			return
		}

		// Initialize database connection
		sqlConfig := sql.DatabaseConfig{
			Host:     dbCfg.Host,
			Port:     dbCfg.Port,
			UserName: dbCfg.UserName,
			Password: dbCfg.Password,
			DBName:   dbCfg.DBName,
			SSLMode:  dbCfg.SSLMode,
		}

		db, err := sql.InitGormDB("controlplane", sqlConfig)
		if err != nil {
			initErr = errors.NewError().
				WithCode(errors.CodeInitializeError).
				WithMessage("failed to initialize control plane database").
				WithError(err)
			return
		}

		// Initialize global facade for use by other packages
		cpdb.InitControlPlaneFacade(db)

		// Create client set with facade reference
		controlPlaneClientSet = &ControlPlaneClientSet{
			DB:     db,
			Facade: cpdb.GetControlPlaneFacade(),
		}

		log.Info("Control plane client initialized successfully")
	})
	return initErr
}

// loadControlPlaneDBConfigFromSecret loads DB config from Kubernetes secret
func loadControlPlaneDBConfigFromSecret(ctx context.Context, cpCfg *config.ControlPlaneConfig) (*ControlPlaneDBConfig, error) {
	// Get K8S client from current cluster
	k8sClient := getCurrentClusterK8SClientSet()
	if k8sClient == nil || k8sClient.Clientsets == nil {
		return nil, fmt.Errorf("K8S client not initialized, cannot load control plane secret")
	}

	secretName := "primus-lens-control-plane-pguser-primus-lens-control-plane"
	secretNamespace := "primus-lens"
	if cpCfg != nil {
		secretName = cpCfg.GetSecretName()
		secretNamespace = cpCfg.GetSecretNamespace()
	}

	log.Infof("Loading control plane DB config from secret: %s/%s", secretNamespace, secretName)

	// Get the secret using client-go
	secret, err := k8sClient.Clientsets.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", secretNamespace, secretName, err)
	}

	// Parse secret data
	// PostgresCluster operator creates secrets with fields: host, port, user, password, dbname
	dbCfg := &ControlPlaneDBConfig{
		Host:     getSecretString(secret, "host", "primus-lens-control-plane-primary.primus-lens.svc.cluster.local"),
		Port:     getSecretInt(secret, "port", 5432),
		UserName: getSecretString(secret, "user", "primus-lens-control-plane"), // PostgresCluster uses "user" not "username"
		Password: getSecretString(secret, "password", ""),
		DBName:   getSecretString(secret, "dbname", "primus-lens-control-plane"),
		SSLMode:  getSecretString(secret, "sslmode", "require"), // Default to require, not in PostgresCluster secret
	}

	if dbCfg.Password == "" {
		return nil, fmt.Errorf("password not found in secret %s/%s", secretNamespace, secretName)
	}

	return dbCfg, nil
}

// getSecretString extracts a string value from secret data
func getSecretString(secret *corev1.Secret, key, defaultValue string) string {
	if secret.Data == nil {
		return defaultValue
	}
	if val, ok := secret.Data[key]; ok {
		return string(val)
	}
	return defaultValue
}

// getSecretInt extracts an int value from secret data
func getSecretInt(secret *corev1.Secret, key string, defaultValue int) int {
	if secret.Data == nil {
		return defaultValue
	}
	if val, ok := secret.Data[key]; ok {
		if i, err := strconv.Atoi(string(val)); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetControlPlaneClientSet returns the control plane client set
func GetControlPlaneClientSet() *ControlPlaneClientSet {
	return controlPlaneClientSet
}

// IsControlPlaneMode returns whether control plane mode is enabled
func IsControlPlaneMode() bool {
	return isControlPlaneMode
}

// MustGetControlPlaneFacade returns the control plane facade, panics if not initialized
func MustGetControlPlaneFacade() *cpdb.ControlPlaneFacade {
	if controlPlaneClientSet == nil || controlPlaneClientSet.Facade == nil {
		panic("control plane client not initialized")
	}
	return controlPlaneClientSet.Facade
}
