// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dataplane_installer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ===== Operators Stage =====

type OperatorsStage struct{}

func (s *OperatorsStage) Name() string      { return model.StageOperators }
func (s *OperatorsStage) IsIdempotent() bool { return true }

func (s *OperatorsStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Check if already installed
	exists, healthy, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseOperators)
	if err != nil {
		return err
	}

	if exists && healthy {
		log.Infof("Operators already installed and healthy, skipping")
		return nil
	}

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"namespace": config.Namespace,
		},
	}

	if exists {
		return helm.Upgrade(ctx, config.Kubeconfig, config.Namespace, ReleaseOperators, ChartOperators, values)
	}
	return helm.Install(ctx, config.Kubeconfig, config.Namespace, ReleaseOperators, ChartOperators, values)
}

// ===== Wait Operators Stage =====

type WaitOperatorsStage struct{}

func (s *WaitOperatorsStage) Name() string      { return model.StageWaitOperators }
func (s *WaitOperatorsStage) IsIdempotent() bool { return true }

func (s *WaitOperatorsStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Wait for operator pods to be ready
	return helm.WaitForPods(ctx, config.Kubeconfig, config.Namespace, "app.kubernetes.io/instance="+ReleaseOperators, 5*time.Minute)
}

// ===== Infrastructure Stage =====

type InfrastructureStage struct{}

func (s *InfrastructureStage) Name() string      { return model.StageInfrastructure }
func (s *InfrastructureStage) IsIdempotent() bool { return true }

func (s *InfrastructureStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Check if already installed
	exists, healthy, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseInfrastructure)
	if err != nil {
		return err
	}

	if exists && healthy {
		log.Infof("Infrastructure already installed and healthy, skipping")
		return nil
	}

	// Build values from managed storage config
	managed := config.ManagedStorage
	if managed == nil {
		managed = &ManagedStorageConfig{
			StorageClass:           config.StorageClass,
			PostgresEnabled:        true,
			PostgresSize:           DefaultPostgresSize,
			OpensearchEnabled:      true,
			OpensearchSize:         DefaultOSSize,
			OpensearchReplicas:     DefaultOSReplicas,
			VictoriametricsEnabled: true,
			VictoriametricsSize:    DefaultVMSize,
		}
	}

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"clusterName":  config.ClusterName,
			"namespace":    config.Namespace,
			"storageClass": managed.StorageClass,
		},
		"postgres": map[string]interface{}{
			"enabled":   managed.PostgresEnabled,
			"instances": 1,
			"storage": map[string]interface{}{
				"size": managed.PostgresSize,
			},
		},
		"victoriametrics": map[string]interface{}{
			"enabled": managed.VictoriametricsEnabled,
			"storage": map[string]interface{}{
				"size": managed.VictoriametricsSize,
			},
		},
		"opensearch": map[string]interface{}{
			"enabled":  managed.OpensearchEnabled,
			"replicas": managed.OpensearchReplicas,
			"storage": map[string]interface{}{
				"size": managed.OpensearchSize,
			},
		},
		"grafana": map[string]interface{}{
			"enabled": false,
		},
	}

	if exists {
		return helm.Upgrade(ctx, config.Kubeconfig, config.Namespace, ReleaseInfrastructure, ChartInfrastructure, values)
	}
	return helm.Install(ctx, config.Kubeconfig, config.Namespace, ReleaseInfrastructure, ChartInfrastructure, values)
}

// ===== Wait Infrastructure Stage =====

type WaitInfraStage struct{}

func (s *WaitInfraStage) Name() string      { return model.StageWaitInfra }
func (s *WaitInfraStage) IsIdempotent() bool { return true }

func (s *WaitInfraStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Wait for infrastructure pods - this may take a while
	// First wait for postgres
	if err := helm.WaitForPods(ctx, config.Kubeconfig, config.Namespace, "cluster-name=primus-lens", 10*time.Minute); err != nil {
		log.Warnf("Postgres pods not ready yet: %v", err)
	}

	// Wait for opensearch
	if err := helm.WaitForPods(ctx, config.Kubeconfig, config.Namespace, "opensearch.cluster.opensearch.org/cluster-name=primus-lens-logs", 10*time.Minute); err != nil {
		log.Warnf("OpenSearch pods not ready yet: %v", err)
	}

	// Wait for victoriametrics
	if err := helm.WaitForPods(ctx, config.Kubeconfig, config.Namespace, "app.kubernetes.io/name=vmcluster", 5*time.Minute); err != nil {
		log.Warnf("VictoriaMetrics pods not ready yet: %v", err)
	}

	return nil
}

