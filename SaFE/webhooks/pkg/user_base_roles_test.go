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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
)

// legacyMutateRoles is mutateRoles exactly as it was before users.base_roles existed, kept here as the
// reference: with the defaults the webhook must produce what this produces, byte for byte. It is a frozen
// copy, not a call into the code under test, so it does not move when that code does.
func legacyMutateRoles(user *v1.User) {
	switch {
	case user.IsSystemAdmin() && len(user.Spec.Roles) > 1:
		user.Spec.Roles = []v1.UserRole{v1.SystemAdminRole}
	default:
		roleSet := sets.NewSet()
		newRoles := make([]v1.UserRole, 0, len(user.Spec.Roles))
		for i, r := range user.Spec.Roles {
			if roleSet.Has(string(r)) || r == v1.WorkspaceAdminRole {
				continue
			}
			newRoles = append(newRoles, user.Spec.Roles[i])
			roleSet.Insert(string(r))
		}
		if len(newRoles) != len(user.Spec.Roles) {
			user.Spec.Roles = newRoles
		}
	}
	if !user.IsSystemAdmin() && !v1.IsContainRole(user.Spec.Roles, v1.DefaultRole) {
		user.Spec.Roles = append(user.Spec.Roles, v1.DefaultRole)
	}
}

// legacyMutateWorkspace is mutateWorkspace as it was before users.join_default_workspaces existed.
func legacyMutateWorkspace(ctx context.Context, m *UserMutator, user *v1.User) {
	workspaceSet := sets.NewSet()
	currentWorkspaces := commonuser.GetWorkspace(user)
	var workspaces []string
	for _, w := range currentWorkspaces {
		if workspaceSet.Has(w) {
			continue
		}
		workspaceSet.Insert(w)
		if _, err := getWorkspace(ctx, m.Client, w); err == nil {
			workspaces = append(workspaces, w)
		}
	}
	workspaceList := &v1.WorkspaceList{}
	err := m.List(ctx, workspaceList)
	if err != nil {
		return
	}
	for _, w := range workspaceList.Items {
		if !w.Spec.IsDefault {
			continue
		}
		if sliceutil.Contains(workspaces, w.Name) {
			continue
		}
		workspaces = append(workspaces, w.Name)
	}
	if len(currentWorkspaces) != len(workspaces) {
		commonuser.AssignWorkspace(user, workspaces...)
	}
}

// setUsersConfig sets the two keys for one test; "unset" leaves them out.
func setUsersConfig(t *testing.T, baseRoles, joinDefault string) {
	t.Helper()
	if baseRoles != "unset" {
		commonconfig.SetValue("users.base_roles", baseRoles)
	}
	if joinDefault != "unset" {
		commonconfig.SetValue("users.join_default_workspaces", joinDefault)
	}
	t.Cleanup(func() {
		commonconfig.UnsetValue("users.base_roles")
		commonconfig.UnsetValue("users.join_default_workspaces")
	})
}

var roleInputs = [][]v1.UserRole{
	nil,
	{},
	{v1.DefaultRole},
	{v1.SystemAdminRole},
	{v1.SystemAdminRole, v1.DefaultRole},
	{v1.SystemAdminReadonlyRole},
	{v1.SystemAdminReadonlyRole, v1.DefaultRole},
	{v1.WorkspaceAdminRole},
	{v1.WorkspaceAdminRole, v1.SystemAdminRole},
	{"viewer"},
	{"viewer", v1.DefaultRole, "viewer"},
	{v1.DefaultRole, v1.DefaultRole},
	{v1.SelfReadRole},
}

// With the defaults, in every way an operator can leave them (unset, written out, empty), the webhook gives
// every user the roles it always gave.
func TestUserMutateRolesUnderTheDefaultsIsWhatItAlwaysWas(t *testing.T) {
	for _, cfg := range []struct{ name, base string }{
		{"unset", "unset"},
		{"written out", "default"},
		{"empty", ""},
		{"only names it ignores", "system-admin,workspace-admin"},
	} {
		t.Run(cfg.name, func(t *testing.T) {
			setUsersConfig(t, cfg.base, "unset")
			m := &UserMutator{}
			for _, in := range roleInputs {
				want := &v1.User{Spec: v1.UserSpec{Roles: append([]v1.UserRole(nil), in...)}}
				got := &v1.User{Spec: v1.UserSpec{Roles: append([]v1.UserRole(nil), in...)}}
				legacyMutateRoles(want)
				m.mutateRoles(got)
				assert.DeepEqual(t, got.Spec.Roles, want.Spec.Roles)
			}
		})
	}
}

