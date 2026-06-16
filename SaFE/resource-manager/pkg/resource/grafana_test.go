/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeGrafanaSyncer() *GrafanaDatasourceSyncer {
	scheme := runtime.NewScheme()
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		grafanaDatasourceGVR: "GrafanaDatasourceList",
	})
	return &GrafanaDatasourceSyncer{
		dynClient:  dc,
		namespace:  "monitoring",
		crdReady:   true,
		crdChecked: true,
	}
}

func TestApplyDatasourceCreate(t *testing.T) {
	s := newFakeGrafanaSyncer()
	ds := s.buildDatasource("c1-prometheus", "prometheus", "http://x", "c1", "c1", nil)
	err := s.applyDatasource(context.Background(), ds)
	assert.NoError(t, err)
	got, err := s.dynClient.Resource(grafanaDatasourceGVR).Namespace("monitoring").Get(context.Background(), "c1-prometheus", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "c1-prometheus", got.GetName())
}

func TestSyncClusterDatasources(t *testing.T) {
	s := newFakeGrafanaSyncer()
	// Should create both prometheus and json-api datasources.
	s.SyncClusterDatasources(context.Background(), "c1", "http://robust:8085")
	list, err := s.dynClient.Resource(grafanaDatasourceGVR).Namespace("monitoring").List(context.Background(), metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, list.Items, 2)
}

func TestRemoveClusterDatasources(t *testing.T) {
	s := newFakeGrafanaSyncer()
	s.SyncClusterDatasources(context.Background(), "c1", "http://robust:8085")
	s.RemoveClusterDatasources(context.Background(), "c1")
	list, err := s.dynClient.Resource(grafanaDatasourceGVR).Namespace("monitoring").List(context.Background(), metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Empty(t, list.Items)
}

func TestEnsureCRDCached(t *testing.T) {
	s := &GrafanaDatasourceSyncer{crdChecked: true, crdReady: true}
	assert.True(t, s.ensureCRD(context.Background()))

	s2 := &GrafanaDatasourceSyncer{crdChecked: true, crdReady: false}
	assert.False(t, s2.ensureCRD(context.Background()))
}
