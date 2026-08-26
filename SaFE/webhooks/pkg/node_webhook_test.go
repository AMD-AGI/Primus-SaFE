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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// validNode builds a node passing spec validation.
func validNode() *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Spec: v1.NodeSpec{
			NodeFlavor: &corev1.ObjectReference{Name: "flavor1"},
			Hostname:   pointer.String("host1"),
			PrivateIP:  "10.0.0.1",
			Port:       pointer.Int32(22),
			SSHSecret:  &corev1.ObjectReference{Name: "ssh"},
		},
	}
}

// gpuFlavor builds a node flavor with a GPU and ephemeral storage.
func gpuFlavor(name string) *v1.NodeFlavor {
	return &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
			Gpu: &v1.GpuChip{
				ResourceName: common.AmdGpu,
				Quantity:     resource.MustParse("8"),
				Product:      "MI300X",
			},
			ExtendResources: corev1.ResourceList{
				corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
			},
		},
	}
}

// TestNodeMutateSpec verifies hostname/ip/port normalization.
func TestNodeMutateSpec(t *testing.T) {
	m := &NodeMutator{}
	node := &v1.Node{Spec: v1.NodeSpec{PrivateIP: " 10.0.0.1 "}}
	m.mutateSpec(context.Background(), node)
	assert.Equal(t, node.Spec.PrivateIP, "10.0.0.1")
	assert.Assert(t, node.Spec.Hostname != nil)
	assert.Assert(t, node.Spec.Port != nil)
	assert.Equal(t, *node.Spec.Port, int32(DefaultNodePort))
}

// TestNodeMutateMeta verifies node name, labels and finalizer.
func TestNodeMutateMeta(t *testing.T) {
	m := &NodeMutator{}
	node := &v1.Node{Spec: v1.NodeSpec{Hostname: pointer.String("Host1")}}
	m.mutateMeta(context.Background(), node)
	assert.Equal(t, node.Name, "host1")
	assert.Equal(t, v1.GetNodeId(node), "host1")
}

// TestNodeMutateByNodeFlavor verifies gpu annotations/labels from node flavor.
func TestNodeMutateByNodeFlavor(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	m := &NodeMutator{Client: k8sClient}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{v1.NodeFlavorIdLabel: "flavor1"},
	}}
	changed := m.mutateByNodeFlavor(context.Background(), node)
	assert.Assert(t, changed)
	assert.Equal(t, v1.GetGpuResourceName(node), common.AmdGpu)

	// nil flavor returns false
	empty := &NodeMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.Assert(t, !empty.mutateByNodeFlavor(context.Background(), &v1.Node{}))
}

// TestNodeMutateTaints verifies taint clearing and timestamping.
func TestNodeMutateTaints(t *testing.T) {
	m := &NodeMutator{}
	unmanaged := &v1.Node{Spec: v1.NodeSpec{Taints: []corev1.Taint{{Key: "k"}}}}
	assert.Assert(t, m.mutateTaints(unmanaged))
	assert.Equal(t, len(unmanaged.Spec.Taints), 0)

	managed := &v1.Node{Spec: v1.NodeSpec{
		Cluster: pointer.String("c1"),
		Taints:  []corev1.Taint{{Key: "k", Effect: corev1.TaintEffectNoSchedule}},
	}}
	assert.Assert(t, m.mutateTaints(managed))
	assert.Assert(t, managed.Spec.Taints[0].TimeAdded != nil)
}

// TestNodeMutateOnCreation verifies the full create mutation path.
func TestNodeMutateOnCreation(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &NodeMutator{Client: k8sClient}
	node := validNode()
	assert.Assert(t, m.mutateOnCreation(context.Background(), node))
}

// TestNodeMutateOnUpdate verifies update mutation produces action annotations.
func TestNodeMutateOnUpdate(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &NodeMutator{Client: k8sClient}
	oldNode := validNode()
	newNode := validNode()
	v1.SetLabel(newNode, v1.NodeGpuCountLabel, "8")
	v1.SetAnnotation(newNode, v1.NodeDiskAnnotation, "disk")
	assert.Assert(t, m.mutateOnUpdate(context.Background(), newNode, oldNode))
}

