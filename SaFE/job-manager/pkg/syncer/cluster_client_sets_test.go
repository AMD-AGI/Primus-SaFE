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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestClusterClientSetsNeedsInformerRetry(t *testing.T) {
	c := newTestClientSets()
	rtList := &v1.ResourceTemplateList{
		Items: []v1.ResourceTemplate{
			{Spec: v1.ResourceTemplateSpec{
				GroupVersionKind: v1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			}},
		},
	}
	assert.Equal(t, c.needsInformerRetry(rtList), true)
}

func TestClusterClientSetsInformerCount(t *testing.T) {
	c := newTestClientSets()
	assert.Equal(t, c.informerCount(), 0)
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

func TestToUnstructuredTombstone(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Sandbox")
	u.SetName("sb")
	got, ok := toUnstructured(cache.DeletedFinalStateUnknown{Obj: u})
	assert.Assert(t, ok)
	assert.Equal(t, got.GetName(), "sb")
}

func TestHandleResourceDeleteTombstone(t *testing.T) {
	var captured *resourceMessage
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { captured = m }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Sandbox")
	u.SetName("sb")
	u.SetNamespace("ws")
	u.SetLabels(map[string]string{v1.WorkloadIdLabel: "w"})
	tombstone := cache.DeletedFinalStateUnknown{Obj: u}
	c.handleResource(context.Background(), tombstone, tombstone, ResourceDel)
	assert.Assert(t, captured != nil)
	assert.Equal(t, captured.action, ResourceDel)
	assert.Equal(t, captured.workloadId, "w")
	assert.Equal(t, captured.name, "sb")
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

func TestNeedsClientFactoryRefreshInvalid(t *testing.T) {
	cs := newTestClientSets()
	factory := commonclient.NewClientFactoryForTest("c1", "https://10.0.0.1:6443")
	factory.SetValid(false, "watch error")
	cs.dataClientFactory = factory
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status: v1.ClusterStatus{
			ControlPlaneStatus: v1.ControlPlaneStatus{Phase: v1.ReadyPhase},
		},
	}
	assert.Assert(t, cs.needsClientFactoryRefresh(context.Background(), cluster, nil))
}

func TestNeedsClientFactoryRefreshBackendIPsChanged(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.96.1.1", Ports: []corev1.ServicePort{{Port: 6443}}},
	}
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
		}},
	}
	adminClient := ctrlfake.NewClientBuilder().WithScheme(mockScheme).WithObjects(service, endpoints).Build()
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status:     v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{Phase: v1.ReadyPhase}},
	}
	cs := newTestClientSets()
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	factory.SetBackendFingerprint("10.0.0.1,10.0.0.2")
	cs.dataClientFactory = factory
	assert.Assert(t, cs.needsClientFactoryRefresh(ctx, cluster, adminClient))
}
