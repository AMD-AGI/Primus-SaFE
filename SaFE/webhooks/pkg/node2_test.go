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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// cpuFlavorWithDisk builds a non-gpu node flavor with data disk and ephemeral storage.
func cpuFlavorWithDisk(name string) *v1.NodeFlavor {
	return &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.NodeFlavorSpec{
			Cpu:      v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory:   resource.MustParse("16Gi"),
			DataDisk: &v1.DiskFlavor{Type: "nvme", Count: 2, Quantity: resource.MustParse("1Ti")},
			ExtendResources: corev1.ResourceList{
				corev1.ResourceEphemeralStorage: resource.MustParse("8000Gi"),
			},
		},
	}
}

// TestNodeMutateByNodeFlavorNoGpu covers gpu removal and disk annotation branches.
func TestNodeMutateByNodeFlavorNoGpu(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpuFlavorWithDisk("flavor1")).Build()
	m := &NodeMutator{Client: k8sClient}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Labels:      map[string]string{v1.NodeFlavorIdLabel: "flavor1", v1.NodeGpuCountLabel: "8"},
		Annotations: map[string]string{v1.GpuResourceNameAnnotation: "amd.com/gpu"},
	}}
	assert.Assert(t, m.mutateByNodeFlavor(context.Background(), node))
	assert.Equal(t, v1.GetGpuResourceName(node), "")
	assert.Assert(t, v1.HasAnnotation(node, v1.NodeDiskAnnotation))
}

// TestNodeMutateLabelsSubnetRemove covers subnet annotation removal branch.
func TestNodeMutateLabelsSubnetRemove(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &NodeMutator{Client: k8sClient}
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{v1.NodeSubnetAnnotation: "10.0.0.0/16"}},
		Spec:       v1.NodeSpec{Cluster: pointer.String("cluster1"), Hostname: pointer.String("h1")},
	}
	m.mutateLabels(context.Background(), node)
	assert.Assert(t, !v1.HasAnnotation(node, v1.NodeSubnetAnnotation))
}

// TestNodeValidateNodeSpecErrors covers node spec validation error branches.
func TestNodeValidateNodeSpecErrors(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	v := &NodeValidator{Client: k8sClient}

	noPrivateIP := validNode()
	noPrivateIP.Spec.PrivateIP = ""
	assert.Assert(t, v.validateNodeSpec(context.Background(), noPrivateIP) != nil)

	badPort := validNode()
	badPort.Spec.Port = pointer.Int32(0)
	assert.Assert(t, v.validateNodeSpec(context.Background(), badPort) != nil)

	noSSH := validNode()
	noSSH.Spec.SSHSecret = nil
	assert.Assert(t, v.validateNodeSpec(context.Background(), noSSH) != nil)
}

// TestNodeValidateImmutableFieldsBranches covers immutable field error branches.
func TestNodeValidateImmutableFieldsBranches(t *testing.T) {
	v := &NodeValidator{}
	base := func() *v1.Node {
		return &v1.Node{Spec: v1.NodeSpec{
			Hostname:  pointer.String("h1"),
			Cluster:   pointer.String("cluster1"),
			Workspace: pointer.String("ws1"),
			PrivateIP: "10.0.0.1",
		}}
	}
	clusterChanged := base()
	clusterChanged.Spec.Cluster = pointer.String("cluster2")
	assert.Assert(t, v.validateImmutableFields(clusterChanged, base()) != nil)

	wsChanged := base()
	wsChanged.Spec.Workspace = pointer.String("ws2")
	assert.Assert(t, v.validateImmutableFields(wsChanged, base()) != nil)
}

// TestNodeMutatorHandleDecodeError covers the node mutator decode-error branch.
func TestNodeMutatorHandleDecodeError(t *testing.T) {
	scheme := newScheme(t)
	m := &NodeMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}
