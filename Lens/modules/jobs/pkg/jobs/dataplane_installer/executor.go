// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dataplane_installer

import (
	"context"
	"encoding/base64"
	"fmt"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
)

// DataplaneInstaller handles dataplane installation tasks
type DataplaneInstaller struct {
	helmClient *HelmClient
	stages     map[string]Stage
}

// NewDataplaneInstaller creates a new DataplaneInstaller
func NewDataplaneInstaller() *DataplaneInstaller {
	installer := &DataplaneInstaller{
		helmClient: NewHelmClient(),
		stages:     make(map[string]Stage),
	}

	// Register stages
	installer.stages[model.StageOperators] = &OperatorsStage{}
	installer.stages[model.StageWaitOperators] = &WaitOperatorsStage{}
	installer.stages[model.StageInfrastructure] = &InfrastructureStage{}
	installer.stages[model.StageWaitInfra] = &WaitInfraStage{}
	installer.stages[model.StageInit] = &InitStage{}
	installer.stages[model.StageStorageSecret] = &StorageSecretStage{}
	installer.stages[model.StageApplications] = &ApplicationsStage{}
	installer.stages[model.StageWaitApps] = &WaitAppsStage{}

	return installer
}

// Run executes pending installation tasks
func (d *DataplaneInstaller) Run(ctx context.Context) (*common.ExecutionStats, error) {
	stats := &common.ExecutionStats{}
	facade := cpdb.GetControlPlaneFacade().GetDataplaneInstallTask()

	// Get pending or running tasks
	tasks, err := facade.GetPendingTasks(ctx, 5)
	if err != nil {
		log.Errorf("Failed to get pending tasks: %v", err)
		return stats, err
	}

	if len(tasks) == 0 {
		return stats, nil
	}

	log.Infof("Found %d pending dataplane install tasks", len(tasks))

	for _, task := range tasks {
		if err := d.processTask(ctx, task); err != nil {
			log.Errorf("Failed to process task %d for cluster %s: %v", task.ID, task.ClusterName, err)
			stats.ItemsDeleted++ // Track failures
		} else {
			stats.ItemsUpdated++
		}
	}

	return stats, nil
}

// processTask processes a single installation task
func (d *DataplaneInstaller) processTask(ctx context.Context, task *model.DataplaneInstallTask) error {
	facade := cpdb.GetControlPlaneFacade()

	// Build install config from task
	config, err := d.buildInstallConfig(ctx, task)
	if err != nil {
		return facade.GetDataplaneInstallTask().MarkFailed(ctx, task.ID, fmt.Sprintf("invalid config: %v", err))
	}

	// Mark as running if pending
	if task.Status == model.TaskStatusPending {
		if err := facade.GetDataplaneInstallTask().MarkRunning(ctx, task.ID); err != nil {
			return err
		}
		// Update cluster status
		facade.GetClusterConfig().UpdateDataplaneStatus(ctx, task.ClusterName, model.DataplaneStatusDeploying, "Installation started")
	}

	// Get next stage to execute
	nextStage := d.getNextStage(task.CurrentStage, task.StorageMode)
	if nextStage == model.StageCompleted {
		return d.completeTask(ctx, task)
	}

	// Execute stage
	log.Infof("Executing stage '%s' for cluster '%s' (task %d)", nextStage, task.ClusterName, task.ID)

	stage, ok := d.stages[nextStage]
	if !ok {
		return facade.GetDataplaneInstallTask().MarkFailed(ctx, task.ID, fmt.Sprintf("unknown stage: %s", nextStage))
	}

	if err := stage.Execute(ctx, d.helmClient, config); err != nil {
		log.Errorf("Stage '%s' failed for cluster '%s': %v", nextStage, task.ClusterName, err)

		// Check if retryable
		if task.RetryCount < task.MaxRetries {
			log.Infof("Retrying task %d (attempt %d/%d)", task.ID, task.RetryCount+1, task.MaxRetries)
			return facade.GetDataplaneInstallTask().IncrementRetry(ctx, task.ID, err.Error())
		}

		// Mark as failed
		facade.GetClusterConfig().UpdateDataplaneStatus(ctx, task.ClusterName, model.DataplaneStatusFailed, err.Error())
		return facade.GetDataplaneInstallTask().MarkFailed(ctx, task.ID, err.Error())
	}

	log.Infof("Stage '%s' completed for cluster '%s'", nextStage, task.ClusterName)

	// Update to next stage
	return facade.GetDataplaneInstallTask().UpdateStage(ctx, task.ID, nextStage)
}