// With users.base_roles set, every user who is not a system administrator holds those roles, added when
// missing and never in place of what the user holds, and a system administrator still holds that role alone.
func TestUserMutateRolesWithBaseRoles(t *testing.T) {
	for _, tt := range []struct {
		name string
		base string
		in   []v1.UserRole
		want []v1.UserRole
	}{
		{"a new user holds only the minimal role", "self-read", nil, []v1.UserRole{v1.SelfReadRole}},
		{"an empty list is the same as none", "self-read", []v1.UserRole{}, []v1.UserRole{v1.SelfReadRole}},
		{"a user of today keeps the default role and gains the minimal one", "self-read", []v1.UserRole{v1.DefaultRole}, []v1.UserRole{v1.DefaultRole, v1.SelfReadRole}},
		{"the default role is not added back", "self-read", []v1.UserRole{"viewer"}, []v1.UserRole{"viewer", v1.SelfReadRole}},
		{"already held", "self-read", []v1.UserRole{v1.SelfReadRole}, []v1.UserRole{v1.SelfReadRole}},
		{"a system administrator holds that role alone", "self-read", []v1.UserRole{v1.SystemAdminRole}, []v1.UserRole{v1.SystemAdminRole}},
		{"and not the base role next to it", "self-read", []v1.UserRole{v1.SystemAdminRole, v1.SelfReadRole}, []v1.UserRole{v1.SystemAdminRole}},
		{"workspace-admin is stripped as ever", "self-read", []v1.UserRole{v1.WorkspaceAdminRole}, []v1.UserRole{v1.SelfReadRole}},
		{"a read-only administrator is an ordinary user as well", "self-read", []v1.UserRole{v1.SystemAdminReadonlyRole}, []v1.UserRole{v1.SystemAdminReadonlyRole, v1.SelfReadRole}},
		{"repeats are dropped", "self-read", []v1.UserRole{"viewer", "viewer"}, []v1.UserRole{"viewer", v1.SelfReadRole}},
		{"two base roles, in the order given", "self-read,viewer", nil, []v1.UserRole{v1.SelfReadRole, "viewer"}},
		{"two base roles, one held", "self-read,viewer", []v1.UserRole{"viewer"}, []v1.UserRole{"viewer", v1.SelfReadRole}},
		{"the platform roles cannot be made everyone's", "system-admin,self-read", nil, []v1.UserRole{v1.SelfReadRole}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setUsersConfig(t, tt.base, "unset")
			user := &v1.User{Spec: v1.UserSpec{Roles: append([]v1.UserRole(nil), tt.in...)}}
			(&UserMutator{}).mutateRoles(user)
			assert.DeepEqual(t, user.Spec.Roles, tt.want)
		})
	}
}

func workspaceFixtures() (*v1.Workspace, *v1.Workspace) {
	return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-default"}, Spec: v1.WorkspaceSpec{IsDefault: true}},
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-other"}}
}

// With the default (users.join_default_workspaces unset or true) a user is added to the default workspaces
// exactly as before, on every input.
func TestUserMutateWorkspaceUnderTheDefaultsIsWhatItAlwaysWas(t *testing.T) {
	scheme := newScheme(t)
	def, other := workspaceFixtures()
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(def, other).Build()
	m := &UserMutator{Client: cli}
	for _, join := range []string{"unset", "true"} {
		t.Run("join "+join, func(t *testing.T) {
			setUsersConfig(t, "unset", join)
			for _, in := range [][]string{nil, {"ws-other"}, {"ws-default"}, {"ws-other", "ws-other"}, {"missing", "ws-other"}, {"missing"}, {"ws-other", "ws-default", "missing"}} {
				want, got := validUser("u"), validUser("u")
				commonuser.AssignWorkspace(want, in...)
				commonuser.AssignWorkspace(got, in...)
				legacyMutateWorkspace(context.Background(), m, want)
				m.mutateWorkspace(context.Background(), got)
				assert.DeepEqual(t, got.Spec.Resources, want.Spec.Resources)
			}
		})
	}
}

// With users.join_default_workspaces false a user is not added to the default workspaces; what the user has is
// still cleaned (repeats, workspaces that no longer exist) as ever.
func TestUserMutateWorkspaceWithoutJoiningTheDefaults(t *testing.T) {
	scheme := newScheme(t)
	def, other := workspaceFixtures()
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(def, other).Build()
	m := &UserMutator{Client: cli}
	for _, tt := range []struct {
		name string
		in   []string
		want []string
	}{
		{"a new user has none", nil, nil},
		{"its own are kept", []string{"ws-other"}, []string{"ws-other"}},
		{"a default workspace it was given is kept", []string{"ws-default"}, []string{"ws-default"}},
		{"repeats and workspaces that are gone are cleaned", []string{"ws-other", "ws-other", "missing"}, []string{"ws-other"}},
		{"nothing left but gone ones", []string{"missing"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setUsersConfig(t, "unset", "false")
			user := validUser("u")
			commonuser.AssignWorkspace(user, tt.in...)
			m.mutateWorkspace(context.Background(), user)
			assert.DeepEqual(t, commonuser.GetWorkspace(user), tt.want)
		})
	}
}

