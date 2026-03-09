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

	// For upgrade mode we always run; otherwise skip if already healthy
	if exists && healthy && !config.IsUpgrade {
		return false, "Applications already installed and healthy", nil
	}

	if exists {
		return true, "Applications release exists but not healthy or upgrade requested, will upgrade", nil
	}

	return true, "Applications not installed, will install", nil
}

func (s *ApplicationsStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Info("Deploying Primus-Lens applications...")

	var values map[string]interface{}
	if len(config.MergedValues) > 0 {
		log.Infof("Using merged values from release management")
		values = config.MergedValues
		// Ensure global settings are set
		if globalVals, ok := values["global"].(map[string]interface{}); ok {
			globalVals["clusterName"] = config.ClusterName
			globalVals["namespace"] = config.Namespace
			if _, hasRegistry := globalVals["imageRegistry"]; !hasRegistry {
				globalVals["imageRegistry"] = map[string]interface{}{
					"url":        config.ImageRegistry,
					"pullPolicy": "IfNotPresent",
					"pullSecret": "",
				}
			}
		} else {
			values["global"] = map[string]interface{}{
				"clusterName": config.ClusterName,
				"namespace":   config.Namespace,
				"imageRegistry": map[string]interface{}{
					"url":        config.ImageRegistry,
					"pullPolicy": "IfNotPresent",
					"pullSecret": "",
				},
			}
		}
	} else {
		values = map[string]interface{}{
			"global": map[string]interface{}{
				"clusterName":   config.ClusterName,
				"namespace":     config.Namespace,
				"storageClass":  s.getStorageClass(config),
				"imageRegistry": map[string]interface{}{
					"url":        config.ImageRegistry,
					"pullPolicy": "IfNotPresent",
					"pullSecret": "",
				},
			},
			"telemetryCollector":  map[string]interface{}{"enabled": true},
			"jobs":                map[string]interface{}{"enabled": true},
			"nodeExporter":        map[string]interface{}{"enabled": true},
			"gpuResourceExporter": map[string]interface{}{"enabled": true},
			"systemTuner":         map[string]interface{}{"enabled": true},
			"aiAdvisor":           map[string]interface{}{"enabled": true},
		}
	}

	chartName := installer.ChartApplications
	if config.ChartVersion != "" {
		log.Infof("Using chart version: %s", config.ChartVersion)
	}

	exists, _, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseApplications)
	if err != nil {
		return err
	}

	if exists || config.IsUpgrade {
		log.Infof("Upgrading applications release")
		return s.helmClient.Upgrade(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseApplications, chartName, values)
	}
	log.Infof("Installing applications release")
	return s.helmClient.Install(ctx, client.GetKubeconfig(), config.Namespace, installer.ReleaseApplications, chartName, values)
}

func (s *ApplicationsStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	log.Info("Waiting for applications to be ready...")

	// Wait for key dataplane application pods
	appLabels := []string{
		"app=telemetry-processor",
		"app=jobs",
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
