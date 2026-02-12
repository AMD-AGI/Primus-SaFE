// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stages

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/installer/pkg/installer"
)

// DatabaseInitStage handles database initialization
type DatabaseInitStage struct {
	installer.BaseStage
	helmClient *installer.HelmClient
}

// NewDatabaseInitStage creates a new database init stage
func NewDatabaseInitStage(helmClient *installer.HelmClient) *DatabaseInitStage {
	return &DatabaseInitStage{
		helmClient: helmClient,
	}
}

func (s *DatabaseInitStage) Name() string {
	return "database-init"
}

func (s *DatabaseInitStage) CheckPrerequisites(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) ([]string, error) {
	var missing []string

	// Skip for external storage mode
	if config.StorageMode == installer.StorageModeExternal {
		return nil, nil
	}

	// Check PostgreSQL secret exists
	secretName := "primus-lens-pguser-primus-lens"
	exists, err := client.SecretExists(ctx, config.Namespace, secretName)
	if err != nil {
		return nil, fmt.Errorf("failed to check PostgreSQL secret: %w", err)
	}
	if !exists {
		missing = append(missing, fmt.Sprintf("PostgreSQL secret '%s' not found", secretName))
	}

	return missing, nil
}

func (s *DatabaseInitStage) ShouldRun(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (bool, string, error) {
	// Skip for external storage mode
	if config.StorageMode == installer.StorageModeExternal {
		return false, "External storage mode, assuming DB is pre-configured", nil
	}

	// Check if init release exists
	exists, _, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseInit)
	if err != nil {
		return true, "Cannot check init release status, will run", nil
	}

	if exists {
		return false, "Init already run", nil
	}

	return true, "Init not run yet", nil
}

func (s *DatabaseInitStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Info("Running database initialization...")

	// Get PostgreSQL password from secret
	secret, err := client.GetSecret(ctx, config.Namespace, "primus-lens-pguser-primus-lens")
	if err != nil {
		return fmt.Errorf("failed to get PostgreSQL secret: %w", err)
	}

	password := string(secret.Data["password"])
	host := fmt.Sprintf("primus-lens-primary.%s.svc.cluster.local", config.Namespace)

	values := map[string]interface{}{
		"postgres": map[string]interface{}{
			"host":     host,
			"port":     5432,
			"username": "primus-lens",
			"password": password,
			"database": "primus_lens",
			"sslMode":  "require",
		},
	}

	return s.helmClient.Install(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseInit, installer.ChartInit, values)
}

func (s *DatabaseInitStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	// The init job should complete quickly
	log.Info("Waiting for database init job to complete...")

	// Wait for the job to complete
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		exists, healthy, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseInit)
		if err != nil {
			return err
		}
		if exists && healthy {
			log.Info("Database init completed")
			return nil
		}

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("timeout waiting for database init to complete")
}

// DatabaseMigrationStage handles database migrations
type DatabaseMigrationStage struct {
	installer.BaseStage
	helmClient *installer.HelmClient
}

// NewDatabaseMigrationStage creates a new database migration stage
func NewDatabaseMigrationStage(helmClient *installer.HelmClient) *DatabaseMigrationStage {
	return &DatabaseMigrationStage{
		helmClient: helmClient,
	}
}

func (s *DatabaseMigrationStage) Name() string {
	return "database-migration"
}

func (s *DatabaseMigrationStage) CheckPrerequisites(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) ([]string, error) {
	var missing []string

	// For lens-managed storage, check init has run
	if config.StorageMode == installer.StorageModeLensManaged {
		exists, _, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseInit)
		if err != nil {
			return nil, err
		}
		if !exists {
			missing = append(missing, "Database init has not run")
		}
	}

	return missing, nil
}

func (s *DatabaseMigrationStage) ShouldRun(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (bool, string, error) {
	// Always run migrations to ensure schema is up to date
	return true, "Will run database migrations", nil
}

func (s *DatabaseMigrationStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Info("Running database migrations...")

	var host, user, password, dbName, sslMode string
	var port int

	if config.StorageMode == installer.StorageModeLensManaged {
		// Get credentials from secret
		secret, err := client.GetSecret(ctx, config.Namespace, "primus-lens-pguser-primus-lens")
		if err != nil {
			return fmt.Errorf("failed to get PostgreSQL secret: %w", err)
		}

		password = string(secret.Data["password"])
		host = fmt.Sprintf("primus-lens-primary.%s.svc.cluster.local", config.Namespace)
		port = 5432
		user = "primus-lens"
		dbName = "primus_lens"
		sslMode = "require"
	} else if config.ExternalStorage != nil {
		host = config.ExternalStorage.PostgresHost
		port = config.ExternalStorage.PostgresPort
		user = config.ExternalStorage.PostgresUsername
		password = config.ExternalStorage.PostgresPassword
		dbName = config.ExternalStorage.PostgresDBName
		sslMode = config.ExternalStorage.PostgresSSLMode
	} else {
		return fmt.Errorf("no storage configuration available")
	}

	// Run migrations using migrate CLI
	// This is typically done via a Job or direct execution
	log.Infof("Database migration config: host=%s, port=%d, user=%s, db=%s", host, port, user, dbName)

	// For now, we'll skip actual migration execution as it requires the migrate binary
	// In production, this would run the actual migrations
	_ = sslMode
	_ = password

	log.Info("Database migrations completed (placeholder)")
	return nil
}

func (s *DatabaseMigrationStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	// Migrations run synchronously, no wait needed
	return nil
}
