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

// OpenSearchStage handles OpenSearch cluster deployment
type OpenSearchStage struct {
	installer.BaseStage
	helmClient *installer.HelmClient
}

// NewOpenSearchStage creates a new OpenSearch stage
func NewOpenSearchStage(helmClient *installer.HelmClient) *OpenSearchStage {
	return &OpenSearchStage{
		helmClient: helmClient,
	}
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

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"namespace":    config.Namespace,
			"storageClass": s.getStorageClass(config),
		},
		"opensearch": map[string]interface{}{
			"enabled":  true,
			"name":     "primus-lens-logs",
			"version":  "2.11.0",
			"httpPort": 9200,
			"dashboards": map[string]interface{}{
				"enabled": false,
			},
			"nodePools": []map[string]interface{}{
				{
					"name":     "nodes",
					"replicas": s.getOpenSearchReplicas(config),
					"roles":    []string{"master", "data", "ingest"},
					"diskSize": s.getOpenSearchSize(config),
					"jvm":      "-Xms1g -Xmx1g",
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu":    "500m",
							"memory": "2Gi",
						},
						"limits": map[string]interface{}{
							"cpu":    "2000m",
							"memory": "4Gi",
						},
					},
				},
			},
		},
	}

	releaseName := "primus-lens-opensearch"
	log.Infof("Installing OpenSearchCluster via Helm chart %s", ChartOpenSearch)

	// Check if release exists
	exists, _, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, releaseName)
	if err != nil {
		return fmt.Errorf("failed to check release status: %w", err)
	}

	if exists {
		log.Infof("Release %s exists, upgrading...", releaseName)
		return s.helmClient.Upgrade(ctx, client.GetKubeconfig(), config.Namespace, releaseName, ChartOpenSearch, values)
	}

	return s.helmClient.Install(ctx, client.GetKubeconfig(), config.Namespace, releaseName, ChartOpenSearch, values)
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
