/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

var grafanaDatasourceGVR = schema.GroupVersionResource{
	Group:    "grafana.integreatly.org",
	Version:  "v1beta1",
	Resource: "grafanadatasources",
}

// GrafanaDatasourceSyncer manages GrafanaDatasource CRs for data-plane clusters.
type GrafanaDatasourceSyncer struct {
	dynClient    dynamic.Interface
	namespace    string
	crdReady     bool
	crdChecked   bool
}

// NewGrafanaDatasourceSyncer creates a syncer that targets the given namespace.
func NewGrafanaDatasourceSyncer(cfg *rest.Config, namespace string) (*GrafanaDatasourceSyncer, error) {
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	return &GrafanaDatasourceSyncer{
		dynClient: dc,
		namespace: namespace,
	}, nil
}

func (s *GrafanaDatasourceSyncer) ensureCRD(ctx context.Context) bool {
	if s.crdChecked {
		return s.crdReady
	}
	_, err := s.dynClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		if isNoGrafanaCRD(err) {
			klog.V(3).Infof("[grafana-syncer] GrafanaDatasource CRD not available, skipping")
		} else {
			klog.Warningf("[grafana-syncer] CRD probe failed: %v", err)
		}
		s.crdReady = false
	} else {
		s.crdReady = true
	}
	s.crdChecked = true
	return s.crdReady
}

func isNoGrafanaCRD(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "the server could not find the requested resource")
}

// SyncClusterDatasources creates/updates the Prometheus and JSON-API datasources
// for a data-plane cluster identified by clusterName and its robust-api endpoint.
func (s *GrafanaDatasourceSyncer) SyncClusterDatasources(ctx context.Context, clusterName, robustEndpoint string) {
	if !s.ensureCRD(ctx) {
		return
	}

	promName := clusterName + "-prometheus"
	promURL := strings.TrimRight(robustEndpoint, "/") + "/api/v1/vm-proxy"
	promDS := s.buildDatasource(promName, "prometheus", promURL, clusterName, clusterName, map[string]interface{}{
		"timeInterval":  "30s",
		"tlsSkipVerify": true,
	})
	if err := s.applyDatasource(ctx, promDS); err != nil {
		klog.Warningf("[grafana-syncer] sync prometheus datasource for %s: %v", clusterName, err)
	}

	jsonName := clusterName + "-robust-api"
	// Use the explicit name as UID for the JSON API to match future dashboard variables.
	jsonDS := s.buildDatasource(jsonName, "marcusolsson-json-datasource", robustEndpoint, clusterName, jsonName, nil)
	if err := s.applyDatasource(ctx, jsonDS); err != nil {
		klog.Warningf("[grafana-syncer] sync json-api datasource for %s: %v", clusterName, err)
	}
}

// RemoveClusterDatasources deletes datasources previously created for a cluster.
func (s *GrafanaDatasourceSyncer) RemoveClusterDatasources(ctx context.Context, clusterName string) {
	if !s.ensureCRD(ctx) {
		return
	}
	for _, suffix := range []string{"-prometheus", "-robust-api"} {
		name := clusterName + suffix
		err := s.dynClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			klog.Warningf("[grafana-syncer] delete datasource %s: %v", name, err)
		} else if err == nil {
			klog.Infof("[grafana-syncer] deleted datasource %s", name)
		}
	}
}

func (s *GrafanaDatasourceSyncer) buildDatasource(name, dsType, url, clusterName, uid string, jsonData map[string]interface{}) *unstructured.Unstructured {
	if jsonData == nil {
		jsonData = map[string]interface{}{}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "grafana.integreatly.org/v1beta1",
			"kind":       "GrafanaDatasource",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": s.namespace,
				"labels": map[string]interface{}{
					"app":                          "primus-safe",
					"primus-safe.amd.com/cluster":  clusterName,
					"app.kubernetes.io/managed-by": "resource-manager",
				},
			},
			"spec": map[string]interface{}{
				"allowCrossNamespaceImport": true,
				"instanceSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"system": "primus-safe",
					},
				},
				"datasource": map[string]interface{}{
					"name":     name,
					"type":     dsType,
					"uid":      uid,
					"access":   "proxy",
					"url":      url,
					"jsonData": jsonData,
				},
				"resyncPeriod": "10m0s",
			},
		},
	}
}

func (s *GrafanaDatasourceSyncer) applyDatasource(ctx context.Context, ds *unstructured.Unstructured) error {
	name := ds.GetName()
	existing, err := s.dynClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = s.dynClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Create(ctx, ds, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("create %s: %w", name, err)
			}
			klog.Infof("[grafana-syncer] created datasource %s", name)
			return nil
		}
		return fmt.Errorf("get %s: %w", name, err)
	}

	ds.SetResourceVersion(existing.GetResourceVersion())
	_, err = s.dynClient.Resource(grafanaDatasourceGVR).Namespace(s.namespace).Update(ctx, ds, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update %s: %w", name, err)
	}
	klog.Infof("[grafana-syncer] updated datasource %s", name)
	return nil
}