// Marking a workspace as default adds the users to it, unless the deployment keeps users out of the default
// workspaces until an administrator adds them: the same setting the user webhook honours. With the default
// (unset or true) the workspace webhook does what it always did, on every transition; with false it adds no
// one, and what an administrator does by hand (a manager, a user's own list) is not its business.
func TestMarkingAWorkspaceAsDefaultFollowsJoinDefaultWorkspaces(t *testing.T) {
	notDefault := func() *v1.Workspace { return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}} }
	isDefault := func() *v1.Workspace {
		return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{IsDefault: true}}
	}
	for _, tt := range []struct {
		name     string
		old, new *v1.Workspace
		// joinsOnDefault is what the users hold afterwards when the setting is unset or true.
		joinsOnDefault bool
	}{
		{"a workspace created as default", nil, isDefault(), true},
		{"a workspace marked as default", notDefault(), isDefault(), true},
		{"a workspace that already was default", isDefault(), isDefault(), false},
		{"a workspace that is not default", nil, notDefault(), false},
		{"a workspace that stops being default", isDefault(), notDefault(), false},
	} {
		for _, join := range []string{"unset", "true", "false"} {
			t.Run(tt.name+", join "+join, func(t *testing.T) {
				setUsersConfig(t, "unset", join)
				scheme := newScheme(t)
				u1, u2 := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}, &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u2"}}
				cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(u1, u2).Build()
				m := &WorkspaceMutator{Client: cli}
				assert.NilError(t, m.mutateDefaultWorkspaceUsers(context.Background(), tt.old, tt.new))
				for _, name := range []string{"u1", "u2"} {
					got := &v1.User{}
					assert.NilError(t, cli.Get(context.Background(), client.ObjectKey{Name: name}, got))
					assert.Equal(t, commonuser.HasWorkspaceRight(got, "ws1"), tt.joinsOnDefault && join != "false", name)
				}
			})
		}
	}
}

// A new user of a deployment that grants nothing until an administrator does: created by whoever vouches for
// them (single sign-on, the identity provider's provisioner) with no roles and no workspaces, the admission
// holds the minimal role and nothing else; the validator accepts the result, and the same record updated later
// does not gain the default role.
func TestANewUserOfAMinimalDeploymentHoldsOnlyTheMinimalRole(t *testing.T) {
	setUsersConfig(t, "self-read", "false")
	scheme := newScheme(t)
	def, other := workspaceFixtures()
	selfRead := &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: string(v1.SelfReadRole)}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(def, other, selfRead).Build()
	m := &UserMutator{Client: cli, decoder: newDecoder(t)}
	v := &UserValidator{Client: cli, decoder: newDecoder(t)}

	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "new-user-id"},
		Spec:       v1.UserSpec{Type: v1.SSOUserType},
	}
	m.mutateOnCreation(context.Background(), user)
	assert.DeepEqual(t, user.Spec.Roles, []v1.UserRole{v1.SelfReadRole})
	assert.Equal(t, len(commonuser.GetWorkspace(user)), 0)
	assert.Equal(t, len(commonuser.GetManagedWorkspace(user)), 0)
	assert.NilError(t, v.validateCommon(context.Background(), user))

	m.mutateOnUpdate(context.Background(), user)
	assert.DeepEqual(t, user.Spec.Roles, []v1.UserRole{v1.SelfReadRole})
	assert.Equal(t, len(commonuser.GetWorkspace(user)), 0)

	// Through the admission handler too, as the apiserver's client reaches it.
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "another-id"},
		Spec:       v1.UserSpec{Type: v1.SSOUserType},
	}, nil))
	assert.Assert(t, resp.Allowed)
	found := false
	for _, p := range resp.Patches {
		if p.Path == "/spec/roles" || p.Path == "/spec/roles/0" {
			found = true
		}
	}
	assert.Assert(t, found, "the admission patch must set the roles: %v", resp.Patches)

	// The role has to exist: a deployment that names one it has not installed refuses its new users, it does not
	// hand them a role that grants nothing by accident.
	bare := fake.NewClientBuilder().WithScheme(scheme).WithObjects(def, other).Build()
	vb := &UserValidator{Client: bare, decoder: newDecoder(t)}
	assert.Assert(t, vb.validateCommon(context.Background(), user) != nil)
}