// ===== Init Stage =====

type InitStage struct{}

func (s *InitStage) Name() string      { return model.StageInit }
func (s *InitStage) IsIdempotent() bool { return true } // DB migrations are idempotent (IF NOT EXISTS)

func (s *InitStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// For external storage, skip init job as DB should already be set up
	if config.StorageMode == model.StorageModeExternal {
		log.Info("External storage mode, skipping init job (assuming DB is pre-configured)")
		return nil
	}

	// Check if already run
	exists, _, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseInit)
	if err != nil {
		return err
	}

	if exists {
		log.Infof("Init already run, skipping")
		return nil
	}

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"clusterName": config.ClusterName,
			"namespace":   config.Namespace,
		},
	}

	return helm.Install(ctx, config.Kubeconfig, config.Namespace, ReleaseInit, ChartInit, values)
}

// ===== Storage Secret Stage =====

type StorageSecretStage struct{}

func (s *StorageSecretStage) Name() string      { return model.StageStorageSecret }
func (s *StorageSecretStage) IsIdempotent() bool { return true }

func (s *StorageSecretStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	var storageConfig StorageConfig

	if config.StorageMode == model.StorageModeLensManaged {
		// Get credentials from managed storage secrets
		var err error
		storageConfig, err = s.buildFromManagedStorage(ctx, config)
		if err != nil {
			return fmt.Errorf("failed to build storage config from managed storage: %w", err)
		}
	} else {
		// Use external storage config
		storageConfig = s.buildFromExternalStorage(config)
	}

	// Create storage secret YAML
	secretYAML := s.buildSecretYAML(config.Namespace, storageConfig)

	// Apply the secret
	return helm.ApplyYAML(ctx, config.Kubeconfig, config.Namespace, secretYAML)
}

func (s *StorageSecretStage) buildFromManagedStorage(ctx context.Context, config *InstallConfig) (StorageConfig, error) {
	// Get postgres password from secret
	pgPassword, err := s.getSecretValue(ctx, config.Kubeconfig, config.Namespace,
		"primus-lens.primus-lens.credentials.postgresql.acid.zalan.do", "password")
	if err != nil {
		return StorageConfig{}, fmt.Errorf("failed to get postgres password: %w", err)
	}

	// Get opensearch password from secret
	osPassword, err := s.getSecretValue(ctx, config.Kubeconfig, config.Namespace,
		"primus-lens-logs-admin-credentials", "password")
	if err != nil {
		return StorageConfig{}, fmt.Errorf("failed to get opensearch password: %w", err)
	}

	return StorageConfig{
		Postgres: PostgresConfig{
			Service:   "primus-lens-primary",
			Port:      5432,
			Namespace: config.Namespace,
			Username:  "primus-lens",
			Password:  pgPassword,
			DBName:    "primus-lens",
			SSLMode:   "require",
		},
		OpenSearch: OpenSearchConfig{
			Service:     "primus-lens-logs",
			Port:        9200,
			Namespace:   config.Namespace,
			Username:    "admin",
			Password:    osPassword,
			TLSEnabled:  true,
			TLSInsecure: true,
			Scheme:      "https",
		},
		Prometheus: PrometheusConfig{
			ReadEndpoint:  fmt.Sprintf("http://vmselect-primus-lens-vmcluster.%s.svc.cluster.local:8481/select/0/prometheus", config.Namespace),
			WriteEndpoint: fmt.Sprintf("http://vminsert-primus-lens-vmcluster.%s.svc.cluster.local:8480/insert/0/prometheus", config.Namespace),
		},
	}, nil
}

