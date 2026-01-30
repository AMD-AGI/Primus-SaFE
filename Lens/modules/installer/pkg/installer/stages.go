// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package installer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Stage constants
const (
	StagePending           = "pending"
	StageOperators         = "operators"
	StageWaitOperators     = "wait_operators"
	StageInfrastructure    = "infrastructure"
	StageWaitInfra         = "wait_infrastructure"
	StageInit              = "init"
	StageDatabaseMigration = "database_migration"
	StageStorageSecret     = "storage_secret"
	StageApplications      = "applications"
	StageWaitApps          = "wait_applications"
	StageCompleted         = "completed"
)

// GetStageSequence returns the ordered list of stages based on storage mode
func GetStageSequence(storageMode string) []string {
	if storageMode == StorageModeLensManaged {
		return []string{
			StageOperators,
			StageWaitOperators,
			StageInfrastructure,
			StageWaitInfra,
			StageInit,
			StageDatabaseMigration,
			StageStorageSecret,
			StageApplications,
			StageWaitApps,
		}
	}
	// External storage mode - skip operators/infrastructure
	return []string{
		StageInit,
		StageDatabaseMigration,
		StageStorageSecret,
		StageApplications,
		StageWaitApps,
	}
}

// GetUpgradeStageSequence returns stages for upgrade/rollback (skip infrastructure)
func GetUpgradeStageSequence() []string {
	return []string{
		StageApplications,
		StageWaitApps,
	}
}

// GetInfrastructureStageSequence returns stages for infrastructure initialization only
func GetInfrastructureStageSequence(storageMode string) []string {
	if storageMode == StorageModeLensManaged {
		return []string{
			StageOperators,
			StageWaitOperators,
			StageInfrastructure,
			StageWaitInfra,
			StageInit,
			StageDatabaseMigration,
			StageStorageSecret,
		}
	}
	// External storage mode - only init and storage secret
	return []string{
		StageInit,
		StageDatabaseMigration,
		StageStorageSecret,
	}
}

// GetAppsStageSequence returns stages for apps deployment only
func GetAppsStageSequence() []string {
	return []string{
		StageApplications,
		StageWaitApps,
	}
}

// GetStageSequenceByScope returns the ordered list of stages based on install scope and storage mode
func GetStageSequenceByScope(scope, storageMode string) []string {
	switch scope {
	case InstallScopeInfrastructure:
		return GetInfrastructureStageSequence(storageMode)
	case InstallScopeApps:
		return GetAppsStageSequence()
	default:
		// Full scope (backward compatible)
		return GetStageSequence(storageMode)
	}
}

// ===== Operators Stage =====

type OperatorsStage struct{}

func (s *OperatorsStage) Name() string       { return StageOperators }
func (s *OperatorsStage) IsIdempotent() bool { return true }

