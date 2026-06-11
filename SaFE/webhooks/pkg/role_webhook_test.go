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

// TestRoleMutateOnCreation verifies role name normalization and resource lowercasing.
func TestRoleMutateOnCreation(t *testing.T) {
	m := &RoleMutator{}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "MyRole"},
		Rules: []v1.PolicyRule{
			{Resources: []string{"Workloads", "NODES"}, Verbs: []v1.RoleVerb{v1.GetVerb}},
		},
	}
	m.mutateOnCreation(role)
	assert.Equal(t, role.Rules[0].Resources[0], "workloads")
	assert.Equal(t, role.Rules[0].Resources[1], "nodes")
}

// TestRoleMutatorHandle verifies the role mutator admission handler.
func TestRoleMutatorHandle(t *testing.T) {
	m := &RoleMutator{decoder: newDecoder(t)}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "role1"},
		Rules:      []v1.PolicyRule{{Resources: []string{"WL"}, Verbs: []v1.RoleVerb{v1.GetVerb}}},
	}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, role, nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, role, nil))
	assert.Assert(t, resp.Allowed)
}

// TestRoleValidate verifies role rule validation.
func TestRoleValidate(t *testing.T) {
	v := &RoleValidator{}
	assert.Assert(t, v.validate(&v1.Role{}) != nil)
	assert.Assert(t, v.validate(&v1.Role{Rules: []v1.PolicyRule{{Verbs: []v1.RoleVerb{v1.GetVerb}}}}) != nil)
	assert.Assert(t, v.validate(&v1.Role{Rules: []v1.PolicyRule{{Resources: []string{"wl"}}}}) != nil)
	assert.NilError(t, v.validate(&v1.Role{
		Rules: []v1.PolicyRule{{Resources: []string{"wl"}, Verbs: []v1.RoleVerb{v1.GetVerb}}},
	}))
}

// TestRoleValidatorHandle verifies the role validator admission handler.
func TestRoleValidatorHandle(t *testing.T) {
	v := &RoleValidator{decoder: newDecoder(t)}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "role1"},
		Rules:      []v1.PolicyRule{{Resources: []string{"wl"}, Verbs: []v1.RoleVerb{v1.GetVerb}}},
	}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, role, nil))
	assert.Assert(t, resp.Allowed)

	bad := &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: "role1"}}
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, bad, nil))
	assert.Assert(t, !resp.Allowed)
}
