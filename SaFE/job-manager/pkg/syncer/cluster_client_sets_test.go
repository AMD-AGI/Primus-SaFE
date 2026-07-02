/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func newTestClientSets() *ClusterClientSets {
	return &ClusterClientSets{
		name:              "c1",
		resourceInformers: commonutils.NewObjectManager(),
	}
}

func TestClusterClientSetsGettersSetters(t *testing.T) {
	c := newTestClientSets()
	c.SetName("c2")
	assert.Equal(t, c.name, "c2")

	// ClientFactory getter returns whatever was set (nil here).
	c.SetClientFactory(nil)
	assert.Assert(t, c.ClientFactory() == nil)
}

func TestClusterClientSetsGetResourceInformerMissing(t *testing.T) {
	c := newTestClientSets()
	gvk := schema.GroupVersionKind{Group: "g", Version: "v", Kind: "Pod"}

	// Internal getter returns nil when not present.
	assert.Assert(t, c.getResourceInformer(gvk) == nil)

	// Public getter returns an error when not present.
	_, err := c.GetResourceInformer(context.Background(), gvk)
	assert.Assert(t, err != nil)
}

func TestClusterClientSetsReleaseAndDelTemplate(t *testing.T) {
	c := newTestClientSets()
	gvk := schema.GroupVersionKind{Group: "g", Version: "v", Kind: "Pod"}
	// delResourceTemplate on an empty manager is a no-op (logs only).
	c.delResourceTemplate(gvk)
	// Release clears informers without error.
	assert.NilError(t, c.Release())
}

func TestGetClusterClientSets(t *testing.T) {
	mgr := commonutils.NewObjectManager()

	// Missing entry -> error.
	_, err := GetClusterClientSets(mgr, "missing")
	assert.Assert(t, err != nil)

	// Present entry -> returned.
	c := newTestClientSets()
	assert.NilError(t, mgr.Add("c1", c))
	got, err := GetClusterClientSets(mgr, "c1")
	assert.NilError(t, err)
	assert.Equal(t, got.name, "c1")
}

func TestHandleResourceWrongType(t *testing.T) {
	called := false
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { called = true }),
	}
	// Not an *unstructured.Unstructured -> ignored.
	c.handleResource(context.Background(), nil, &corev1.Pod{}, ResourceAdd)
	assert.Equal(t, called, false)
}

func TestHandleResourceNoWorkloadId(t *testing.T) {
	called := false
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { called = true }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Pod")
	u.SetName("p1")
	// No workload-id label and no mesh label -> not a managed object, ignored.
	c.handleResource(context.Background(), nil, u, ResourceAdd)
	assert.Equal(t, called, false)
}

func TestHandleResourceManaged(t *testing.T) {
	var captured *resourceMessage
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { captured = m }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Job")
	u.SetName("obj")
	u.SetNamespace("ns")
	u.SetLabels(map[string]string{
		v1.WorkloadIdLabel:          "w",
		v1.WorkloadDispatchCntLabel: "3",
	})
	c.handleResource(context.Background(), nil, u, ResourceAdd)
	assert.Assert(t, captured != nil)
	assert.Equal(t, captured.workloadId, "w")
	assert.Equal(t, captured.dispatchCount, 3)
	assert.Equal(t, captured.cluster, "cl")
}