func (s *OperatorsStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Check if already installed by this release
	exists, healthy, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseOperators)
	if err != nil {
		return err
	}

	if exists && healthy {
		log.Infof("Operators already installed and healthy, skipping")
		return nil
	}

	// Detect which operators already exist in the cluster
	operatorStatus, err := helm.DetectOperators(ctx, config.Kubeconfig)
	if err != nil {
		log.Warnf("Failed to detect existing operators: %v, will attempt to install all", err)
		operatorStatus = &OperatorStatus{} // All false - install everything
	}

	// If all operators exist, skip the stage entirely
	if operatorStatus.AllExist() {
		log.Infof("All operators already exist in cluster, skipping operators stage")
		return nil
	}

	// Build values with existing operators disabled
	values := map[string]interface{}{
		"global": map[string]interface{}{
			"namespace": config.Namespace,
		},
	}

	// Disable operators that already exist to avoid conflicts
	if operatorStatus.PGO {
		log.Infof("PGO already exists, disabling in chart")
		values["database"] = map[string]interface{}{"enabled": false}
		values["pgo"] = map[string]interface{}{"enabled": false}
	}
	if operatorStatus.OpenSearch {
		log.Infof("OpenSearch Operator already exists, disabling in chart")
		values["opensearch"] = map[string]interface{}{"enabled": false}
		values["opensearch-operator"] = map[string]interface{}{"enabled": false}
	}
	if operatorStatus.Grafana {
		log.Infof("Grafana Operator already exists, disabling in chart")
		values["grafana"] = map[string]interface{}{"enabled": false}
		values["grafana-operator"] = map[string]interface{}{"enabled": false}
	}
	if operatorStatus.VictoriaMetrics {
		log.Infof("VictoriaMetrics Operator already exists, disabling in chart")
		values["victoriametrics"] = map[string]interface{}{"enabled": false}
		values["vm-operator"] = map[string]interface{}{"enabled": false}
	}
	if operatorStatus.Fluent {
		log.Infof("Fluent Operator already exists, disabling in chart")
		values["logging"] = map[string]interface{}{"enabled": false}
		values["fluent-operator"] = map[string]interface{}{"enabled": false}
	}
	if operatorStatus.KubeStateMetrics {
		log.Infof("Kube State Metrics already exists, disabling in chart")
		values["monitoring"] = map[string]interface{}{
			"kubeStateMetrics": map[string]interface{}{"enabled": false},
		}
		values["kube-state-metrics"] = map[string]interface{}{"enabled": false}
	}

	// Log what will be installed
	toInstall := []string{}
	if !operatorStatus.PGO {
		toInstall = append(toInstall, "PGO")
	}
	if !operatorStatus.OpenSearch {
		toInstall = append(toInstall, "OpenSearch")
	}
	if !operatorStatus.Grafana {
		toInstall = append(toInstall, "Grafana")
	}
	if !operatorStatus.VictoriaMetrics {
		toInstall = append(toInstall, "VictoriaMetrics")
	}
	if !operatorStatus.Fluent {
		toInstall = append(toInstall, "Fluent")
	}
	if !operatorStatus.KubeStateMetrics {
		toInstall = append(toInstall, "KubeStateMetrics")
	}
	log.Infof("Installing missing operators: %v", toInstall)

	if exists {
		return helm.Upgrade(ctx, config.Kubeconfig, config.Namespace, ReleaseOperators, ChartOperators, values)
	}
	return helm.Install(ctx, config.Kubeconfig, config.Namespace, ReleaseOperators, ChartOperators, values)
}

// ===== Wait Operators Stage =====

type WaitOperatorsStage struct{}

func (s *WaitOperatorsStage) Name() string       { return StageWaitOperators }
func (s *WaitOperatorsStage) IsIdempotent() bool { return true }

func (s *WaitOperatorsStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Check if operators release exists
	exists, _, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseOperators)
	if err != nil {
		log.Warnf("Failed to check operators release status: %v", err)
	}

	if !exists {
		// Operators release doesn't exist, check if all operators already exist
		operatorStatus, _ := helm.DetectOperators(ctx, config.Kubeconfig)
		if operatorStatus != nil && operatorStatus.AllExist() {
			log.Infof("All operators already exist, skipping wait stage")
			return nil
		}
	}

	// Wait for operator pods from our release
	return helm.WaitForPods(ctx, config.Kubeconfig, config.Namespace, "app.kubernetes.io/instance="+ReleaseOperators, 5*time.Minute)
}

// ===== Infrastructure Stage =====

type InfrastructureStage struct{}

func (s *InfrastructureStage) Name() string       { return StageInfrastructure }
func (s *InfrastructureStage) IsIdempotent() bool { return true }