// TestNodeMutatorHandle verifies the node mutator admission handler.
func TestNodeMutatorHandle(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &NodeMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, validNode(), nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, validNode(), nil))
	assert.Assert(t, resp.Allowed)
}

// TestNodeValidateNodeTaints verifies taint validation rules.
func TestNodeValidateNodeTaints(t *testing.T) {
	v := &NodeValidator{}
	ok := &v1.Node{Spec: v1.NodeSpec{
		Taints: []corev1.Taint{{Key: "k1", Effect: corev1.TaintEffectNoSchedule}},
	}}
	assert.NilError(t, v.validateNodeTaints(ok))

	dup := &v1.Node{Spec: v1.NodeSpec{Taints: []corev1.Taint{
		{Key: "k1", Effect: corev1.TaintEffectNoSchedule},
		{Key: "k1", Effect: corev1.TaintEffectNoSchedule},
	}}}
	assert.Assert(t, v.validateNodeTaints(dup) != nil)

	badEffect := &v1.Node{Spec: v1.NodeSpec{Taints: []corev1.Taint{{Key: "k1", Effect: "bad"}}}}
	assert.Assert(t, v.validateNodeTaints(badEffect) != nil)
}

// TestNodeValidateNodeFlavor verifies node flavor existence validation.
func TestNodeValidateNodeFlavor(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.Assert(t, v.validateNodeFlavor(ctx, &v1.Node{}) != nil)

	missing := &v1.Node{Spec: v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "x"}}}
	assert.Assert(t, v.validateNodeFlavor(ctx, missing) != nil)

	withFlavor := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(gpuFlavor("flavor1")).Build()}
	ok := &v1.Node{Spec: v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "flavor1"}}}
	assert.NilError(t, withFlavor.validateNodeFlavor(ctx, ok))
}

// TestNodeValidateNodeSSH verifies SSH secret presence validation.
func TestNodeValidateNodeSSH(t *testing.T) {
	v := &NodeValidator{}
	assert.Assert(t, v.validateNodeSSH(context.Background(), &v1.Node{}) != nil)
	ok := &v1.Node{Spec: v1.NodeSpec{SSHSecret: &corev1.ObjectReference{Name: "ssh"}}}
	assert.NilError(t, v.validateNodeSSH(context.Background(), ok))
}

// TestNodeValidateImmutableFields verifies hostname/cluster/workspace immutability.
func TestNodeValidateImmutableFields(t *testing.T) {
	v := &NodeValidator{}
	oldNode := &v1.Node{Spec: v1.NodeSpec{Hostname: pointer.String("h1")}}
	same := &v1.Node{Spec: v1.NodeSpec{Hostname: pointer.String("h1")}}
	assert.NilError(t, v.validateImmutableFields(same, oldNode))

	changed := &v1.Node{Spec: v1.NodeSpec{Hostname: pointer.String("h2")}}
	assert.Assert(t, v.validateImmutableFields(changed, oldNode) != nil)
}

// TestNodeValidateOnCreation verifies the full create validation path.
func TestNodeValidateOnCreation(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	v := &NodeValidator{Client: k8sClient}
	assert.NilError(t, v.validateOnCreation(context.Background(), validNode()))
}

// TestNodeValidateOnUpdate verifies the update validation path.
func TestNodeValidateOnUpdate(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	v := &NodeValidator{Client: k8sClient}
	assert.NilError(t, v.validateOnUpdate(context.Background(), validNode(), validNode()))
}

// TestGetNode verifies node retrieval helper.
func TestGetNode(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(validNode()).Build()

	got, err := getNode(ctx, k8sClient, "")
	assert.NilError(t, err)
	assert.Assert(t, got == nil)

	got, err = getNode(ctx, k8sClient, "node1")
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
}

// TestNodeValidatorHandle verifies the node validator admission handler.
func TestNodeValidatorHandle(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gpuFlavor("flavor1")).Build()
	v := &NodeValidator{Client: k8sClient, decoder: newDecoder(t)}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, validNode(), nil))
	assert.Assert(t, resp.Allowed)
}

// --- merged from node2_test.go ---

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
