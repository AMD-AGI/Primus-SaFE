/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestClusterValidateOnCreationBadDisplayName covers the display name error branch.
func TestClusterValidateOnCreationBadDisplayName(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: c}
	cl := validControlPlaneCluster()
	v1.SetLabel(cl, v1.DisplayNameLabel, "Bad_Name")
	assert.Assert(t, v.validateOnCreation(context.Background(), cl) != nil)
}

// TestClusterValidateControlPlaneNodesInUse covers the nodes-in-use error branch.
func TestClusterValidateControlPlaneNodesInUse(t *testing.T) {
	scheme := newScheme(t)
	other := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "other"},
		Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(other, readyNode("node1")).Build()
	v := &ClusterValidator{Client: c}
	assert.Assert(t, v.validateControlPlane(context.Background(), validControlPlaneCluster()) != nil)
}

// TestClusterValidateControlPlaneNodesNotReady covers the nodes-not-ready error branch.
func TestClusterValidateControlPlaneNodesNotReady(t *testing.T) {
	scheme := newScheme(t)
	notReady := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(notReady).Build()
	v := &ClusterValidator{Client: c}
	assert.Assert(t, v.validateControlPlane(context.Background(), validControlPlaneCluster()) != nil)
}

// TestClusterValidateOnUpdateBadLabels covers the update label validation branch.
func TestClusterValidateOnUpdateBadLabels(t *testing.T) {
	v := &ClusterValidator{}
	oldCluster := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	newCluster := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	newCluster.Labels = map[string]string{"Bad Key": "v"}
	assert.Assert(t, v.validateOnUpdate(newCluster, oldCluster) != nil)
}

// TestNodeValidateCommonBadDisplayName covers display name validation in node common.
func TestNodeValidateCommonBadDisplayName(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	node := validNode()
	v1.SetLabel(node, v1.DisplayNameLabel, "Bad_Name")
	assert.Assert(t, v.validateCommon(context.Background(), node) != nil)
}

// TestNodeValidateTaintsBadLabelKey covers the taint key validation branch.
func TestNodeValidateTaintsBadLabelKey(t *testing.T) {
	v := &NodeValidator{}
	node := &v1.Node{Spec: v1.NodeSpec{Taints: []corev1.Taint{
		{Key: "Bad Key", Effect: corev1.TaintEffectNoSchedule},
	}}}
	assert.Assert(t, v.validateNodeTaints(node) != nil)
}

// TestNodeValidateImmutablePrivateIP covers the control-plane private IP immutability branch.
func TestNodeValidateImmutablePrivateIP(t *testing.T) {
	v := &NodeValidator{}
	oldNode := &v1.Node{Spec: v1.NodeSpec{Hostname: pointer.String("h1"), PrivateIP: "1.1.1.1"}}
	newNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.KubernetesControlPlane: ""}},
		Spec: v1.NodeSpec{Hostname: pointer.String("h1"), PrivateIP: "2.2.2.2"}}
	assert.Assert(t, v.validateImmutableFields(newNode, oldNode) != nil)
}

// TestNodeValidatorHandleDeletion covers the node validator deletion-timestamp branch.
func TestNodeValidatorHandleDeletion(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	now := metav1.Now()
	node := validNode()
	node.DeletionTimestamp = &now
	node.Finalizers = []string{"x"}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, node, node))
	assert.Assert(t, resp.Allowed)
}

// TestUserValidatorHandleDeletion covers the user validator deletion-timestamp branch.
func TestUserValidatorHandleDeletion(t *testing.T) {
	scheme := newScheme(t)
	v := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	now := metav1.Now()
	user := validUser("u1")
	user.DeletionTimestamp = &now
	user.Finalizers = []string{"x"}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, user, user))
	assert.Assert(t, resp.Allowed)
}

// TestUserValidateOnCreationReserved covers reserved-name validation in user creation.
func TestUserValidateOnCreationReserved(t *testing.T) {
	scheme := newScheme(t)
	v := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	user := validUser(common.UserSelf)
	assert.Assert(t, v.validateOnCreation(context.Background(), user) != nil)
}
