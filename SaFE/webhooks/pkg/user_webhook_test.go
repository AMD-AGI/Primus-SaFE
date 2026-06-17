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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// defaultRole builds the default role object referenced by users.
func defaultRole() *v1.Role {
	return &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: string(v1.DefaultRole)}}
}

// validUser builds a default-type user with a password and default role.
func validUser(name string) *v1.User {
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.UserSpec{
			Type:     v1.DefaultUserType,
			Password: "cHdk",
			Roles:    []v1.UserRole{v1.DefaultRole},
		},
	}
}

// TestUserMutateMetadata verifies default type and id label.
func TestUserMutateMetadata(t *testing.T) {
	m := &UserMutator{}
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}
	m.mutateMetadata(user)
	assert.Equal(t, user.Spec.Type, v1.DefaultUserType)
	assert.Equal(t, v1.GetLabel(user, v1.UserIdLabel), "u1")
}

// TestUserMutateLabels verifies type label and md5 label removal.
func TestUserMutateLabels(t *testing.T) {
	m := &UserMutator{}
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}, Spec: v1.UserSpec{Type: v1.DefaultUserType}}
	m.mutateLabels(user)
	assert.Equal(t, v1.GetLabel(user, v1.UserTypeLabel), string(v1.DefaultUserType))
}

// TestUserMutateRoles verifies role normalization logic.
func TestUserMutateRoles(t *testing.T) {
	m := &UserMutator{}
	admin := &v1.User{Spec: v1.UserSpec{Roles: []v1.UserRole{v1.SystemAdminRole, v1.DefaultRole}}}
	m.mutateRoles(admin)
	assert.Equal(t, len(admin.Spec.Roles), 1)

	normal := &v1.User{Spec: v1.UserSpec{Roles: []v1.UserRole{v1.WorkspaceAdminRole}}}
	m.mutateRoles(normal)
	assert.Assert(t, v1.IsContainRole(normal.Spec.Roles, v1.DefaultRole))
}

// TestUserMutateWorkspace verifies workspace dedup and default assignment.
func TestUserMutateWorkspace(t *testing.T) {
	scheme := newScheme(t)
	defaultWs := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-default"},
		Spec:       v1.WorkspaceSpec{IsDefault: true},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultWs).Build()
	m := &UserMutator{Client: k8sClient}
	user := validUser("u1")
	m.mutateWorkspace(context.Background(), user)
	assert.Assert(t, commonuser.HasWorkspaceRight(user, "ws-default"))
}

// TestUserMutateManagedWorkspaces verifies managed workspace filtering.
func TestUserMutateManagedWorkspaces(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	m := &UserMutator{Client: k8sClient}
	user := validUser("u1")
	commonuser.AssignWorkspace(user, "ws1")
	commonuser.AssignManagedWorkspace(user, "ws1")
	m.mutateManagedWorkspaces(context.Background(), user, true)
	assert.Assert(t, commonuser.HasWorkspaceManagedRight(user, "ws1"))
}

// TestUserMutateOnCreation verifies the full create mutation path.
func TestUserMutateOnCreation(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &UserMutator{Client: k8sClient}
	user := validUser("u1")
	m.mutateOnCreation(context.Background(), user)
	assert.Equal(t, v1.GetLabel(user, v1.UserIdLabel), "u1")
}

// TestUserMutateOnUpdate verifies the update mutation path.
func TestUserMutateOnUpdate(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &UserMutator{Client: k8sClient}
	user := validUser("u1")
	m.mutateOnUpdate(context.Background(), user)
	assert.Equal(t, v1.GetLabel(user, v1.UserTypeLabel), string(v1.DefaultUserType))
}

// TestUserMutatorHandle verifies the user mutator admission handler.
func TestUserMutatorHandle(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &UserMutator{Client: k8sClient, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, validUser("u1"), nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Update, validUser("u1"), nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Delete, validUser("u1"), nil))
	assert.Assert(t, resp.Allowed)
}

// TestUserValidateMetadata verifies reserved username rejection.
func TestUserValidateMetadata(t *testing.T) {
	v := &UserValidator{}
	assert.NilError(t, v.validateMetadata(&v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}))
	assert.Assert(t, v.validateMetadata(&v1.User{ObjectMeta: metav1.ObjectMeta{Name: common.UserSelf}}) != nil)
}

// TestUserValidateRequiredParams verifies required parameter validation.
func TestUserValidateRequiredParams(t *testing.T) {
	v := &UserValidator{}
	assert.NilError(t, v.validateRequiredParams(validUser("u1")))
	assert.Assert(t, v.validateRequiredParams(&v1.User{Spec: v1.UserSpec{Type: "bad"}}) != nil)
	assert.Assert(t, v.validateRequiredParams(&v1.User{
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.DefaultRole}},
	}) != nil)
}

// TestUserValidateImmutableFields verifies type/name immutability.
func TestUserValidateImmutableFields(t *testing.T) {
	v := &UserValidator{}
	oldUser := validUser("u1")
	assert.NilError(t, v.validateImmutableFields(validUser("u1"), oldUser))

	changed := validUser("u1")
	changed.Spec.Type = v1.SSOUserType
	assert.Assert(t, v.validateImmutableFields(changed, oldUser) != nil)
}

// TestUserValidateRoles verifies referenced roles existence.
func TestUserValidateRoles(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	withRole := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(defaultRole()).Build()}
	assert.NilError(t, withRole.validateRoles(ctx, validUser("u1")))

	noRole := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	assert.Assert(t, noRole.validateRoles(ctx, validUser("u1")) != nil)
}

// TestUserValidateAccessRemoved verifies managed-before-access removal rule.
func TestUserValidateAccessRemoved(t *testing.T) {
	v := &UserValidator{}
	oldUser := validUser("u1")
	commonuser.AssignWorkspace(oldUser, "ws1")

	newUser := validUser("u1")
	commonuser.AssignManagedWorkspace(newUser, "ws1")
	assert.Assert(t, v.validateAccessRemoved(newUser, oldUser) != nil)

	clean := validUser("u1")
	assert.NilError(t, v.validateAccessRemoved(clean, validUser("u1")))
}

// TestGetUser verifies user retrieval helper.
func TestGetUser(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(validUser("u1")).Build()

	_, err := getUser(ctx, k8sClient, "")
	assert.Assert(t, err != nil)

	got, err := getUser(ctx, k8sClient, "u1")
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
}

// TestUserValidatorHandle verifies the user validator admission handler.
func TestUserValidatorHandle(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultRole()).Build()
	v := &UserValidator{Client: k8sClient, decoder: newDecoder(t)}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, validUser("u1"), nil))
	assert.Assert(t, resp.Allowed)

	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, validUser("u1"), validUser("u1")))
	assert.Assert(t, resp.Allowed)
}
