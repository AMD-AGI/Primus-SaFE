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

// VictoriaMetricsStage handles VictoriaMetrics cluster deployment
type VictoriaMetricsStage struct {
	installer.BaseStage
	helmClient *installer.HelmClient
}

// NewVictoriaMetricsStage creates a new VictoriaMetrics stage
func NewVictoriaMetricsStage(helmClient *installer.HelmClient) *VictoriaMetricsStage {
	return &VictoriaMetricsStage{
		helmClient: helmClient,
	}
}

func (s *VictoriaMetricsStage) Name() string {
	return "infra-victoriametrics"
}

func (s *VictoriaMetricsStage) CheckPrerequisites(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) ([]string, error) {
	var missing []string

	// Check VM operator is deployed
	exists, err := client.ClusterRoleExists(ctx, "vm-operator-victoria-metrics-operator")
	if err != nil {
		return nil, fmt.Errorf("failed to check VM operator: %w", err)
	}
	if !exists {
		missing = append(missing, "VictoriaMetrics operator not installed")
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

func (s *VictoriaMetricsStage) ShouldRun(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (bool, string, error) {
	// Check if VM is enabled
	if config.ManagedStorage != nil && !config.ManagedStorage.VictoriametricsEnabled {
		return false, "VictoriaMetrics disabled in config", nil
	}

	// Check if VMCluster CR exists
	exists, err := client.CustomResourceExists(ctx, "operator.victoriametrics.com/v1beta1", "vmcluster", config.Namespace, "primus-lens-vmcluster")
	if err != nil {
		return true, "VMCluster CR not found, will create", nil
	}

	if !exists {
		return true, "VMCluster CR not found, will create", nil
	}

	return false, "VMCluster already exists", nil
}

func (s *VictoriaMetricsStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Info("Deploying VMCluster CR...")

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"namespace":    config.Namespace,
			"storageClass": s.getStorageClass(config),
		},
		"victoriametrics": map[string]interface{}{
			"enabled":         true,
			"name":            "primus-lens-vmcluster",
			"retentionPeriod": "30d",
			"vmstorage": map[string]interface{}{
				"replicas": 1,
				"size":     s.getVMSize(config),
				"resources": map[string]interface{}{
					"limits": map[string]interface{}{
						"cpu":    "1000m",
						"memory": "2Gi",
					},
					"requests": map[string]interface{}{
						"cpu":    "500m",
						"memory": "1Gi",
					},
				},
			},
			"vmselect": map[string]interface{}{
				"replicas": 1,
				"resources": map[string]interface{}{
					"limits": map[string]interface{}{
						"cpu":    "500m",
						"memory": "1Gi",
					},
					"requests": map[string]interface{}{
						"cpu":    "100m",
						"memory": "256Mi",
					},
				},
			},
			"vminsert": map[string]interface{}{
				"replicas": 1,
				"resources": map[string]interface{}{
					"limits": map[string]interface{}{
						"cpu":    "500m",
						"memory": "512Mi",
					},
					"requests": map[string]interface{}{
						"cpu":    "100m",
						"memory": "128Mi",
					},
				},
			},
		},
	}

	releaseName := "primus-lens-victoriametrics"
	log.Infof("Installing VMCluster via Helm chart %s", ChartVictoriaMetrics)

	// Check if release exists
	exists, _, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, releaseName)
	if err != nil {
		return fmt.Errorf("failed to check release status: %w", err)
	}

	if exists {
		log.Infof("Release %s exists, upgrading...", releaseName)
		return s.helmClient.Upgrade(ctx, client.GetKubeconfig(), config.Namespace, releaseName, ChartVictoriaMetrics, values)
	}

	return s.helmClient.Install(ctx, client.GetKubeconfig(), config.Namespace, releaseName, ChartVictoriaMetrics, values)
}

func (s *VictoriaMetricsStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	log.Info("Waiting for VMCluster pods to be ready...")

	// Wait for vmstorage, vmselect, vminsert pods
	labels := []string{
		"app.kubernetes.io/name=vmstorage",
		"app.kubernetes.io/name=vmselect",
		"app.kubernetes.io/name=vminsert",
	}

	for _, label := range labels {
		if err := client.WaitForPodsWithRetry(ctx, config.Namespace, label, timeout); err != nil {
			return fmt.Errorf("failed waiting for pods with label %s: %w", label, err)
		}
	}

	log.Info("VMCluster is ready")
	return nil
}

func (s *VictoriaMetricsStage) IsRequired() bool {
	return true // VM is required for metrics storage
}

func (s *VictoriaMetricsStage) getStorageClass(config *installer.InstallConfig) string {
	if config.ManagedStorage != nil && config.ManagedStorage.StorageClass != "" {
		return config.ManagedStorage.StorageClass
	}
	if config.StorageClass != "" {
		return config.StorageClass
	}
	return installer.DefaultStorageClass
}

func (s *VictoriaMetricsStage) getVMSize(config *installer.InstallConfig) string {
	if config.ManagedStorage != nil && config.ManagedStorage.VictoriametricsSize != "" {
		return config.ManagedStorage.VictoriametricsSize
	}
	return installer.DefaultVMSize
}
