/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// nestedUserAdmissionClient returns a client whose user updates pass through the user mutator before they
// are stored, the way the apiserver calls the user webhook for an update made inside workspace admission.
// The mutator reads the base client, so a workspace created through the returned client later is unknown to
// it during that admission.
func nestedUserAdmissionClient(t *testing.T, objs ...client.Object) client.WithWatch {
	t.Helper()
	base := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(objs...).Build()
	userMutator := &UserMutator{Client: base}
	return interceptor.NewClient(base, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if u, ok := obj.(*v1.User); ok {
				userMutator.mutateOnUpdate(ctx, u)
			}
			return c.Update(ctx, obj, opts...)
		},
	})
}

// grantFixtures returns an existing default workspace, a user that holds it and a user that holds nothing.
func grantFixtures() (*v1.Workspace, *v1.User, *v1.User) {
	existing := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-existing"}, Spec: v1.WorkspaceSpec{IsDefault: true}}
	member, empty := validUser("member"), validUser("empty")
	commonuser.AssignWorkspace(member, "ws-existing")
	return existing, member, empty
}

// A default workspace created with isDefault set reaches every user, with or without a default workspace
// already: a user is either granted it by the admission or the workspace records that the grant is due.
func TestCreatingDefaultWorkspaceKeepsGrantThroughNestedUserAdmission(t *testing.T) {
	ctx := context.Background()
	existing, member, empty := grantFixtures()
	cli := nestedUserAdmissionClient(t, existing, member, empty)
	created := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-new"}, Spec: v1.WorkspaceSpec{IsDefault: true}}

	assert.NilError(t, (&WorkspaceMutator{Client: cli}).mutateDefaultWorkspaceUsers(ctx, nil, created))
	assert.NilError(t, cli.Create(ctx, created))

	for _, name := range []string{"member", "empty"} {
		got := &v1.User{}
		assert.NilError(t, cli.Get(ctx, client.ObjectKey{Name: name}, got))
		granted := commonuser.HasWorkspaceRight(got, "ws-new") || v1.HasAnnotation(created, v1.WorkspacePrefix+"grant.pending")
		assert.Assert(t, granted, "user %s lost the new default workspace: %v", name, commonuser.GetWorkspace(got))
	}
}

// A manager given on creation holds the workspace and manages it: the admission grants it or records that
// the grant is due.
func TestCreatingWorkspaceWithManagerKeepsGrantThroughNestedUserAdmission(t *testing.T) {
	ctx := context.Background()
	existing, member, _ := grantFixtures()
	cli := nestedUserAdmissionClient(t, existing, member)
	created := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-new"}, Spec: v1.WorkspaceSpec{Managers: []string{"member"}}}

	assert.NilError(t, (&WorkspaceMutator{Client: cli}).mutateManagers(ctx, nil, created))
	assert.NilError(t, cli.Create(ctx, created))

	got := &v1.User{}
	assert.NilError(t, cli.Get(ctx, client.ObjectKey{Name: "member"}, got))
	granted := commonuser.HasWorkspaceManagedRight(got, "ws-new") || v1.HasAnnotation(created, v1.WorkspacePrefix+"grant.pending")
	assert.Assert(t, granted, "the manager lost the new workspace: %v %v",
		commonuser.GetWorkspace(got), commonuser.GetManagedWorkspace(got))
}
