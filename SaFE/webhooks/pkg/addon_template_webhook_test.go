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

// TestAddOnTemplateMutateOnCreation verifies default type and action trimming.
func TestAddOnTemplateMutateOnCreation(t *testing.T) {
	m := &AddOnTemplateMutator{}
	addon := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "MyAddon"},
		Spec:       v1.AddonTemplateSpec{Action: "  do-it  "},
	}
	m.mutateOnCreation(addon)
	assert.Equal(t, addon.Name, "myaddon")
	assert.Equal(t, addon.Spec.Type, v1.AddonTemplateDefault)
	assert.Equal(t, addon.Spec.Action, "do-it")
}

// TestAddOnTemplateMutatorHandle verifies the addon template mutator handler.
func TestAddOnTemplateMutatorHandle(t *testing.T) {
	m := &AddOnTemplateMutator{decoder: newDecoder(t)}
	addon := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "addon1"},
		Spec:       v1.AddonTemplateSpec{Action: "echo"},
	}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, addon, nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, addon, nil))
	assert.Assert(t, resp.Allowed)
}

// TestAddOnTemplateValidateRequiredParams verifies required params per template type.
func TestAddOnTemplateValidateRequiredParams(t *testing.T) {
	v := &AddOnTemplateValidator{}
	assert.Assert(t, v.validateRequiredParams(&v1.AddonTemplate{
		Spec: v1.AddonTemplateSpec{Type: v1.AddonTemplateDefault},
	}) != nil)
	assert.NilError(t, v.validateRequiredParams(&v1.AddonTemplate{
		Spec: v1.AddonTemplateSpec{Type: v1.AddonTemplateDefault, Action: "echo"},
	}))
	assert.NilError(t, v.validateRequiredParams(&v1.AddonTemplate{
		Spec: v1.AddonTemplateSpec{Type: v1.AddonTemplateHelm},
	}))
	assert.Assert(t, v.validateRequiredParams(&v1.AddonTemplate{
		Spec: v1.AddonTemplateSpec{Type: "invalid"},
	}) != nil)
}

// TestAddOnTemplateValidate verifies the combined validate path.
func TestAddOnTemplateValidate(t *testing.T) {
	v := &AddOnTemplateValidator{}
	addon := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "addon1"},
		Spec:       v1.AddonTemplateSpec{Type: v1.AddonTemplateDefault, Action: "echo"},
	}
	assert.NilError(t, v.validate(addon))
}

// TestAddOnTemplateValidatorHandle verifies the addon template validator handler.
func TestAddOnTemplateValidatorHandle(t *testing.T) {
	v := &AddOnTemplateValidator{decoder: newDecoder(t)}
	addon := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "addon1"},
		Spec:       v1.AddonTemplateSpec{Type: v1.AddonTemplateDefault, Action: "echo"},
	}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, addon, nil))
	assert.Assert(t, resp.Allowed)

	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Delete, addon, nil))
	assert.Assert(t, resp.Allowed)
}
