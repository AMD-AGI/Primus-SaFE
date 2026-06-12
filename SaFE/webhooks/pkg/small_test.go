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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// TestHandlersDecodeErrors covers decode-error branches across webhooks.
func TestHandlersDecodeErrors(t *testing.T) {
	d := newDecoder(t)
	bad := newRequest(t, admissionv1.Create, nil, nil)

	assert.Assert(t, !(&AddOnTemplateMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&AddOnTemplateValidator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&ResourceTemplateMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&ResourceTemplateValidator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&RoleMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&RoleValidator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&NodeFlavorMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&NodeFlavorValidator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&FaultMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&ClusterMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
}

// TestHandlersDeletionTimestamp covers deletion-timestamp short-circuit branches.
func TestHandlersDeletionTimestamp(t *testing.T) {
	d := newDecoder(t)
	now := metav1.Now()
	withDeletion := func(o interface{ SetDeletionTimestamp(*metav1.Time); SetFinalizers([]string) }) {
		o.SetDeletionTimestamp(&now)
		o.SetFinalizers([]string{"x"})
	}

	addon := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "a"}}
	withDeletion(addon)
	assert.Assert(t, (&AddOnTemplateValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, addon, nil)).Allowed)

	rt := &v1.ResourceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
	withDeletion(rt)
	assert.Assert(t, (&ResourceTemplateValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, rt, nil)).Allowed)

	role := &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
	withDeletion(role)
	assert.Assert(t, (&RoleValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, role, nil)).Allowed)

	nf := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "nf"}}
	withDeletion(nf)
	assert.Assert(t, (&NodeFlavorValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, nf, nil)).Allowed)

	cl := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}}
	withDeletion(cl)
	assert.Assert(t, (&ClusterValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Create, cl, nil)).Allowed)
	assert.Assert(t, (&ClusterValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, cl, cl)).Allowed)
}

// TestOptionsInitFlagsNil covers the nil receiver branch.
func TestOptionsInitFlagsNil(t *testing.T) {
	var opt *Options
	assert.Assert(t, opt.InitFlags() != nil)
}

// TestNodeFlavorValidateCommonQuantities covers memory/gpu quantity error branches.
func TestNodeFlavorValidateCommonQuantities(t *testing.T) {
	v := &NodeFlavorValidator{}
	badMem := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{Cpu: v1.CpuChip{Quantity: resource.MustParse("8")}}}
	assert.Assert(t, v.validateCommon(badMem) != nil)

	badGpuQty := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
		Memory: resource.MustParse("16Gi"),
		Gpu:    &v1.GpuChip{ResourceName: common.AmdGpu, Quantity: resource.MustParse("0")},
	}}
	assert.Assert(t, v.validateCommon(badGpuQty) != nil)
}

// TestNodeFlavorImmutableProductQuantity covers gpu product/quantity immutability branches.
func TestNodeFlavorImmutableProductQuantity(t *testing.T) {
	v := &NodeFlavorValidator{}
	old := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{Gpu: &v1.GpuChip{
		ResourceName: common.AmdGpu, Quantity: resource.MustParse("8"), Product: "MI300X",
	}}}
	qtyChanged := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{Gpu: &v1.GpuChip{
		ResourceName: common.AmdGpu, Quantity: resource.MustParse("4"), Product: "MI300X",
	}}}
	assert.Assert(t, v.validateImmutableFields(old, qtyChanged) != nil)

	productChanged := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{Gpu: &v1.GpuChip{
		ResourceName: common.AmdGpu, Quantity: resource.MustParse("8"), Product: "MI325X",
	}}}
	assert.Assert(t, v.validateImmutableFields(old, productChanged) != nil)
}

// TestNodeFlavorValidateCommonRdma covers the rdma extend-resource error branch.
func TestNodeFlavorValidateCommonRdma(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	v := &NodeFlavorValidator{}
	nf := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
		Memory: resource.MustParse("16Gi"),
		ExtendResources: corev1.ResourceList{
			"rdma/hca": resource.MustParse("0"),
		},
	}}
	assert.Assert(t, v.validateCommon(nf) != nil)
}

// TestClusterValidateOnCreationLabels covers the label validation error branch.
func TestClusterValidateOnCreationLabels(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: c}
	cl := validControlPlaneCluster()
	v1.SetLabel(cl, v1.DisplayNameLabel, "my-cluster")
	cl.Labels["Bad Key"] = "v"
	assert.Assert(t, v.validateOnCreation(context.Background(), cl) != nil)
}

// TestGetClusterError covers the cluster retrieval error path.
func TestGetClusterError(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := getCluster(context.Background(), c, "missing")
	assert.Assert(t, err != nil)
}

// TestFaultValidateOnCreationDisplayName covers display name validation in fault creation.
func TestFaultValidateOnCreationDisplayName(t *testing.T) {
	v := &FaultValidator{}
	fault := &v1.Fault{Spec: v1.FaultSpec{MonitorId: "m1"}}
	v1.SetLabel(fault, v1.DisplayNameLabel, "Bad_Name")
	assert.Assert(t, v.validateOnCreation(fault) != nil)
}
