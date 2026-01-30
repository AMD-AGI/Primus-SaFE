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

// VictoriaMetricsStage handles VictoriaMetrics cluster deployment
type VictoriaMetricsStage struct {
	installer.BaseStage
}

// NewVictoriaMetricsStage creates a new VictoriaMetrics stage
func NewVictoriaMetricsStage() *VictoriaMetricsStage {
	return &VictoriaMetricsStage{}
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

	yaml, err := s.generateVMClusterYAML(config)
	if err != nil {
		return fmt.Errorf("failed to generate VMCluster YAML: %w", err)
	}

	log.Infof("Applying VMCluster CR to namespace %s", config.Namespace)
	return client.ApplyYAML(ctx, yaml)
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

func (s *VictoriaMetricsStage) generateVMClusterYAML(config *installer.InstallConfig) ([]byte, error) {
	tmpl := `apiVersion: operator.victoriametrics.com/v1beta1
kind: VMCluster
metadata:
  name: primus-lens-vmcluster
  namespace: {{ .Namespace }}
spec:
  retentionPeriod: "30d"
  replicationFactor: 1
  vmstorage:
    replicaCount: 1
    storage:
      volumeClaimTemplate:
        spec:
          storageClassName: {{ .StorageClass }}
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: {{ .VMSize }}
    resources:
      limits:
        cpu: "1000m"
        memory: "2Gi"
      requests:
        cpu: "500m"
        memory: "1Gi"
  vmselect:
    replicaCount: 1
    cacheMountPath: /cache
    resources:
      limits:
        cpu: "500m"
        memory: "1Gi"
      requests:
        cpu: "100m"
        memory: "256Mi"
  vminsert:
    replicaCount: 1
    resources:
      limits:
        cpu: "500m"
        memory: "512Mi"
      requests:
        cpu: "100m"
        memory: "128Mi"
`

	t, err := template.New("vmcluster").Parse(tmpl)
	if err != nil {
		return nil, err
	}

	data := map[string]string{
		"Namespace":    config.Namespace,
		"StorageClass": s.getStorageClass(config),
		"VMSize":       s.getVMSize(config),
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
