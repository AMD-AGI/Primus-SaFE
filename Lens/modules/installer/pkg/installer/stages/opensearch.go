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

// OpenSearchStage handles OpenSearch cluster deployment
type OpenSearchStage struct {
	installer.BaseStage
}

// NewOpenSearchStage creates a new OpenSearch stage
func NewOpenSearchStage() *OpenSearchStage {
	return &OpenSearchStage{}
}

func (s *OpenSearchStage) Name() string {
	return "infra-opensearch"
}

func (s *OpenSearchStage) CheckPrerequisites(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) ([]string, error) {
	var missing []string

	// Check OpenSearch operator is deployed
	exists, err := client.ClusterRoleExists(ctx, "opensearch-operator-manager-role")
	if err != nil {
		return nil, fmt.Errorf("failed to check OpenSearch operator: %w", err)
	}
	if !exists {
		missing = append(missing, "OpenSearch operator not installed")
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

func (s *OpenSearchStage) ShouldRun(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (bool, string, error) {
	// Check if OpenSearch is enabled
	if config.ManagedStorage != nil && !config.ManagedStorage.OpensearchEnabled {
		return false, "OpenSearch disabled in config", nil
	}

	// Check if OpenSearchCluster CR exists
	exists, err := client.CustomResourceExists(ctx, "opensearch.opster.io/v1", "opensearchcluster", config.Namespace, "primus-lens-logs")
	if err != nil {
		return true, "OpenSearchCluster CR not found, will create", nil
	}

	if !exists {
		return true, "OpenSearchCluster CR not found, will create", nil
	}

	return false, "OpenSearchCluster already exists", nil
}

func (s *OpenSearchStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Info("Deploying OpenSearchCluster CR...")

	yaml, err := s.generateOpenSearchClusterYAML(config)
	if err != nil {
		return fmt.Errorf("failed to generate OpenSearchCluster YAML: %w", err)
	}

	log.Infof("Applying OpenSearchCluster CR to namespace %s", config.Namespace)
	return client.ApplyYAML(ctx, yaml)
}

func (s *OpenSearchStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	log.Info("Waiting for OpenSearch pods to be ready...")

	label := "opensearch.cluster.opensearch.org/cluster-name=primus-lens-logs"
	if err := client.WaitForPodsWithRetry(ctx, config.Namespace, label, timeout); err != nil {
		return fmt.Errorf("failed waiting for OpenSearch pods: %w", err)
	}

	log.Info("OpenSearchCluster is ready")
	return nil
}

func (s *OpenSearchStage) IsRequired() bool {
	return false // OpenSearch is optional
}

func (s *OpenSearchStage) getStorageClass(config *installer.InstallConfig) string {
	if config.ManagedStorage != nil && config.ManagedStorage.StorageClass != "" {
		return config.ManagedStorage.StorageClass
	}
	if config.StorageClass != "" {
		return config.StorageClass
	}
	return installer.DefaultStorageClass
}

func (s *OpenSearchStage) getOpenSearchSize(config *installer.InstallConfig) string {
	if config.ManagedStorage != nil && config.ManagedStorage.OpensearchSize != "" {
		return config.ManagedStorage.OpensearchSize
	}
	return installer.DefaultOSSize
}

func (s *OpenSearchStage) getOpenSearchReplicas(config *installer.InstallConfig) int {
	if config.ManagedStorage != nil && config.ManagedStorage.OpensearchReplicas > 0 {
		return config.ManagedStorage.OpensearchReplicas
	}
	return installer.DefaultOSReplicas
}

func (s *OpenSearchStage) generateOpenSearchClusterYAML(config *installer.InstallConfig) ([]byte, error) {
	tmpl := `apiVersion: opensearch.opster.io/v1
kind: OpenSearchCluster
metadata:
  name: primus-lens-logs
  namespace: {{ .Namespace }}
spec:
  general:
    version: "2.11.0"
    serviceName: primus-lens-logs
    httpPort: 9200
    setVMMaxMapCount: true
  dashboards:
    enable: false
  security:
    config:
      adminCredentialsSecret:
        name: primus-lens-logs-admin-password
    tls:
      transport:
        generate: true
      http:
        generate: true
  nodePools:
    - component: nodes
      replicas: {{ .Replicas }}
      diskSize: {{ .OpenSearchSize }}
      persistence:
        storageClass: {{ .StorageClass }}
      resources:
        requests:
          cpu: "500m"
          memory: "2Gi"
        limits:
          cpu: "2000m"
          memory: "4Gi"
      roles:
        - master
        - data
        - ingest
      jvm: "-Xms1g -Xmx1g"
---
apiVersion: v1
kind: Secret
metadata:
  name: primus-lens-logs-admin-password
  namespace: {{ .Namespace }}
type: Opaque
stringData:
  username: admin
  password: Primus-Lens-2024!
`

	t, err := template.New("opensearch").Parse(tmpl)
	if err != nil {
		return nil, err
	}

	data := map[string]interface{}{
		"Namespace":      config.Namespace,
		"StorageClass":   s.getStorageClass(config),
		"OpenSearchSize": s.getOpenSearchSize(config),
		"Replicas":       s.getOpenSearchReplicas(config),
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
