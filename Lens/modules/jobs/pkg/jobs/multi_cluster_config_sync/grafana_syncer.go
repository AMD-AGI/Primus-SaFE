// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package multi_cluster_config_sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// GrafanaSyncer syncs Grafana datasources based on cluster storage configs
type GrafanaSyncer struct {
	dynamicClient  dynamic.Interface
	namespace      string
	instanceLabels map[string]string
	crdAvailable   bool
	crdChecked     bool
}

var grafanaDatasourceGVR = schema.GroupVersionResource{
	Group:    "grafana.integreatly.org",
	Version:  "v1beta1",
	Resource: "grafanadatasources",
}

// NewGrafanaSyncer creates a new Grafana syncer
func NewGrafanaSyncer(namespace string, instanceLabels map[string]string) *GrafanaSyncer {
	return &GrafanaSyncer{
		namespace:      namespace,
		instanceLabels: instanceLabels,
	}
}

// Initialize initializes the Grafana syncer
func (s *GrafanaSyncer) Initialize(ctx context.Context) error {
	currentClients := clientsets.GetClusterManager().GetCurrentClusterClients()
	if currentClients == nil || currentClients.K8SClientSet == nil {
		return fmt.Errorf("current cluster clients not available")
	}

	dynamicClient, err := dynamic.NewForConfig(currentClients.K8SClientSet.Config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}
	s.dynamicClient = dynamicClient

	s.checkCRDAvailability(ctx)
	return nil
}

// checkCRDAvailability checks if GrafanaDatasource CRD exists
func (s *GrafanaSyncer) checkCRDAvailability(ctx context.Context) {
	if s.dynamicClient == nil {
		s.crdAvailable = false
		s.crdChecked = true
		return
	}

	_, err := s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		if errors.IsNotFound(err) || isNoKindMatchError(err) {
			log.Warnf("GrafanaDatasource CRD not available, Grafana sync will be skipped")
			s.crdAvailable = false
		} else {
			log.Warnf("Failed to check GrafanaDatasource CRD: %v", err)
			s.crdAvailable = false
		}
	} else {
		log.Info("GrafanaDatasource CRD is available")
		s.crdAvailable = true
	}
	s.crdChecked = true
}

// isNoKindMatchError checks if error is due to missing CRD
func isNoKindMatchError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "no matches for kind") ||
		strings.Contains(errStr, "the server could not find the requested resource")
}

// SyncDatasources syncs Grafana datasources for all clusters
func (s *GrafanaSyncer) SyncDatasources(ctx context.Context, clusterConfigs map[string]*clientsets.PrimusLensClientConfig) error {
	if !s.crdChecked {
		s.checkCRDAvailability(ctx)
	}

	if !s.crdAvailable {
		log.Debug("Skipping Grafana datasource sync - CRD not available")
		return nil
	}

	log.Infof("Syncing Grafana datasources for %d clusters", len(clusterConfigs))

	// Sync static datasources (primus-lens API)
	if err := s.syncPrimusLensAPIDatasource(ctx); err != nil {
		log.Warnf("Failed to sync primus-lens API datasource: %v", err)
	}

	for clusterName, config := range clusterConfigs {
		if err := s.syncClusterDatasources(ctx, clusterName, config); err != nil {
			log.Warnf("Failed to sync Grafana datasources for cluster %s: %v", clusterName, err)
		}
	}

	return nil
}

// syncPrimusLensAPIDatasource creates the primus-lens JSON API datasource
func (s *GrafanaSyncer) syncPrimusLensAPIDatasource(ctx context.Context) error {
	datasourceName := "primus-lens-api"
	url := fmt.Sprintf("http://primus-lens-api.%s.svc.cluster.local:8989", s.namespace)

	datasource := s.buildDatasourceObject(
		datasourceName,
		"marcusolsson-json-datasource",
		url,
		"default",
		nil,
	)

	return s.createOrUpdateDatasource(ctx, datasource)
}

// syncClusterDatasources syncs datasources for a single cluster
func (s *GrafanaSyncer) syncClusterDatasources(ctx context.Context, clusterName string, config *clientsets.PrimusLensClientConfig) error {
	// Skip default cluster
	if clusterName == "default" {
		return nil
	}

	// Sync Prometheus read datasource
	if config.Prometheus != nil && config.Prometheus.ReadService != "" {
		url := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/select/0/prometheus",
			config.Prometheus.ReadService,
			s.namespace,
			config.Prometheus.ReadPort,
		)

		datasource := s.buildDatasourceObject(
			clusterName,
			"prometheus",
			url,
			clusterName,
			map[string]interface{}{
				"timeInterval":  "5s",
				"tlsSkipVerify": true,
			},
		)

		if err := s.createOrUpdateDatasource(ctx, datasource); err != nil {
			return fmt.Errorf("failed to sync Prometheus datasource: %w", err)
		}
	}

	return nil
}

// buildDatasourceObject builds a GrafanaDatasource object
func (s *GrafanaSyncer) buildDatasourceObject(name, dsType, url, clusterName string, jsonData map[string]interface{}) *unstructured.Unstructured {
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

	return &unstructured.Unstructured{
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
}

// createOrUpdateDatasource creates or updates a GrafanaDatasource
func (s *GrafanaSyncer) createOrUpdateDatasource(ctx context.Context, datasource *unstructured.Unstructured) error {
	name := datasource.GetName()

	existing, err := s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Create(ctx, datasource, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create datasource %s: %w", name, err)
			}
			log.Infof("Created Grafana datasource: %s", name)
			return nil
		}
		return fmt.Errorf("failed to get datasource %s: %w", name, err)
	}

	datasource.SetResourceVersion(existing.GetResourceVersion())
	_, err = s.dynamicClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Update(ctx, datasource, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update datasource %s: %w", name, err)
	}
	log.Infof("Updated Grafana datasource: %s", name)
	return nil
}
