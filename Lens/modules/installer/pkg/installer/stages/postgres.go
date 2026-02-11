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

// PostgresStage handles PostgreSQL cluster deployment using CrunchyData PGO
type PostgresStage struct {
	installer.BaseStage
	helmClient *installer.HelmClient
}

// NewPostgresStage creates a new PostgreSQL stage
func NewPostgresStage(helmClient *installer.HelmClient) *PostgresStage {
	return &PostgresStage{
		helmClient: helmClient,
	}
}

func (s *PostgresStage) Name() string {
	return "infra-postgres"
}

func (s *PostgresStage) CheckPrerequisites(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) ([]string, error) {
	var missing []string

	// Check PGO operator is deployed
	exists, err := client.ClusterRoleExists(ctx, "pgo")
	if err != nil {
		return nil, fmt.Errorf("failed to check PGO operator: %w", err)
	}
	if !exists {
		missing = append(missing, "PGO operator not installed (ClusterRole 'pgo' not found)")
	}

	// Check PGO operator deployment is ready
	ready, err := client.DeploymentReady(ctx, "postgres-operator", "pgo")
	if err != nil {
		return nil, fmt.Errorf("failed to check PGO deployment: %w", err)
	}
	if !ready {
		missing = append(missing, "PGO operator deployment not ready")
	}

	// Check StorageClass exists (if specified)
	storageClass := s.getStorageClass(config)
	if storageClass != "" && storageClass != "default" {
		scExists, err := client.StorageClassExists(ctx, storageClass)
		if err != nil {
			return nil, fmt.Errorf("failed to check StorageClass: %w", err)
		}
		if !scExists {
			missing = append(missing, fmt.Sprintf("StorageClass '%s' not found", storageClass))
		}
	}

	return missing, nil
}

func (s *PostgresStage) ShouldRun(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (bool, string, error) {
	// Check if PostgreSQL is enabled
	if config.ManagedStorage != nil && !config.ManagedStorage.PostgresEnabled {
		return false, "PostgreSQL disabled in config", nil
	}

	// Check if PostgresCluster CR exists
	exists, err := client.CustomResourceExists(ctx, "postgres-operator.crunchydata.com/v1beta1", "postgrescluster", config.Namespace, "primus-lens")
	if err != nil {
		// CRD might not exist yet, which is fine
		return true, "PostgresCluster CR not found, will create", nil
	}

	if !exists {
		return true, "PostgresCluster CR not found, will create", nil
	}

	// Check if it's healthy
	status, err := client.GetCustomResourceStatus(ctx, "postgrescluster", config.Namespace, "primus-lens", "{.status.state}")
	if err != nil {
		return true, "Cannot get PostgresCluster status, will ensure it's configured", nil
	}

	if status == "healthy" {
		return false, "PostgresCluster already exists and is healthy", nil
	}

	return true, fmt.Sprintf("PostgresCluster exists but status is '%s', will wait for it", status), nil
}

func (s *PostgresStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Info("Deploying PostgresCluster CR...")

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"namespace":    config.Namespace,
			"storageClass": s.getStorageClass(config),
		},
		"postgres": map[string]interface{}{
			"enabled":    true,
			"name":       "primus-lens",
			"version":    16,
			"dataSize":   s.getPostgresSize(config),
			"backupSize": s.getPostgresSize(config),
			"replicas":   1,
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "2000m",
					"memory": "4Gi",
				},
				"requests": map[string]interface{}{
					"cpu":    "500m",
					"memory": "2Gi",
				},
			},
			"users": []map[string]interface{}{
				{
					"name":      "primus-lens",
					"databases": []string{"primus_lens"},
					"options":   "SUPERUSER",
				},
			},
		},
	}

	releaseName := "primus-lens-postgres"
	log.Infof("Installing PostgresCluster via Helm chart %s", ChartPostgres)

	// Check if release exists
	exists, _, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, releaseName)
	if err != nil {
		return fmt.Errorf("failed to check release status: %w", err)
	}

	if exists {
		log.Infof("Release %s exists, upgrading...", releaseName)
		return s.helmClient.Upgrade(ctx, client.GetKubeconfig(), config.Namespace, releaseName, ChartPostgres, values)
	}

	return s.helmClient.Install(ctx, client.GetKubeconfig(), config.Namespace, releaseName, ChartPostgres, values)
}

func (s *PostgresStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	log.Info("Waiting for PostgresCluster to be ready...")

	deadline := time.Now().Add(timeout)
	secretName := "primus-lens-pguser-primus-lens"

	for time.Now().Before(deadline) {
		// Check PostgresCluster status
		status, err := client.GetCustomResourceStatus(ctx, "postgrescluster", config.Namespace, "primus-lens", "{.status.state}")
		if err != nil {
			log.Infof("PostgresCluster not found yet, waiting...")
			time.Sleep(10 * time.Second)
			continue
		}

		if status != "healthy" {
			log.Infof("PostgresCluster status: %s, waiting...", status)
			time.Sleep(10 * time.Second)
			continue
		}

		// Check if user secret exists (final confirmation)
		exists, err := client.SecretExists(ctx, config.Namespace, secretName)
		if err != nil {
			return fmt.Errorf("failed to check secret: %w", err)
		}
		if !exists {
			log.Infof("Waiting for secret %s to be created...", secretName)
			time.Sleep(10 * time.Second)
			continue
		}

		log.Info("PostgresCluster is ready")
		return nil
	}

	return fmt.Errorf("timeout waiting for PostgresCluster to be ready")
}

func (s *PostgresStage) IsRequired() bool {
	return true
}

func (s *PostgresStage) getStorageClass(config *installer.InstallConfig) string {
	if config.ManagedStorage != nil && config.ManagedStorage.StorageClass != "" {
		return config.ManagedStorage.StorageClass
	}
	if config.StorageClass != "" {
		return config.StorageClass
	}
	return installer.DefaultStorageClass
}

func (s *PostgresStage) getPostgresSize(config *installer.InstallConfig) string {
	if config.ManagedStorage != nil && config.ManagedStorage.PostgresSize != "" {
		return config.ManagedStorage.PostgresSize
	}
	return installer.DefaultPostgresSize
}
