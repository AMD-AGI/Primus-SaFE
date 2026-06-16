/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

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

func TestDeletePodNilObject(t *testing.T) {
	r := &SyncerReconciler{}
	res, err := r.deletePod(context.Background(), nil, nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestDeletePodRecentDeletionRequeues(t *testing.T) {
	r := &SyncerReconciler{}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	now := metav1.NewTime(time.Now())
	obj.SetDeletionTimestamp(&now)
	res, err := r.deletePod(context.Background(), obj, nil)
	assert.NilError(t, err)
	// Recently-deleted pod -> requeue, not yet force-deleted.
	assert.Assert(t, res.RequeueAfter > 0)
}
