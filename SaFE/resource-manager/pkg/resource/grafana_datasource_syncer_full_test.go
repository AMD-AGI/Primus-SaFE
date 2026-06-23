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

func newGrafanaSyncer() *GrafanaDatasourceSyncer {
	scheme := runtime.NewScheme()
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			grafanaDatasourceGVR: "GrafanaDatasourceList",
		})
	return &GrafanaDatasourceSyncer{dynClient: dc, namespace: "monitoring"}
}

func TestGrafanaSyncAndRemove(t *testing.T) {
	s := newGrafanaSyncer()
	ctx := context.Background()

	assert.True(t, s.ensureCRD(ctx))
	// cached path
	assert.True(t, s.ensureCRD(ctx))

	s.SyncClusterDatasources(ctx, "c1", "http://robust.local/")
	got, err := s.dynClient.Resource(grafanaDatasourceGVR).Namespace("monitoring").Get(ctx, "c1-prometheus", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, got)

	// update path (apply again)
	s.SyncClusterDatasources(ctx, "c1", "http://robust.local/")

	s.RemoveClusterDatasources(ctx, "c1")
	_, err = s.dynClient.Resource(grafanaDatasourceGVR).Namespace("monitoring").Get(ctx, "c1-prometheus", metav1.GetOptions{})
	assert.Error(t, err)
}