func (s *InfrastructureStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	exists, healthy, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseInfrastructure)
	if err != nil {
		return err
	}

	if exists && healthy {
		log.Infof("Infrastructure already installed and healthy, skipping")
		return nil
	}

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

	// Ensure storage class is set
	storageClass := managed.StorageClass
	if storageClass == "" {
		storageClass = config.StorageClass
	}
	if storageClass == "" {
		storageClass = DefaultStorageClass
	}

	// Ensure storage sizes have defaults
	postgresSize := managed.PostgresSize
	if postgresSize == "" {
		postgresSize = DefaultPostgresSize
	}
	opensearchSize := managed.OpensearchSize
	if opensearchSize == "" {
		opensearchSize = DefaultOSSize
	}
	vmSize := managed.VictoriametricsSize
	if vmSize == "" {
		vmSize = DefaultVMSize
	}
	osReplicas := managed.OpensearchReplicas
	if osReplicas == 0 {
		osReplicas = DefaultOSReplicas
	}

	log.Infof("Infrastructure config: storageClass=%s, postgres=%s, opensearch=%s (replicas=%d), vm=%s",
		storageClass, postgresSize, opensearchSize, osReplicas, vmSize)

	values := map[string]interface{}{
		"global": map[string]interface{}{
			"clusterName":  config.ClusterName,
			"namespace":    config.Namespace,
			"storageClass": storageClass,
		},
		"database": map[string]interface{}{
			"enabled": managed.PostgresEnabled,
			"storage": map[string]interface{}{
				"size":       postgresSize,
				"backupSize": postgresSize, // Use same size for backup
			},
		},
		"victoriametrics": map[string]interface{}{
			"enabled": managed.VictoriametricsEnabled,
			"storage": map[string]interface{}{
				"size": vmSize,
			},
		},
		"opensearch": map[string]interface{}{
			"enabled":     managed.OpensearchEnabled,
			"clusterName": "primus-lens-logs",
			"nodeSets": []map[string]interface{}{
				{
					"name":     "nodes",
					"replicas": osReplicas,
					"roles":    []string{"master", "data", "ingest"},
				},
			},
			"storage": map[string]interface{}{
				"size": opensearchSize,
			},
		},
	}

	if exists {
		return helm.Upgrade(ctx, config.Kubeconfig, config.Namespace, ReleaseInfrastructure, ChartInfrastructure, values)
	}
	return helm.Install(ctx, config.Kubeconfig, config.Namespace, ReleaseInfrastructure, ChartInfrastructure, values)
}

// ===== Wait Infrastructure Stage =====

type WaitInfraStage struct{}

func (s *WaitInfraStage) Name() string       { return StageWaitInfra }
func (s *WaitInfraStage) IsIdempotent() bool { return true }

func (s *WaitInfraStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	// Wait for postgres - required for database_migration stage
	// Retry with backoff since pods may not exist immediately after CR creation
	if err := s.waitForPodsWithRetry(ctx, helm, config, "postgres-operator.crunchydata.com/cluster=primus-lens", "Postgres", 10*time.Minute, true); err != nil {
		return err
	}

	// Wait for opensearch (optional - warn on failure but continue)
	// Note: OpenSearch operator uses 'opster.io/opensearch-cluster' label
	if err := s.waitForPodsWithRetry(ctx, helm, config, "opster.io/opensearch-cluster=primus-lens-logs", "OpenSearch", 10*time.Minute, false); err != nil {
		log.Warnf("OpenSearch not ready, continuing: %v", err)
	}

	// Wait for victoriametrics (optional - warn on failure but continue)
	// VictoriaMetrics has 3 components: vmstorage, vmselect, vminsert
	// Use instance label to match all components
	if err := s.waitForPodsWithRetry(ctx, helm, config, "app.kubernetes.io/instance=primus-lens-vmcluster", "VictoriaMetrics", 5*time.Minute, false); err != nil {
		log.Warnf("VictoriaMetrics not ready, continuing: %v", err)
	}

	return nil
}

