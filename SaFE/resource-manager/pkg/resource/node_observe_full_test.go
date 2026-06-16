/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func newNodeReconcilerFull(t *testing.T, cs *k8sfake.Clientset, objs ...ctrlclient.Object) *NodeReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Node{}).WithObjects(objs...).Build()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	return &NodeReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs},
		clientManager:         mgr,
	}
}

func TestNodeObserveCleanChain(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newNodeReconcilerFull(t, cs)
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	_, err := r.observe(context.Background(), adminNode, nil)
	assert.NoError(t, err)

	// drive the individual observe helpers directly for a clean node
	clean := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}}
	for _, f := range []func(context.Context, *v1.Node, *corev1.Node) (bool, error){
		r.observeTaints, r.observeLabelAction, r.observeAnnotationAction, r.observeWorkspace, r.observeCluster,
	} {
		ok, err := f(context.Background(), clean, nil)
		assert.NoError(t, err)
		assert.True(t, ok)
	}
}

func TestDeleteK8sNodeFull(t *testing.T) {
	ctx := context.Background()

	// empty cluster/k8s name -> no-op
	cs0 := k8sfake.NewSimpleClientset()
	r0 := newNodeReconcilerFull(t, cs0)
	_, err := r0.deleteK8sNode(ctx, &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n0"}})
	assert.NoError(t, err)

	// with a valid factory and k8s node name -> deletes the node
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "kn1"}}
	cs := k8sfake.NewSimpleClientset(k8sNode)
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	v1.SetLabel(adminNode, v1.ClusterIdLabel, "c1")
	adminNode.Status.MachineStatus.HostName = "kn1"
	r := newNodeReconcilerFull(t, cs, adminNode)
	_, err = r.deleteK8sNode(ctx, adminNode)
	assert.NoError(t, err)
	_, err = cs.CoreV1().Nodes().Get(ctx, "kn1", metav1.GetOptions{})
	assert.Error(t, err)
}
