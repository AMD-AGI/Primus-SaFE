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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// TestUserMutateLabelsWithEmailName covers email/name md5 label assignment.
func TestUserMutateLabelsWithEmailName(t *testing.T) {
	m := &UserMutator{}
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}, Spec: v1.UserSpec{Type: v1.DefaultUserType}}
	v1.SetAnnotation(user, v1.UserEmailAnnotation, "a@b.com")
	v1.SetAnnotation(user, v1.UserNameAnnotation, "alice")
	m.mutateLabels(user)
	assert.Assert(t, v1.GetLabel(user, v1.UserEmailMd5Label) != "")
	assert.Assert(t, v1.GetLabel(user, v1.UserNameMd5Label) != "")
}

// TestUserMutateWorkspaceDedup covers workspace dedup and non-existent removal.
func TestUserMutateWorkspaceDedup(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	m := &UserMutator{Client: k8sClient}
	user := validUser("u1")
	commonuser.AssignWorkspace(user, "ws1", "ws1", "missing")
	m.mutateWorkspace(context.Background(), user)
	assert.Assert(t, commonuser.HasWorkspaceRight(user, "ws1"))
	assert.Assert(t, !commonuser.HasWorkspaceRight(user, "missing"))
}

// TestUserMutateManagedWorkspacesNoRight covers managed workspace access filtering.
func TestUserMutateManagedWorkspacesNoRight(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	m := &UserMutator{Client: k8sClient}
	user := validUser("u1")
	commonuser.AssignManagedWorkspace(user, "ws1")
	// user has no access right to ws1, so managed should be filtered out
	m.mutateManagedWorkspaces(context.Background(), user, true)
	assert.Assert(t, !commonuser.HasWorkspaceManagedRight(user, "ws1"))
}

// TestUserValidateImmutableNameChange covers default-user name immutability branch.
func TestUserValidateImmutableNameChange(t *testing.T) {
	v := &UserValidator{}
	oldUser := validUser("u1")
	v1.SetAnnotation(oldUser, v1.UserNameAnnotation, "alice")
	newUser := validUser("u1")
	v1.SetAnnotation(newUser, v1.UserNameAnnotation, "bob")
	assert.Assert(t, v.validateImmutableFields(newUser, oldUser) != nil)
}

// TestUserMutatorHandleDecodeError covers the user mutator decode-error branch.
func TestUserMutatorHandleDecodeError(t *testing.T) {
	scheme := newScheme(t)
	m := &UserMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestUserValidatorHandleDecodeError covers the user validator decode-error branch.
func TestUserValidatorHandleDecodeError(t *testing.T) {
	scheme := newScheme(t)
	v := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}