// buildInstallConfig builds InstallConfig from task and cluster config
func (d *DataplaneInstaller) buildInstallConfig(ctx context.Context, task *model.DataplaneInstallTask) (*InstallConfig, error) {
	// Get cluster config
	clusterConfig, err := cpdb.GetControlPlaneFacade().GetClusterConfig().GetByName(ctx, task.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("cluster not found: %w", err)
	}

	// Build kubeconfig from cluster config
	kubeconfig, err := buildKubeconfig(clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	config := &InstallConfig{
		ClusterName:   task.ClusterName,
		Kubeconfig:    kubeconfig,
		Namespace:     task.InstallConfig.Namespace,
		StorageClass:  task.InstallConfig.StorageClass,
		StorageMode:   task.StorageMode,
		ImageRegistry: task.InstallConfig.ImageRegistry,
	}

	// Apply defaults
	if config.Namespace == "" {
		config.Namespace = DefaultNamespace
	}
	if config.StorageClass == "" {
		config.StorageClass = DefaultStorageClass
	}
	if config.ImageRegistry == "" {
		config.ImageRegistry = DefaultImageRegistry
	}

	// Copy storage config
	if task.InstallConfig.ManagedStorage != nil {
		config.ManagedStorage = &ManagedStorageConfig{
			StorageClass:           task.InstallConfig.ManagedStorage.StorageClass,
			PostgresEnabled:        task.InstallConfig.ManagedStorage.PostgresEnabled,
			PostgresSize:           task.InstallConfig.ManagedStorage.PostgresSize,
			OpensearchEnabled:      task.InstallConfig.ManagedStorage.OpensearchEnabled,
			OpensearchSize:         task.InstallConfig.ManagedStorage.OpensearchSize,
			OpensearchReplicas:     task.InstallConfig.ManagedStorage.OpensearchReplicas,
			VictoriametricsEnabled: task.InstallConfig.ManagedStorage.VictoriametricsEnabled,
			VictoriametricsSize:    task.InstallConfig.ManagedStorage.VictoriametricsSize,
		}
	}

	if task.InstallConfig.ExternalStorage != nil {
		config.ExternalStorage = &ExternalStorageConfig{
			PostgresHost:        task.InstallConfig.ExternalStorage.PostgresHost,
			PostgresPort:        task.InstallConfig.ExternalStorage.PostgresPort,
			PostgresUsername:    task.InstallConfig.ExternalStorage.PostgresUsername,
			PostgresPassword:    task.InstallConfig.ExternalStorage.PostgresPassword,
			PostgresDBName:      task.InstallConfig.ExternalStorage.PostgresDBName,
			PostgresSSLMode:     task.InstallConfig.ExternalStorage.PostgresSSLMode,
			OpensearchHost:      task.InstallConfig.ExternalStorage.OpensearchHost,
			OpensearchPort:      task.InstallConfig.ExternalStorage.OpensearchPort,
			OpensearchUsername:  task.InstallConfig.ExternalStorage.OpensearchUsername,
			OpensearchPassword:  task.InstallConfig.ExternalStorage.OpensearchPassword,
			OpensearchScheme:    task.InstallConfig.ExternalStorage.OpensearchScheme,
			PrometheusReadHost:  task.InstallConfig.ExternalStorage.PrometheusReadHost,
			PrometheusReadPort:  task.InstallConfig.ExternalStorage.PrometheusReadPort,
			PrometheusWriteHost: task.InstallConfig.ExternalStorage.PrometheusWriteHost,
			PrometheusWritePort: task.InstallConfig.ExternalStorage.PrometheusWritePort,
		}
	}

	return config, nil
}

// getNextStage returns the next stage based on current stage and storage mode
func (d *DataplaneInstaller) getNextStage(current, storageMode string) string {
	if storageMode == model.StorageModeLensManaged {
		// Full sequence for lens-managed storage
		switch current {
		case model.StagePending:
			return model.StageOperators
		case model.StageOperators:
			return model.StageWaitOperators
		case model.StageWaitOperators:
			return model.StageInfrastructure
		case model.StageInfrastructure:
			return model.StageWaitInfra
		case model.StageWaitInfra:
			return model.StageInit
		case model.StageInit:
			return model.StageStorageSecret
		case model.StageStorageSecret:
			return model.StageApplications
		case model.StageApplications:
			return model.StageWaitApps
		case model.StageWaitApps:
			return model.StageCompleted
		}
	} else {
		// Shortened sequence for external storage
		switch current {
		case model.StagePending:
			return model.StageInit
		case model.StageInit:
			return model.StageStorageSecret
		case model.StageStorageSecret:
			return model.StageApplications
		case model.StageApplications:
			return model.StageWaitApps
		case model.StageWaitApps:
			return model.StageCompleted
		}
	}
	return model.StageCompleted
}

// completeTask marks a task as completed and updates cluster status
func (d *DataplaneInstaller) completeTask(ctx context.Context, task *model.DataplaneInstallTask) error {
	facade := cpdb.GetControlPlaneFacade()

	log.Infof("Dataplane installation completed for cluster '%s'", task.ClusterName)

	// Mark task as completed
	if err := facade.GetDataplaneInstallTask().MarkCompleted(ctx, task.ID); err != nil {
		return err
	}

	// Update cluster_config status
	return facade.GetClusterConfig().UpdateDataplaneStatus(
		ctx, task.ClusterName, model.DataplaneStatusDeployed, "Installation completed successfully",
	)
}

// buildKubeconfig builds a kubeconfig from cluster config
func buildKubeconfig(config *model.ClusterConfig) ([]byte, error) {
	if config.K8SEndpoint == "" {
		return nil, fmt.Errorf("k8s endpoint is empty")
	}

	// Build kubeconfig YAML
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: %s
  cluster:
    server: %s
    certificate-authority-data: %s
users:
- name: %s-user
  user:
`, config.ClusterName, config.K8SEndpoint, config.K8SCAData, config.ClusterName)

	// Add auth method
	if config.K8SToken != "" {
		kubeconfig += fmt.Sprintf("    token: %s\n", config.K8SToken)
	} else if config.K8SCertData != "" && config.K8SKeyData != "" {
		kubeconfig += fmt.Sprintf("    client-certificate-data: %s\n", config.K8SCertData)
		kubeconfig += fmt.Sprintf("    client-key-data: %s\n", config.K8SKeyData)
	} else {
		return nil, fmt.Errorf("no valid authentication method found")
	}

	kubeconfig += fmt.Sprintf(`contexts:
- name: %s
  context:
    cluster: %s
    user: %s-user
current-context: %s
`, config.ClusterName, config.ClusterName, config.ClusterName, config.ClusterName)

	return []byte(kubeconfig), nil
}

// encodeBase64 encodes a string to base64
func encodeBase64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
