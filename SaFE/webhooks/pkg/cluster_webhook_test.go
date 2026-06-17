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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// readyNode builds a managed and ready node for cluster validation tests.
func readyNode(name string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1.NodeStatus{
			MachineStatus: v1.MachineStatus{Phase: v1.NodeReady},
		},
	}
}

// validControlPlaneCluster builds a cluster spec that passes control plane validation.
func validControlPlaneCluster() *v1.Cluster {
	return &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
		Spec: v1.ClusterSpec{
			ControlPlane: v1.ControlPlane{
				Nodes:              []string{"node1"},
				KubePodsSubnet:     pointer.String("10.0.0.0/16"),
				KubeServiceAddress: pointer.String("10.1.0.0/16"),
				NodeLocalDNSIP:     pointer.String("169.254.25.10"),
				KubeSprayImage:     pointer.String("kubespray:latest"),
			},
		},
	}
}

// TestClusterMutateOnCreation verifies default network plugin and finalizer.
func TestClusterMutateOnCreation(t *testing.T) {
	m := &ClusterMutator{}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "MyCluster"}}
	m.mutateOnCreation(context.Background(), cluster)
	assert.Equal(t, cluster.Name, "mycluster")
	assert.Assert(t, cluster.Spec.ControlPlane.KubeNetworkPlugin != nil)
	assert.Equal(t, *cluster.Spec.ControlPlane.KubeNetworkPlugin, v1.FlannelNetworkPlugin)
}

// TestClusterMutatorHandle verifies the cluster mutator admission handler.
func TestClusterMutatorHandle(t *testing.T) {
	m := &ClusterMutator{decoder: newDecoder(t)}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, cluster, nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, cluster, nil))
	assert.Assert(t, resp.Allowed)
}

// TestClusterValidateControlPlane verifies control plane validation logic.
func TestClusterValidateControlPlane(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	emptyValidator := &ClusterValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.Assert(t, emptyValidator.validateControlPlane(ctx, &v1.Cluster{}) != nil)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: k8sClient}
	assert.NilError(t, v.validateControlPlane(ctx, validControlPlaneCluster()))
}

// TestClusterValidateNodesInUse verifies node-in-use detection.
func TestClusterValidateNodesInUse(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	existing := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "other"},
		Spec:       v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &ClusterValidator{Client: k8sClient}

	conflict := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	assert.Assert(t, v.validateNodesInUse(ctx, conflict) != nil)

	ok := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node2"}}}}
	assert.NilError(t, v.validateNodesInUse(ctx, ok))
}

// TestClusterValidateNodesReady verifies node readiness validation.
func TestClusterValidateNodesReady(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	notReady := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(notReady).Build()
	v := &ClusterValidator{Client: k8sClient}
	cluster := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	assert.Assert(t, v.validateNodesReady(ctx, cluster) != nil)
}

// TestClusterValidateOnCreation verifies create-time validation.
func TestClusterValidateOnCreation(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: k8sClient}
	assert.NilError(t, v.validateOnCreation(context.Background(), validControlPlaneCluster()))
}

// TestClusterValidateImmutableFields verifies control plane node immutability.
func TestClusterValidateImmutableFields(t *testing.T) {
	v := &ClusterValidator{}
	oldCluster := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	same := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	assert.NilError(t, v.validateImmutableFields(same, oldCluster))

	changed := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node2"}}}}
	assert.Assert(t, v.validateImmutableFields(changed, oldCluster) != nil)
}

// TestClusterValidateOnUpdate verifies update-time validation.
func TestClusterValidateOnUpdate(t *testing.T) {
	v := &ClusterValidator{}
	oldCluster := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	same := &v1.Cluster{Spec: v1.ClusterSpec{ControlPlane: v1.ControlPlane{Nodes: []string{"node1"}}}}
	assert.NilError(t, v.validateOnUpdate(same, oldCluster))
}

// TestGetCluster verifies cluster retrieval helper.
func TestGetCluster(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

	got, err := getCluster(ctx, k8sClient, "")
	assert.NilError(t, err)
	assert.Assert(t, got == nil)

	got, err = getCluster(ctx, k8sClient, "cluster1")
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
}

// TestClusterValidatorHandle verifies the cluster validator admission handler.
func TestClusterValidatorHandle(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: k8sClient, decoder: newDecoder(t)}

	cluster := validControlPlaneCluster()
	v1.SetLabel(cluster, v1.DisplayNameLabel, "my-cluster")
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, cluster, nil))
	assert.Assert(t, resp.Allowed)
}
