/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func newNodeK8sReconciler(t *testing.T, objs ...client.Object) *NodeK8sReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Node{}).
		WithObjects(objs...).
		Build()
	return &NodeK8sReconciler{
		ctx:                   context.Background(),
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         commonutils.NewObjectManager(),
	}
}

func TestNodeK8sReconcileNoop(t *testing.T) {
	r := newNodeK8sReconciler(t)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestIsConcernedLabelsEqual(t *testing.T) {
	a := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	b := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	assert.True(t, isConcernedLabelsEqual(a, b))
	c := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "c2"}}}
	assert.False(t, isConcernedLabelsEqual(a, c))
}

func TestGenFaultNodeByMessage(t *testing.T) {
	fn := genFaultNodeByMessage(&nodeQueueMessage{adminNodeName: "n1", clusterName: "c1"})
	assert.Equal(t, "n1", fn.AdminName)
	assert.Equal(t, "c1", fn.ClusterName)
}

func TestDeleteConcernedMeta(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	assert.True(t, deleteConcernedMeta(node))
	// Nothing left -> false.
	assert.False(t, deleteConcernedMeta(&v1.Node{}))
}

func TestNodeK8sIsRelevantFieldChanged(t *testing.T) {
	r := newNodeK8sReconciler(t)
	old := &corev1.Node{}
	same := old.DeepCopy()
	assert.False(t, r.isRelevantFieldChanged(old, same))

	changed := old.DeepCopy()
	changed.Spec.Unschedulable = true
	assert.True(t, r.isRelevantFieldChanged(old, changed))
}

func TestNodeK8sDoMissingNode(t *testing.T) {
	r := newNodeK8sReconciler(t)
	_, err := r.Do(context.Background(), &nodeQueueMessage{adminNodeName: "missing", action: NodeUpdate})
	assert.NoError(t, err)
}

func TestNodeK8sDoUnmanaged(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	r := newNodeK8sReconciler(t, node)
	_, err := r.Do(context.Background(), &nodeQueueMessage{adminNodeName: "n1", action: NodeUnmanaged})
	assert.NoError(t, err)
	updated := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "n1"}, updated))
	assert.Equal(t, "", v1.GetClusterId(updated))
}

func TestHandleNodeUnmanaged(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.WorkspaceIdLabel: "ws1"},
	}}
	r := newNodeK8sReconciler(t, node)
	err := r.handleNodeUnmanaged(context.Background(), &nodeQueueMessage{clusterName: "c1"}, node)
	assert.NoError(t, err)
	assert.True(t, node.Status.Unschedulable)
}

func TestSyncK8sMetadata(t *testing.T) {
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	r := newNodeK8sReconciler(t, adminNode)
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.WorkspaceIdLabel: "ws1"},
	}}
	err := r.syncK8sMetadata(context.Background(), adminNode, k8sNode)
	assert.NoError(t, err)
	assert.Equal(t, "c1", v1.GetClusterId(adminNode))
}

func TestSyncK8sStatus(t *testing.T) {
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	r := newNodeK8sReconciler(t, adminNode)
	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec:       corev1.NodeSpec{Unschedulable: true},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
	err := r.syncK8sStatus(context.Background(), adminNode, k8sNode)
	assert.NoError(t, err)
	assert.True(t, adminNode.Status.Unschedulable)
}

func TestProcessFaultNoConfigmap(t *testing.T) {
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	r := newNodeK8sReconciler(t, adminNode)
	// No fault configmap -> nil.
	err := r.processFault(context.Background(), adminNode, &nodeQueueMessage{adminNodeName: "n1"})
	assert.NoError(t, err)
}

func TestHandleNodeUpdateViaInformer(t *testing.T) {
	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	r := newNodeK8sReconciler(t, adminNode)

	cs := k8sfake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "k8s-n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}})
	patches := gomonkey.ApplyFunc(rmutils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs), nil
		})
	defer patches.Reset()

	msg := &nodeQueueMessage{clusterName: "c1", k8sNodeName: "k8s-n1", adminNodeName: "n1", action: NodeUpdate}
	err := r.handleNodeUpdate(context.Background(), msg, adminNode)
	assert.NoError(t, err)
}
