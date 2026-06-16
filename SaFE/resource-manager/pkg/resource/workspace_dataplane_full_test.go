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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func newWorkspaceReconcilerFull(t *testing.T, cs *k8sfake.Clientset, objs ...ctrlclient.Object) *WorkspaceReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Workspace{}).WithObjects(objs...).Build()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	return &WorkspaceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs},
		clientManager:         mgr,
		expectations:          map[string]sets.Set{},
		option:                &WorkspaceReconcilerOption{},
	}
}

func TestGuaranteeAndDeleteDataPlaneResourcesFull(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	r := newWorkspaceReconcilerFull(t, cs, cluster, ws)
	ctx := context.Background()

	assert.NoError(t, r.guaranteeDataPlaneResources(ctx, ws, cs))
	_, err := cs.CoreV1().Namespaces().Get(ctx, "ws1", metav1.GetOptions{})
	assert.NoError(t, err)

	assert.NoError(t, r.deleteDataPlaneResources(ctx, ws))
	_, err = cs.CoreV1().Namespaces().Get(ctx, "ws1", metav1.GetOptions{})
	assert.Error(t, err)
}

func TestWorkspaceReconcileActive(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	r := newWorkspaceReconcilerFull(t, cs, cluster, ws)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "ws1"}})
	assert.NoError(t, err)
}

func TestWorkspaceProcessWorkspaceNoFlavor(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	r := newWorkspaceReconcilerFull(t, cs, cluster, ws)
	_, err := r.processWorkspace(context.Background(), ws)
	assert.NoError(t, err)
}

func TestGetClientSetOfDataplaneWorkspace(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newWorkspaceReconcilerFull(t, cs, testCluster("c1"))
	ctx := context.Background()

	got, err := r.getClientSetOfDataplane(ctx, "")
	assert.NoError(t, err)
	assert.Nil(t, got)

	got, err = r.getClientSetOfDataplane(ctx, "c1")
	assert.NoError(t, err)
	assert.NotNil(t, got)
}