// waitForPodsWithRetry waits for pods with retry logic for "no matching resources found" case
func (s *WaitInfraStage) waitForPodsWithRetry(ctx context.Context, helm *HelmClient, config *InstallConfig, labelSelector, componentName string, timeout time.Duration, required bool) error {
	maxRetries := 30       // Max retries for "no resources found" case
	retryInterval := 20 * time.Second
	startTime := time.Now()

	for retry := 0; retry < maxRetries; retry++ {
		// Check if we've exceeded total timeout
		if time.Since(startTime) > timeout {
			if required {
				return fmt.Errorf("%s pods not ready within timeout", componentName)
			}
			return fmt.Errorf("%s pods not found within timeout", componentName)
		}

		err := helm.WaitForPods(ctx, config.Kubeconfig, config.Namespace, labelSelector, timeout-time.Since(startTime))
		if err == nil {
			log.Infof("%s pods are ready", componentName)
			return nil
		}

		// Check if error is "no matching resources found" - pods don't exist yet
		errStr := err.Error()
		if strings.Contains(errStr, "no matching resources found") {
			log.Infof("%s pods not yet created, waiting... (retry %d/%d)", componentName, retry+1, maxRetries)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
				continue
			}
		}

		// Other error - pods exist but not ready yet, kubectl wait handles this
		// If it returned error, it means timeout or other issue
		if required {
			return fmt.Errorf("%s pods not ready: %w", componentName, err)
		}
		return err
	}

	if required {
		return fmt.Errorf("%s pods not created after %d retries", componentName, maxRetries)
	}
	return fmt.Errorf("%s pods not found after %d retries", componentName, maxRetries)
}

// ===== Init Stage =====

type InitStage struct{}

func (s *InitStage) Name() string       { return StageInit }
func (s *InitStage) IsIdempotent() bool { return true }

func (s *InitStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	if config.StorageMode == StorageModeExternal {
		log.Info("External storage mode, skipping init job (assuming DB is pre-configured)")
		return nil
	}

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

// ===== Database Migration Stage =====

type DatabaseMigrationStage struct{}

func (s *DatabaseMigrationStage) Name() string       { return StageDatabaseMigration }
func (s *DatabaseMigrationStage) IsIdempotent() bool { return true }

func (s *DatabaseMigrationStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	log.Info("Running database migrations...")

	// Get database connection info
	var host, user, password, dbName, sslMode string
	var port int

	if config.StorageMode == StorageModeExternal && config.ExternalStorage != nil {
		// External storage mode - use provided connection info
		host = config.ExternalStorage.PostgresHost
		port = config.ExternalStorage.PostgresPort
		user = config.ExternalStorage.PostgresUsername
		password = config.ExternalStorage.PostgresPassword
		dbName = config.ExternalStorage.PostgresDBName
		sslMode = config.ExternalStorage.PostgresSSLMode
	} else if config.StorageMode == StorageModeLensManaged {
		// Lens-managed mode - get credentials from cluster secrets
		log.Info("Lens-managed storage mode, fetching database credentials from cluster...")

		// Get postgres password from secret (CrunchyData PGO format: {cluster}-pguser-{user})
		var err error
		password, err = helm.GetSecretValue(ctx, config.Kubeconfig, config.Namespace,
			"primus-lens-pguser-primus-lens", "password")
		if err != nil {
			return fmt.Errorf("failed to get postgres password: %w", err)
		}

		host = fmt.Sprintf("primus-lens-primary.%s.svc.cluster.local", config.Namespace)
		port = 5432
		user = "primus-lens"
		dbName = "primus-lens"
		sslMode = "require"
	} else {
		log.Warn("No storage configuration available, skipping database migration")
		return nil
	}

	// Default values
	if port == 0 {
		port = 5432
	}
	if sslMode == "" {
		sslMode = "require"
	}

	// Run migrations
	migrationsPath := DefaultMigrationsPath
	if envPath := getEnvOrDefault("MIGRATIONS_PATH", ""); envPath != "" {
		migrationsPath = envPath
	}

	if err := ConnectAndMigrate(ctx, host, port, user, password, dbName, sslMode, migrationsPath); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}

	log.Info("Database migrations completed successfully")
	return nil
}

// getEnvOrDefault returns the environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ===== Storage Secret Stage =====

