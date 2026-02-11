// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/installer/pkg/installer"
)

// StorageSecretStage creates the storage configuration secret for applications
type StorageSecretStage struct {
	installer.BaseStage
}

// NewStorageSecretStage creates a new storage secret stage
func NewStorageSecretStage() *StorageSecretStage {
	return &StorageSecretStage{}
}

func (s *StorageSecretStage) Name() string {
	return "storage-secret"
}

func (s *StorageSecretStage) CheckPrerequisites(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) ([]string, error) {
	var missing []string

	if config.StorageMode == installer.StorageModeLensManaged {
		// Check PostgreSQL secret exists
		pgSecretName := "primus-lens-pguser-primus-lens"
		exists, err := client.SecretExists(ctx, config.Namespace, pgSecretName)
		if err != nil {
			return nil, err
		}
		if !exists {
			missing = append(missing, fmt.Sprintf("PostgreSQL secret '%s' not found", pgSecretName))
		}
	}

	return missing, nil
}

func (s *StorageSecretStage) ShouldRun(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (bool, string, error) {
	// Check if storage secret exists
	exists, err := client.SecretExists(ctx, config.Namespace, "primus-lens-storage-config")
	if err != nil {
		return true, "Cannot check storage secret, will create", nil
	}

	if exists {
		// Could check if it's up to date, but for now just skip
		return false, "Storage secret already exists", nil
	}

	return true, "Storage secret not found, will create", nil
}

func (s *StorageSecretStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Info("Creating storage configuration secret...")

	var storageConfig installer.StorageConfig

	if config.StorageMode == installer.StorageModeLensManaged {
		// Build from managed storage
		pgConfig, err := s.buildPostgresConfig(ctx, client, config)
		if err != nil {
			return fmt.Errorf("failed to build postgres config: %w", err)
		}
		storageConfig.Postgres = pgConfig

		osConfig, err := s.buildOpenSearchConfig(ctx, client, config)
		if err != nil {
			log.Warnf("Failed to build OpenSearch config: %v", err)
			// OpenSearch is optional, continue
		} else {
			storageConfig.OpenSearch = osConfig
		}

		storageConfig.Prometheus = s.buildPrometheusConfig(config)
	} else if config.ExternalStorage != nil {
		// Use external storage config
		storageConfig.Postgres = installer.PostgresConfig{
			Service:   config.ExternalStorage.PostgresHost,
			Port:      config.ExternalStorage.PostgresPort,
			Namespace: "",
			Username:  config.ExternalStorage.PostgresUsername,
			Password:  config.ExternalStorage.PostgresPassword,
			DBName:    config.ExternalStorage.PostgresDBName,
			SSLMode:   config.ExternalStorage.PostgresSSLMode,
		}
		storageConfig.OpenSearch = installer.OpenSearchConfig{
			Service:  config.ExternalStorage.OpensearchHost,
			Port:     config.ExternalStorage.OpensearchPort,
			Username: config.ExternalStorage.OpensearchUsername,
			Password: config.ExternalStorage.OpensearchPassword,
			Scheme:   config.ExternalStorage.OpensearchScheme,
		}
		storageConfig.Prometheus = installer.PrometheusConfig{
			ReadEndpoint:  fmt.Sprintf("http://%s:%d", config.ExternalStorage.PrometheusReadHost, config.ExternalStorage.PrometheusReadPort),
			WriteEndpoint: fmt.Sprintf("http://%s:%d", config.ExternalStorage.PrometheusWriteHost, config.ExternalStorage.PrometheusWritePort),
		}
	}

	// Create the secret
	secretYAML, err := s.generateSecretYAML(config.Namespace, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to generate secret YAML: %w", err)
	}

	return client.ApplyYAML(ctx, secretYAML)
}

func (s *StorageSecretStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	// Secret is created synchronously, just verify it exists
	return client.WaitForSecret(ctx, config.Namespace, "primus-lens-storage-config", timeout)
}

func (s *StorageSecretStage) buildPostgresConfig(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (installer.PostgresConfig, error) {
	secret, err := client.GetSecret(ctx, config.Namespace, "primus-lens-pguser-primus-lens")
	if err != nil {
		return installer.PostgresConfig{}, fmt.Errorf("failed to get postgres secret: %w", err)
	}

	return installer.PostgresConfig{
		Service:   fmt.Sprintf("primus-lens-primary.%s.svc.cluster.local", config.Namespace),
		Port:      5432,
		Namespace: config.Namespace,
		Username:  "primus-lens",
		Password:  string(secret.Data["password"]),
		DBName:    "primus_lens",
		SSLMode:   "require",
	}, nil
}

func (s *StorageSecretStage) buildOpenSearchConfig(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (installer.OpenSearchConfig, error) {
	secret, err := client.GetSecret(ctx, config.Namespace, "primus-lens-logs-admin-password")
	if err != nil {
		return installer.OpenSearchConfig{}, fmt.Errorf("failed to get opensearch secret: %w", err)
	}

	return installer.OpenSearchConfig{
		Service:     fmt.Sprintf("primus-lens-logs-nodes.%s.svc.cluster.local", config.Namespace),
		Port:        9200,
		Namespace:   config.Namespace,
		Username:    string(secret.Data["username"]),
		Password:    string(secret.Data["password"]),
		TLSEnabled:  true,
		TLSInsecure: true,
		Scheme:      "https",
	}, nil
}

func (s *StorageSecretStage) buildPrometheusConfig(config *installer.InstallConfig) installer.PrometheusConfig {
	return installer.PrometheusConfig{
		ReadEndpoint:  fmt.Sprintf("http://vmselect-primus-lens-vmcluster.%s.svc.cluster.local:8481/select/0/prometheus", config.Namespace),
		WriteEndpoint: fmt.Sprintf("http://vminsert-primus-lens-vmcluster.%s.svc.cluster.local:8480/insert/0/prometheus", config.Namespace),
	}
}

func (s *StorageSecretStage) generateSecretYAML(namespace string, storageConfig installer.StorageConfig) ([]byte, error) {
	configJSON, err := json.Marshal(storageConfig)
	if err != nil {
		return nil, err
	}

	yaml := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: primus-lens-storage-config
  namespace: %s
type: Opaque
stringData:
  config.json: |
    %s
`, namespace, string(configJSON))

	return []byte(yaml), nil
}
