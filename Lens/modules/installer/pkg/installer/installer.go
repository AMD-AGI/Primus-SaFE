// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package installer

import (
	"context"
	"fmt"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// DataplaneInstaller handles dataplane installation with real-time status updates
type DataplaneInstaller struct {
	facade            *cpdb.ControlPlaneFacade
	helmClient        *HelmClient
	stageListProvider StageListProvider
}

// installerStageReporter implements StageReporter to persist stage progress to the control plane DB
type installerStageReporter struct {
	d      *DataplaneInstaller
	taskID int32
}

func (r *installerStageReporter) OnStageStart(ctx context.Context, stageName string) {
	if err := r.d.updateStage(ctx, r.taskID, stageName, ""); err != nil {
		log.Warnf("Failed to update stage in DB: %v", err)
	}
}

func (r *installerStageReporter) OnStageEnd(ctx context.Context, stageName string, result StageResult) {
	if result.Status != StageStatusFailed || result.Error == nil {
		return
	}
	if err := r.d.updateStage(ctx, r.taskID, stageName, result.Error.Error()); err != nil {
		log.Warnf("Failed to update stage error in DB: %v", err)
	}
}

// StageListProvider returns the list of stages for the new pipeline (implemented by the stages package to avoid import cycle).
type StageListProvider interface {
	GetStagesByScope(scope string) []StageV2
	GetAppsStages() []StageV2
	GetMappedStageName(currentStage string) string
}

// NewDataplaneInstaller creates a new DataplaneInstaller with control plane DB connection
func NewDataplaneInstaller(facade *cpdb.ControlPlaneFacade) *DataplaneInstaller {
	return &DataplaneInstaller{
		facade:     facade,
		helmClient: NewHelmClient(),
	}
}

// GetHelmClient returns the Helm client for use by StageListProvider (e.g. in cmd when building stage list).
func (d *DataplaneInstaller) GetHelmClient() *HelmClient {
	return d.helmClient
}

// SetStageListProvider sets the provider for the new pipeline (must be set by cmd using stages package).
func (d *DataplaneInstaller) SetStageListProvider(p StageListProvider) {
	d.stageListProvider = p
}

func (d *DataplaneInstaller) getStageListProvider() StageListProvider {
	return d.stageListProvider
}

// ExecuteTask runs the installation/upgrade/rollback for a task
func (d *DataplaneInstaller) ExecuteTask(ctx context.Context, task *model.DataplaneInstallTask, clusterConfig *model.ClusterConfig) error {
	log.Infof("Executing task %d for cluster '%s', type: %s", task.ID, task.ClusterName, task.TaskType)

	// Check if there's a linked release history
	var releaseHistory *model.ReleaseHistory
	var releaseVersion *model.ReleaseVersion
	var mergedValues model.ValuesJSON

	// Try to find the release history linked to this task
	histories, err := d.facade.GetReleaseHistory().ListByCluster(ctx, task.ClusterName, 1)
	if err == nil && len(histories) > 0 {
		// Find the history that matches this task
		for _, h := range histories {
			if h.TaskID != nil && *h.TaskID == task.ID {
				releaseHistory = h
				break
			}
		}
	}

	if releaseHistory != nil {
		log.Infof("Found linked release history %d, using values from release management", releaseHistory.ID)
		mergedValues = releaseHistory.ValuesSnapshot

		// Get the release version for chart/image info
		releaseVersion, err = d.facade.GetReleaseVersion().GetByID(ctx, releaseHistory.ReleaseVersionID)
		if err != nil {
			log.Warnf("Failed to get release version: %v, falling back to task config", err)
			releaseVersion = nil
		}

		// Update release history status
		if err := d.facade.GetReleaseHistory().MarkRunning(ctx, releaseHistory.ID); err != nil {
			log.Warnf("Failed to mark release history as running: %v", err)
		}
	}

	// Build install config
	config, err := d.buildInstallConfig(task, clusterConfig, releaseVersion, mergedValues)
	if err != nil {
		return fmt.Errorf("failed to build install config: %w", err)
	}

	// Get stage list and run via Executor (new pipeline). StageListProvider is passed from cmd (stages package).
	provider := d.getStageListProvider()
	if provider == nil {
		return fmt.Errorf("StageListProvider not set; install pipeline requires provider from stages package")
	}
	var stageList []StageV2
	if task.TaskType == model.TaskTypeUpgrade || task.TaskType == model.TaskTypeRollback {
		config.IsUpgrade = true
		stageList = provider.GetAppsStages()
	} else {
		stageList = provider.GetStagesByScope(task.InstallScope)
	}

	exec, err := NewExecutor(config.Kubeconfig, config)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}
	exec.SetReporter(&installerStageReporter{d: d, taskID: task.ID})

	startStage := provider.GetMappedStageName(task.CurrentStage)
	var results []StageResult
	var executeErr error
	if startStage != "" {
		results, executeErr = exec.ExecuteWithResume(ctx, stageList, startStage)
	} else {
		results, executeErr = exec.ExecuteStages(ctx, stageList)
	}

	stagesCompleted := make([]string, 0, len(results))
	for _, r := range results {
		if r.Status == StageStatusCompleted {
			stagesCompleted = append(stagesCompleted, r.Stage)
		}
	}
	if executeErr != nil {
		if releaseHistory != nil {
			if err := d.facade.GetReleaseHistory().MarkFailed(ctx, releaseHistory.ID, executeErr.Error(), stagesCompleted); err != nil {
				log.Errorf("Failed to mark release history as failed: %v", err)
			}
		}
		return executeErr
	}

	// Update release history on success
	if releaseHistory != nil {
		if err := d.facade.GetReleaseHistory().MarkCompleted(ctx, releaseHistory.ID, stagesCompleted); err != nil {
			log.Errorf("Failed to mark release history as completed: %v", err)
		}
		if releaseVersion != nil {
			if err := d.facade.GetClusterReleaseConfig().MarkDeployed(ctx, task.ClusterName, releaseVersion.ID, mergedValues); err != nil {
				log.Errorf("Failed to update cluster release config: %v", err)
			}
		}
	}

	return nil
}

