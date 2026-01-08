// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package grafana

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// DatasourceSyncer syncs Grafana datasources based on cluster storage configs
type DatasourceSyncer struct {
	dynamicClient   dynamic.Interface
	namespace       string
	instanceLabels  map[string]string // Labels to select Grafana instance
	crdAvailable    bool              // Whether the CRD is available
	crdChecked      bool              // Whether we've checked for CRD availability
}

var grafanaDatasourceGVR = schema.GroupVersionResource{
	Group:    "grafana.integreatly.org",
	Version:  "v1beta1",
	Resource: "grafanadatasources",
}

// NewDatasourceSyncer creates a new Grafana datasource syncer
func NewDatasourceSyncer(namespace string, instanceLabels map[string]string) *DatasourceSyncer {
	return &DatasourceSyncer{
		namespace:      namespace,
		instanceLabels: instanceLabels,
	}
}

// Initialize initializes the syncer with the current cluster's dynamic client
func (s *DatasourceSyncer) Initialize(ctx context.Context) error {
	currentClients := clientsets.GetClusterManager().GetCurrentClusterClients()
	if currentClients == nil || currentClients.K8SClientSet == nil {
		return fmt.Errorf("current cluster clients not available")
	}

	// Create dynamic client from the rest config
	dynamicClient, err := dynamic.NewForConfig(currentClients.K8SClientSet.Config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}
	s.dynamicClient = dynamicClient

	// Check if CRD is available
	s.checkCRDAvailability(ctx)

	return nil
}

// checkCRDAvailability checks if the GrafanaDatasource CRD is available
func (s *DatasourceSyncer) checkCRDAvailability(ctx context.Context) {
	if s.dynamicClient == nil {
		s.crdAvailable = false
		s.crdChecked = true
		return
	}

	// Try to list grafanadatasources - if it fails with NotFound, CRD doesn't exist
	_, err := s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		if errors.IsNotFound(err) || isNoKindMatchError(err) {
			log.Warnf("GrafanaDatasource CRD not available in cluster, Grafana datasource sync will be skipped")
			s.crdAvailable = false
		} else {
			log.Warnf("Failed to check GrafanaDatasource CRD availability: %v", err)
			s.crdAvailable = false
		}
	} else {
		log.Info("GrafanaDatasource CRD is available, Grafana datasource sync enabled")
		s.crdAvailable = true
	}
	s.crdChecked = true
}

// isNoKindMatchError checks if the error is due to missing CRD
func isNoKindMatchError(err error) bool {
	// Check if error message contains "no matches for kind"
	return err != nil && (errors.IsNotFound(err) || 
		// Handle "no matches for kind" error which is returned when CRD doesn't exist
		(err.Error() != "" && 
			(contains(err.Error(), "no matches for kind") ||
			 contains(err.Error(), "the server could not find the requested resource"))))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// IsCRDAvailable returns whether the GrafanaDatasource CRD is available
func (s *DatasourceSyncer) IsCRDAvailable() bool {
	return s.crdAvailable
}

// SyncDatasources syncs Grafana datasources for all clusters based on their storage configs
// This is called after storage configs are synced
func (s *DatasourceSyncer) SyncDatasources(ctx context.Context, clusterConfigs map[string]*clientsets.PrimusLensClientConfig) error {
	if !s.crdChecked {
		s.checkCRDAvailability(ctx)
	}

	if !s.crdAvailable {
		log.Debug("Skipping Grafana datasource sync - CRD not available")
		return nil
	}

	if s.dynamicClient == nil {
		if err := s.Initialize(ctx); err != nil {
			log.Warnf("Failed to initialize Grafana datasource syncer: %v", err)
			return nil // Don't block main flow
		}
	}

	log.Infof("Syncing Grafana datasources for %d clusters", len(clusterConfigs))

	for clusterName, config := range clusterConfigs {
		if err := s.syncClusterDatasources(ctx, clusterName, config); err != nil {
			log.Warnf("Failed to sync Grafana datasources for cluster %s: %v", clusterName, err)
			// Continue with other clusters, don't fail the entire sync
		}
	}

	return nil
}

// syncClusterDatasources syncs datasources for a single cluster
func (s *DatasourceSyncer) syncClusterDatasources(ctx context.Context, clusterName string, config *clientsets.PrimusLensClientConfig) error {
	// Sync Prometheus read datasource
	if config.Prometheus != nil && config.Prometheus.ReadService != "" {
		if err := s.syncPrometheusDatasource(ctx, clusterName, config.Prometheus); err != nil {
			log.Warnf("Failed to sync Prometheus datasource for cluster %s: %v", clusterName, err)
		}
	}

	// Sync Postgres datasource
	if config.Postgres != nil && config.Postgres.Service != "" {
		if err := s.syncPostgresDatasource(ctx, clusterName, config.Postgres); err != nil {
			log.Warnf("Failed to sync Postgres datasource for cluster %s: %v", clusterName, err)
		}
	}

	return nil
}

// syncPrometheusDatasource creates or updates a Prometheus datasource for a cluster
func (s *DatasourceSyncer) syncPrometheusDatasource(ctx context.Context, clusterName string, config *clientsets.PrimusLensClientConfigPrometheus) error {
	datasourceName := fmt.Sprintf("prometheus-%s", clusterName)
	
	// Build the datasource URL
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/select/0/prometheus",
		config.ReadService,
		s.namespace,
		config.ReadPort,
	)

	datasource := s.buildDatasourceObject(
		datasourceName,
		"prometheus",
		url,
		clusterName,
		map[string]interface{}{
			"timeInterval":   "5s",
			"tlsSkipVerify":  true,
		},
		nil,
	)

	return s.createOrUpdateDatasource(ctx, datasource)
}

