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

// ApplicationsStage handles the deployment of Primus-Lens applications
type ApplicationsStage struct {
	installer.BaseStage
	helmClient *installer.HelmClient
}

// NewApplicationsStage creates a new applications stage
func NewApplicationsStage(helmClient *installer.HelmClient) *ApplicationsStage {
	return &ApplicationsStage{
		helmClient: helmClient,
	}
}

func (s *ApplicationsStage) Name() string {
	return "applications"
}

func (s *ApplicationsStage) CheckPrerequisites(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) ([]string, error) {
	var missing []string

	// Check storage secret exists
	exists, err := client.SecretExists(ctx, config.Namespace, "primus-lens-storage-config")
	if err != nil {
		return nil, err
	}
	if !exists {
		missing = append(missing, "Storage configuration secret not found")
	}

	return missing, nil
}

func (s *ApplicationsStage) ShouldRun(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (bool, string, error) {
	// Check if release exists
	exists, healthy, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseApplications)
	if err != nil {
		return true, "Cannot check applications release status, will install", nil
	}

	if exists && healthy {
		return false, "Applications already installed and healthy", nil
	}

	if exists {
		return true, "Applications release exists but not healthy, will upgrade", nil
	}

	return true, "Applications not installed, will install", nil
}

func (s *ApplicationsStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Info("Deploying Primus-Lens applications...")

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"namespace":    config.Namespace,
			"storageClass": s.getStorageClass(config),
		},
		"api": map[string]interface{}{
			"enabled":  true,
			"replicas": 2,
		},
		"jobs": map[string]interface{}{
			"enabled": true,
		},
		"grafana": map[string]interface{}{
			"enabled": true,
		},
	}

	// Merge with config's merged values if available
	if config.MergedValues != nil {
		for k, v := range config.MergedValues {
			values[k] = v
		}
	}

	// Check if release exists
	exists, _, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseApplications)
	if err != nil {
		return err
	}

	if exists {
		log.Info("Upgrading applications release...")
		return s.helmClient.Upgrade(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseApplications, installer.ChartApplications, values)
	}

	log.Info("Installing applications release...")
	return s.helmClient.Install(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseApplications, installer.ChartApplications, values)
}

func (s *ApplicationsStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	log.Info("Waiting for applications to be ready...")

	// Wait for key application pods
	appLabels := []string{
		"app.kubernetes.io/name=primus-lens-api",
	}

	for _, label := range appLabels {
		if err := client.WaitForPodsWithRetry(ctx, config.Namespace, label, timeout); err != nil {
			return fmt.Errorf("failed waiting for pods with label %s: %w", label, err)
		}
	}

	log.Info("Applications are ready")
	return nil
}

func (s *ApplicationsStage) getStorageClass(config *installer.InstallConfig) string {
	if config.ManagedStorage != nil && config.ManagedStorage.StorageClass != "" {
		return config.ManagedStorage.StorageClass
	}
	if config.StorageClass != "" {
		return config.StorageClass
	}
	return installer.DefaultStorageClass
}
