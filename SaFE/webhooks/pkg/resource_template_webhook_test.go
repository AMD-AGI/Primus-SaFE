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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestResourceTemplateMutateOnCreation verifies name normalization.
func TestResourceTemplateMutateOnCreation(t *testing.T) {
	m := &ResourceTemplateMutator{}
	rt := &v1.ResourceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "MyRT"}}
	m.mutateOnCreation(rt)
	assert.Equal(t, rt.Name, "myrt")
}

// TestResourceTemplateMutatorHandle verifies the resource template mutator handler.
func TestResourceTemplateMutatorHandle(t *testing.T) {
	m := &ResourceTemplateMutator{decoder: newDecoder(t)}
	rt := &v1.ResourceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "rt1"}}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, rt, nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, rt, nil))
	assert.Assert(t, resp.Allowed)
}

// TestResourceTemplateValidate verifies kind/version validation.
func TestResourceTemplateValidate(t *testing.T) {
	v := &ResourceTemplateValidator{}
	assert.Assert(t, v.validate(&v1.ResourceTemplate{}) != nil)
	rt := &v1.ResourceTemplate{Spec: v1.ResourceTemplateSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: "PyTorchJob"},
	}}
	assert.Assert(t, v.validate(rt) != nil)
	rt.Spec.GroupVersionKind.Version = "v1"
	assert.NilError(t, v.validate(rt))
}

// TestResourceTemplateValidatorHandle verifies the resource template validator handler.
func TestResourceTemplateValidatorHandle(t *testing.T) {
	v := &ResourceTemplateValidator{decoder: newDecoder(t)}
	rt := &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "rt1"},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: "PyTorchJob", Version: "v1"},
		},
	}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, rt, nil))
	assert.Assert(t, resp.Allowed)

	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Delete, rt, nil))
	assert.Assert(t, resp.Allowed)
}
