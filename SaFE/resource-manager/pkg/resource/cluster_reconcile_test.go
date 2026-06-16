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
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestClusterReconcileNotFound(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         commonutils.NewObjectManager(),
	}
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestCleanupClusterResources(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := readyCluster("c1")
	r := newClusterReconcilerWithFactory(t, "c1", cs, cluster)
	// All deletes are no-ops on empty cluster.
	assert.NoError(t, r.cleanupClusterResources(context.Background(), cluster))
}

func TestResetNodesOfCluster(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "n1",
			Labels: map[string]string{v1.ClusterIdLabel: "c1"},
		},
	}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Node{}).
		WithObjects(node).
		Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	cluster := testCluster("c1")
	assert.NoError(t, r.resetNodesOfCluster(context.Background(), cluster))
	updated := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "n1"}, updated))
	assert.Nil(t, updated.Spec.Cluster)
}

func TestClusterDelete(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := testCluster("c1")
	cluster.Finalizers = []string{v1.ClusterFinalizer}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         commonutils.NewObjectManager(),
	}
	assert.NoError(t, r.delete(context.Background(), cluster))
}

func TestClusterReconcileReadyHappyPath(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := readyCluster("c1")
	cluster.Finalizers = []string{v1.ClusterFinalizer}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Cluster{}).
		WithObjects(cluster).
		Build()
	cs := k8sfake.NewSimpleClientset()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         mgr,
	}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
	assert.NoError(t, err)
	// Priority classes created in data plane.
	list, err := cs.SchedulingV1().PriorityClasses().List(context.Background(), metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, list.Items, 3)
}

func TestGuaranteeClientFactoryNotReady(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         commonutils.NewObjectManager(),
	}
	// Not ready -> no-op nil.
	assert.NoError(t, r.guaranteeClientFactory(context.Background(), testCluster("c1")))
}