// syncPostgresDatasource creates or updates a Postgres datasource for a cluster
func (s *DatasourceSyncer) syncPostgresDatasource(ctx context.Context, clusterName string, config *clientsets.PrimusLensClientConfigPostgres) error {
	datasourceName := fmt.Sprintf("postgresql-%s", clusterName)
	
	// Build the datasource URL
	url := fmt.Sprintf("%s.%s.svc.cluster.local:%d",
		config.Service,
		s.namespace,
		config.Port,
	)

	sslMode := config.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}

	datasource := s.buildDatasourceObject(
		datasourceName,
		"postgres",
		url,
		clusterName,
		map[string]interface{}{
			"database":         config.DBName,
			"sslmode":          sslMode,
			"maxOpenConns":     0,
			"maxIdleConns":     2,
			"connMaxLifetime":  14400,
			"postgresVersion":  1400,
			"timescaledb":      false,
		},
		map[string]string{
			"password": config.Password,
		},
	)

	// Add user field
	datasource.Object["spec"].(map[string]interface{})["datasource"].(map[string]interface{})["user"] = config.Username

	return s.createOrUpdateDatasource(ctx, datasource)
}

// buildDatasourceObject builds an unstructured GrafanaDatasource object
func (s *DatasourceSyncer) buildDatasourceObject(
	name, dsType, url, clusterName string,
	jsonData map[string]interface{},
	secureJsonData map[string]string,
) *unstructured.Unstructured {
	labels := map[string]interface{}{
		"app":     "primus-lens",
		"cluster": clusterName,
	}

	instanceSelector := map[string]interface{}{
		"matchLabels": s.instanceLabels,
	}

	datasource := map[string]interface{}{
		"name":     name,
		"type":     dsType,
		"access":   "proxy",
		"url":      url,
		"jsonData": jsonData,
	}

	if secureJsonData != nil {
		datasource["secureJsonData"] = secureJsonData
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "grafana.integreatly.org/v1beta1",
			"kind":       "GrafanaDatasource",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": s.namespace,
				"labels":    labels,
			},
			"spec": map[string]interface{}{
				"allowCrossNamespaceImport": true,
				"datasource":                datasource,
				"instanceSelector":          instanceSelector,
				"resyncPeriod":              "10m0s",
			},
		},
	}

	return obj
}

// createOrUpdateDatasource creates or updates a GrafanaDatasource
func (s *DatasourceSyncer) createOrUpdateDatasource(ctx context.Context, datasource *unstructured.Unstructured) error {
	name := datasource.GetName()
	
	// Try to get existing datasource
	existing, err := s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new datasource
			_, err = s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Create(ctx, datasource, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create datasource %s: %w", name, err)
			}
			log.Infof("Created Grafana datasource: %s/%s", s.namespace, name)
			return nil
		}
		return fmt.Errorf("failed to get existing datasource %s: %w", name, err)
	}

	// Update existing datasource - preserve resourceVersion
	datasource.SetResourceVersion(existing.GetResourceVersion())
	_, err = s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Update(ctx, datasource, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update datasource %s: %w", name, err)
	}
	log.Infof("Updated Grafana datasource: %s/%s", s.namespace, name)
	return nil
}

// DeleteDatasource deletes a Grafana datasource
func (s *DatasourceSyncer) DeleteDatasource(ctx context.Context, name string) error {
	if !s.crdAvailable {
		return nil
	}

	err := s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete datasource %s: %w", name, err)
	}
	log.Infof("Deleted Grafana datasource: %s/%s", s.namespace, name)
	return nil
}

// ListClusterDatasources lists all Grafana datasources created by this syncer for a cluster
func (s *DatasourceSyncer) ListClusterDatasources(ctx context.Context, clusterName string) ([]string, error) {
	if !s.crdAvailable {
		return nil, nil
	}

	labelSelector := fmt.Sprintf("app=primus-lens,cluster=%s", clusterName)
	list, err := s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	var names []string
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	return names, nil
}

// ClusterConfigFromStorageData parses cluster storage config from the secret data
func ClusterConfigFromStorageData(data []byte) (*clientsets.PrimusLensClientConfig, error) {
	// The data is a JSON map of component -> config
	configMap := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &configMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config data: %w", err)
	}

	config := &clientsets.PrimusLensClientConfig{}

	if prometheusData, ok := configMap["prometheus"]; ok {
		config.Prometheus = &clientsets.PrimusLensClientConfigPrometheus{}
		if err := json.Unmarshal(prometheusData, config.Prometheus); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prometheus config: %w", err)
		}
	}

	if postgresData, ok := configMap["postgres"]; ok {
		config.Postgres = &clientsets.PrimusLensClientConfigPostgres{}
		if err := json.Unmarshal(postgresData, config.Postgres); err != nil {
			return nil, fmt.Errorf("failed to unmarshal postgres config: %w", err)
		}
	}

	if opensearchData, ok := configMap["opensearch"]; ok {
		config.Opensearch = &clientsets.PrimusLensClientConfigOpensearch{}
		if err := json.Unmarshal(opensearchData, config.Opensearch); err != nil {
			return nil, fmt.Errorf("failed to unmarshal opensearch config: %w", err)
		}
	}

	return config, nil
}

