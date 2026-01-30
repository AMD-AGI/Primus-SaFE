// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stages

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/installer/pkg/installer"
)

// PostgresStage handles PostgreSQL cluster deployment using CrunchyData PGO
type PostgresStage struct {
	installer.BaseStage
}

// NewPostgresStage creates a new PostgreSQL stage
func NewPostgresStage() *PostgresStage {
	return &PostgresStage{}
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

	// Generate PostgresCluster YAML
	yaml, err := s.generatePostgresClusterYAML(config)
	if err != nil {
		return fmt.Errorf("failed to generate PostgresCluster YAML: %w", err)
	}

	log.Infof("Applying PostgresCluster CR to namespace %s", config.Namespace)
	return client.ApplyYAML(ctx, yaml)
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

func (s *PostgresStage) generatePostgresClusterYAML(config *installer.InstallConfig) ([]byte, error) {
	tmpl := `apiVersion: postgres-operator.crunchydata.com/v1beta1
kind: PostgresCluster
metadata:
  name: primus-lens
  namespace: {{ .Namespace }}
spec:
  postgresVersion: 16
  instances:
    - name: instance1
      replicas: 1
      dataVolumeClaimSpec:
        storageClassName: {{ .StorageClass }}
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: {{ .PostgresSize }}
      resources:
        limits:
          cpu: "2000m"
          memory: "4Gi"
        requests:
          cpu: "500m"
          memory: "2Gi"
  backups:
    pgbackrest:
      global:
        repo1-retention-full: "14"
        repo1-retention-full-type: time
      repos:
        - name: repo1
          schedules:
            full: "0 2 * * 0"
            differential: "0 2 * * 1-6"
          volume:
            volumeClaimSpec:
              storageClassName: {{ .StorageClass }}
              accessModes:
                - ReadWriteOnce
              resources:
                requests:
                  storage: {{ .PostgresSize }}
  patroni:
    dynamicConfiguration:
      postgresql:
        parameters:
          max_connections: "200"
          shared_buffers: "256MB"
          effective_cache_size: "1GB"
  users:
    - name: primus-lens
      databases:
        - primus_lens
      options: "SUPERUSER"
`

	t, err := template.New("postgres").Parse(tmpl)
	if err != nil {
		return nil, err
	}

	data := map[string]string{
		"Namespace":    config.Namespace,
		"StorageClass": s.getStorageClass(config),
		"PostgresSize": s.getPostgresSize(config),
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