type StorageSecretStage struct{}

func (s *StorageSecretStage) Name() string       { return StageStorageSecret }
func (s *StorageSecretStage) IsIdempotent() bool { return true }

func (s *StorageSecretStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	var storageConfig StorageConfig

	if config.StorageMode == StorageModeLensManaged {
		var err error
		storageConfig, err = s.buildFromManagedStorage(ctx, helm, config)
		if err != nil {
			return fmt.Errorf("failed to build storage config from managed storage: %w", err)
		}
	} else {
		storageConfig = s.buildFromExternalStorage(config)
	}

	secretYAML := s.buildSecretYAML(config.Namespace, storageConfig)
	return helm.ApplyYAML(ctx, config.Kubeconfig, config.Namespace, secretYAML)
}

func (s *StorageSecretStage) buildFromManagedStorage(ctx context.Context, helm *HelmClient, config *InstallConfig) (StorageConfig, error) {
	// CrunchyData PGO secret format: {cluster}-pguser-{user}
	pgPassword, err := helm.GetSecretValue(ctx, config.Kubeconfig, config.Namespace,
		"primus-lens-pguser-primus-lens", "password")
	if err != nil {
		return StorageConfig{}, fmt.Errorf("failed to get postgres password: %w", err)
	}

	osPassword, err := helm.GetSecretValue(ctx, config.Kubeconfig, config.Namespace,
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

// ===== Applications Stage =====

type ApplicationsStage struct{}

func (s *ApplicationsStage) Name() string       { return StageApplications }
func (s *ApplicationsStage) IsIdempotent() bool { return true }

func (s *ApplicationsStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	exists, healthy, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseApplications)
	if err != nil {
		return err
	}

	// For upgrade mode, we always want to upgrade even if healthy
	if exists && healthy && !config.IsUpgrade {
		log.Infof("Applications already installed and healthy, skipping")
		return nil
	}

	// Use merged values from release management if available
	var values map[string]interface{}
	if config.MergedValues != nil && len(config.MergedValues) > 0 {
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
		// Build default values
		values = map[string]interface{}{
			"global": map[string]interface{}{
				"clusterName": config.ClusterName,
				"namespace":   config.Namespace,
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

	// Determine chart name - use version-specific if available
	chartName := ChartApplications
	if config.ChartVersion != "" {
		log.Infof("Using chart version: %s", config.ChartVersion)
	}

	// Execute install or upgrade
	if exists || config.IsUpgrade {
		log.Infof("Upgrading applications release")
		return helm.Upgrade(ctx, config.Kubeconfig, config.Namespace, ReleaseApplications, chartName, values)
	}
	log.Infof("Installing applications release")
	return helm.Install(ctx, config.Kubeconfig, config.Namespace, ReleaseApplications, chartName, values)
}

// ===== Wait Applications Stage =====

type WaitAppsStage struct{}

func (s *WaitAppsStage) Name() string       { return StageWaitApps }
func (s *WaitAppsStage) IsIdempotent() bool { return true }

func (s *WaitAppsStage) Execute(ctx context.Context, helm *HelmClient, config *InstallConfig) error {
	return helm.WaitForPods(ctx, config.Kubeconfig, config.Namespace, "app.kubernetes.io/instance="+ReleaseApplications, 5*time.Minute)
}

// GetAllStages returns all stage implementations
func GetAllStages() map[string]Stage {
	return map[string]Stage{
		StageOperators:         &OperatorsStage{},
		StageWaitOperators:     &WaitOperatorsStage{},
		StageInfrastructure:    &InfrastructureStage{},
		StageWaitInfra:         &WaitInfraStage{},
		StageInit:              &InitStage{},
		StageDatabaseMigration: &DatabaseMigrationStage{},
		StageStorageSecret:     &StorageSecretStage{},
		StageApplications:      &ApplicationsStage{},
		StageWaitApps:          &WaitAppsStage{},
	}
}