func (s *StorageSecretStage) buildFromExternalStorage(config *InstallConfig) StorageConfig {
	ext := config.ExternalStorage
	if ext == nil {
		return StorageConfig{}
	}

	pgPort := ext.PostgresPort
	if pgPort == 0 {
		pgPort = 5432
	}

	osPort := ext.OpensearchPort
	if osPort == 0 {
		osPort = 9200
	}

	osScheme := ext.OpensearchScheme
	if osScheme == "" {
		osScheme = "https"
	}

	readPort := ext.PrometheusReadPort
	if readPort == 0 {
		readPort = 8481
	}

	writePort := ext.PrometheusWritePort
	if writePort == 0 {
		writePort = 8480
	}

	return StorageConfig{
		Postgres: PostgresConfig{
			Service:   ext.PostgresHost,
			Port:      pgPort,
			Namespace: config.Namespace,
			Username:  ext.PostgresUsername,
			Password:  ext.PostgresPassword,
			DBName:    ext.PostgresDBName,
			SSLMode:   ext.PostgresSSLMode,
		},
		OpenSearch: OpenSearchConfig{
			Service:     ext.OpensearchHost,
			Port:        osPort,
			Namespace:   config.Namespace,
			Username:    ext.OpensearchUsername,
			Password:    ext.OpensearchPassword,
			TLSEnabled:  osScheme == "https",
			TLSInsecure: true,
			Scheme:      osScheme,
		},
		Prometheus: PrometheusConfig{
			ReadEndpoint:  fmt.Sprintf("http://%s:%d/select/0/prometheus", ext.PrometheusReadHost, readPort),
			WriteEndpoint: fmt.Sprintf("http://%s:%d/insert/0/prometheus", ext.PrometheusWriteHost, writePort),
		},
	}
}

func (s *StorageSecretStage) buildSecretYAML(namespace string, config StorageConfig) []byte {
	pgJSON, _ := json.Marshal(config.Postgres)
	osJSON, _ := json.Marshal(config.OpenSearch)

	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: primus-lens-storage-config
  namespace: %s
type: Opaque
stringData:
  postgres: '%s'
  opensearch: '%s'
  prometheus_read_endpoint: '%s'
  prometheus_write_endpoint: '%s'
`, namespace, string(pgJSON), string(osJSON), config.Prometheus.ReadEndpoint, config.Prometheus.WriteEndpoint))
}

func (s *StorageSecretStage) getSecretValue(ctx context.Context, kubeconfig []byte, namespace, secretName, key string) (string, error) {
	// Write kubeconfig to temp file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return "", err
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeconfig); err != nil {
		return "", err
	}
	kubeconfigFile.Close()

	args := []string{
		"get", "secret", secretName,
		"--kubeconfig", kubeconfigFile.Name(),
		"--namespace", namespace,
		"-o", fmt.Sprintf("jsonpath={.data.%s}", key),
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl get secret failed: %s", stderr.String())
	}

	// Decode base64
	decoded := make([]byte, len(stdout.Bytes()))
	n, err := decodeBase64(stdout.Bytes(), decoded)
	if err != nil {
		return "", err
	}

	return string(decoded[:n]), nil
}

// decodeBase64 decodes base64 encoded bytes
func decodeBase64(src, dst []byte) (int, error) {
	cmd := exec.Command("base64", "-d")
	cmd.Stdin = bytes.NewReader(src)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return 0, err
	}

	copy(dst, stdout.Bytes())
	return stdout.Len(), nil
}

// ===== Applications Stage =====

type ApplicationsStage struct{}

func (s *ApplicationsStage) Name() string      { return model.StageApplications }
func (s *ApplicationsStage) IsIdempotent() bool { return true }

func (s *ApplicationsStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Check if already installed
	exists, healthy, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseApplications)
	if err != nil {
		return err
	}

	if exists && healthy {
		log.Infof("Applications already installed and healthy, skipping")
		return nil
	}

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"clusterName": config.ClusterName,
			"namespace":   config.Namespace,
			"imageRegistry": map[string]interface{}{
				"url":        config.ImageRegistry,
				"pullPolicy": "IfNotPresent",
				"pullSecret": "",
			},
		},
		// Enable all dataplane applications
		"telemetryCollector":  map[string]interface{}{"enabled": true},
		"jobs":                map[string]interface{}{"enabled": true},
		"nodeExporter":        map[string]interface{}{"enabled": true},
		"gpuResourceExporter": map[string]interface{}{"enabled": true},
		"systemTuner":         map[string]interface{}{"enabled": true},
		"aiAdvisor":           map[string]interface{}{"enabled": true},
	}

	if exists {
		return helm.Upgrade(ctx, config.Kubeconfig, config.Namespace, ReleaseApplications, ChartApplications, values)
	}
	return helm.Install(ctx, config.Kubeconfig, config.Namespace, ReleaseApplications, ChartApplications, values)
}

// ===== Wait Applications Stage =====

type WaitAppsStage struct{}

func (s *WaitAppsStage) Name() string      { return model.StageWaitApps }
func (s *WaitAppsStage) IsIdempotent() bool { return true }

func (s *WaitAppsStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Wait for application pods to be ready
	return helm.WaitForPods(ctx, config.Kubeconfig, config.Namespace, "app.kubernetes.io/instance="+ReleaseApplications, 5*time.Minute)
}
