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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestNodeFlavorMutateOnCreation verifies name normalization and zero-gpu clearing.
func TestNodeFlavorMutateOnCreation(t *testing.T) {
	m := &NodeFlavorMutator{}
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "MyFlavor"},
		Spec: v1.NodeFlavorSpec{
			Gpu: &v1.GpuChip{Quantity: resource.MustParse("0")},
		},
	}
	m.mutateOnCreation(nf)
	assert.Equal(t, nf.Name, "myflavor")
	assert.Assert(t, nf.Spec.Gpu == nil)
}

// TestNodeFlavorMutateExtendResources verifies extend resource key trimming.
func TestNodeFlavorMutateExtendResources(t *testing.T) {
	m := &NodeFlavorMutator{}
	nf := &v1.NodeFlavor{
		Spec: v1.NodeFlavorSpec{
			ExtendResources: map[corev1.ResourceName]resource.Quantity{
				" rdma ": resource.MustParse("1"),
				"   ":    resource.MustParse("2"),
			},
		},
	}
	m.mutateExtendResources(nf)
	_, ok := nf.Spec.ExtendResources["rdma"]
	assert.Assert(t, ok)
	assert.Equal(t, len(nf.Spec.ExtendResources), 1)
}

// TestNodeFlavorMutatorHandle verifies the node flavor mutator handler.
func TestNodeFlavorMutatorHandle(t *testing.T) {
	m := &NodeFlavorMutator{decoder: newDecoder(t)}
	nf := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "nf1"}}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, nf, nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, nf, nil))
	assert.Assert(t, resp.Allowed)
}

// TestNodeFlavorValidateCommon verifies cpu/memory/gpu/disk validation.
func TestNodeFlavorValidateCommon(t *testing.T) {
	v := &NodeFlavorValidator{}
	assert.Assert(t, v.validateCommon(&v1.NodeFlavor{}) != nil)

	valid := &v1.NodeFlavor{
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
			Gpu: &v1.GpuChip{
				ResourceName: common.AmdGpu,
				Quantity:     resource.MustParse("8"),
			},
		},
	}
	assert.NilError(t, v.validateCommon(valid))

	badGpu := &v1.NodeFlavor{
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
			Gpu:    &v1.GpuChip{ResourceName: "bad", Quantity: resource.MustParse("1")},
		},
	}
	assert.Assert(t, v.validateCommon(badGpu) != nil)
}

// TestNodeFlavorValidateImmutableFields verifies gpu immutability checks.
func TestNodeFlavorValidateImmutableFields(t *testing.T) {
	v := &NodeFlavorValidator{}
	withGpu := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Gpu: &v1.GpuChip{ResourceName: common.AmdGpu, Quantity: resource.MustParse("8")},
	}}
	withoutGpu := &v1.NodeFlavor{}
	assert.Assert(t, v.validateImmutableFields(withGpu, withoutGpu) != nil)
	assert.NilError(t, v.validateImmutableFields(withGpu, withGpu))

	changed := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Gpu: &v1.GpuChip{ResourceName: common.NvidiaGpu, Quantity: resource.MustParse("8")},
	}}
	assert.Assert(t, v.validateImmutableFields(withGpu, changed) != nil)
}

// TestIsValidGpuResource verifies gpu resource whitelist matching.
func TestIsValidGpuResource(t *testing.T) {
	assert.Assert(t, isValidGpuResource(common.AmdGpu))
	assert.Assert(t, isValidGpuResource(common.NvidiaGpu))
	assert.Assert(t, !isValidGpuResource("foo"))
}

// TestNodeFlavorValidatorHandle verifies the node flavor validator handler.
func TestNodeFlavorValidatorHandle(t *testing.T) {
	v := &NodeFlavorValidator{decoder: newDecoder(t)}
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "nf1"},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
			Memory: resource.MustParse("16Gi"),
		},
	}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, nf, nil))
	assert.Assert(t, resp.Allowed)
}
