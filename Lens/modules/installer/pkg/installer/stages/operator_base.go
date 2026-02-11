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

// OperatorConfig defines configuration for an operator installation
type OperatorConfig struct {
	// Name is the unique identifier for this operator
	Name string

	// ChartName is the Helm chart name
	ChartName string

	// ReleaseName is the Helm release name
	ReleaseName string

	// Namespace where the operator will be installed
	Namespace string

	// DetectionClusterRole is the ClusterRole name used to detect if operator exists
	DetectionClusterRole string

	// DeploymentName is the name of the operator deployment to wait for
	DeploymentName string

	// Values is the Helm values for this operator
	Values map[string]interface{}

	// Required indicates if this operator is required for installation to continue
	Required bool
}

// OperatorStage is a generic stage for installing Kubernetes operators
type OperatorStage struct {
	installer.BaseStage
	config     OperatorConfig
	helmClient *installer.HelmClient
}

// NewOperatorStage creates a new operator stage
func NewOperatorStage(config OperatorConfig, helmClient *installer.HelmClient) *OperatorStage {
	return &OperatorStage{
		config:     config,
		helmClient: helmClient,
	}
}

func (s *OperatorStage) Name() string {
	return fmt.Sprintf("operator-%s", s.config.Name)
}

func (s *OperatorStage) CheckPrerequisites(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) ([]string, error) {
	// Operators typically don't have prerequisites
	// The namespace will be created by Helm if needed
	return nil, nil
}

func (s *OperatorStage) ShouldRun(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) (bool, string, error) {
	// Check if operator already exists by looking for its ClusterRole
	exists, err := client.ClusterRoleExists(ctx, s.config.DetectionClusterRole)
	if err != nil {
		return false, "", fmt.Errorf("failed to check ClusterRole %s: %w", s.config.DetectionClusterRole, err)
	}

	if exists {
		return false, fmt.Sprintf("operator already installed (ClusterRole '%s' exists)", s.config.DetectionClusterRole), nil
	}

	return true, "operator not found, will install", nil
}

func (s *OperatorStage) Execute(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Infof("Installing operator %s with Helm chart %s", s.config.Name, s.config.ChartName)

	// Merge default values with any custom values
	values := make(map[string]interface{})
	for k, v := range s.config.Values {
		values[k] = v
	}

	// Check if release exists
	exists, _, err := s.helmClient.ReleaseStatus(ctx, client.GetKubeconfig(), s.config.Namespace, s.config.ReleaseName)
	if err != nil {
		return fmt.Errorf("failed to check release status: %w", err)
	}

	if exists {
		log.Infof("Release %s exists, upgrading...", s.config.ReleaseName)
		return s.helmClient.Upgrade(ctx, client.GetKubeconfig(), s.config.Namespace, s.config.ReleaseName, s.config.ChartName, values)
	}

	log.Infof("Installing new release %s...", s.config.ReleaseName)
	return s.helmClient.Install(ctx, client.GetKubeconfig(), s.config.Namespace, s.config.ReleaseName, s.config.ChartName, values)
}

func (s *OperatorStage) WaitForReady(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig, timeout time.Duration) error {
	log.Infof("Waiting for operator %s deployment to be ready...", s.config.Name)

	return client.WaitForDeploymentReady(ctx, s.config.Namespace, s.config.DeploymentName, timeout)
}

func (s *OperatorStage) Rollback(ctx context.Context, client *installer.ClusterClient, config *installer.InstallConfig) error {
	log.Infof("Rolling back operator %s...", s.config.Name)

	// Uninstall the Helm release
	return s.helmClient.Uninstall(ctx, client.GetKubeconfig(), s.config.Namespace, s.config.ReleaseName)
}

func (s *OperatorStage) IsRequired() bool {
	return s.config.Required
}