// Execute runs the full installation process for a task (legacy method for backward compatibility)
func (d *DataplaneInstaller) Execute(ctx context.Context, task *model.DataplaneInstallTask, clusterConfig *model.ClusterConfig) error {
	return d.ExecuteTask(ctx, task, clusterConfig)
}

// buildInstallConfig builds InstallConfig from task, cluster config, and optional release version
func (d *DataplaneInstaller) buildInstallConfig(task *model.DataplaneInstallTask, clusterConfig *model.ClusterConfig, releaseVersion *model.ReleaseVersion, mergedValues model.ValuesJSON) (*InstallConfig, error) {
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

	// If we have release version, use its chart/image info
	if releaseVersion != nil {
		config.ChartRepo = releaseVersion.ChartRepo
		config.ChartVersion = releaseVersion.ChartVersion
		config.ImageRegistry = releaseVersion.ImageRegistry
		config.ImageTag = releaseVersion.ImageTag
	}

	// If we have merged values, extract config from them
	if mergedValues != nil {
		if globalVals, ok := mergedValues["global"].(map[string]interface{}); ok {
			if ns, ok := globalVals["namespace"].(string); ok && ns != "" {
				config.Namespace = ns
			}
			if sc, ok := globalVals["storageClass"].(string); ok && sc != "" {
				config.StorageClass = sc
			}
		}
		config.MergedValues = mergedValues
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

	// Copy managed storage config from task
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

	// Copy external storage config from task
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

// updateStage updates the current stage and optional error message in DB
func (d *DataplaneInstaller) updateStage(ctx context.Context, taskID int32, stage, errorMsg string) error {
	taskFacade := d.facade.GetDataplaneInstallTask()

	if errorMsg != "" {
		// Use a custom update with error message
		return taskFacade.UpdateStageWithError(ctx, taskID, stage, errorMsg)
	}
	return taskFacade.UpdateStage(ctx, taskID, stage)
}

// buildKubeconfig builds a kubeconfig from cluster config
func buildKubeconfig(config *model.ClusterConfig) ([]byte, error) {
	if config.K8SEndpoint == "" {
		return nil, fmt.Errorf("k8s endpoint is empty")
	}

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
