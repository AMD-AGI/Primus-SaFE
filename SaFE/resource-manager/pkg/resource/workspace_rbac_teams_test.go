/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"testing"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// subjectNames returns the names of the subjects of the kind.
func subjectNames(subjects []rbacv1.Subject, kind string) []string {
	var names []string
	for _, s := range subjects {
		if s.Kind == kind {
			names = append(names, s.Name)
		}
	}
	return names
}

// Members get a team:<id> group per bound team and managers one per manager binding, in external mode
// only. A mapped platform group that carries the team prefix is not projected.
func TestWorkspaceTeamSubjects(t *testing.T) {
	ws := workspaceObject("lab")
	ws.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}, {Name: "t2", Role: v1.WorkspaceTeamManager}}
	mappings := []commonconfig.AuthGroupMapping{
		{Group: "lab-users", Workspaces: []string{"lab"}},
		{Group: "team:t9", Workspaces: []string{"lab"}, Roles: []string{string(v1.SystemAdminRole)}},
		{Group: "admins", Roles: []string{string(v1.SystemAdminRole)}},
	}
	external := &WorkspaceRBACReconciler{subjectPrefix: "primus:", groupMappings: mappings}
	require.Equal(t, []string{"primus:lab-users", "primus:team:t1", "primus:team:t2"},
		subjectNames(external.workspaceSubjects(ws, nil), rbacv1.GroupKind))
	require.Equal(t, []string{"primus:team:t2"}, subjectNames(external.managerSubjects(ws, nil), rbacv1.GroupKind))
	require.Equal(t, []string{"primus:admins"}, subjectNames(external.platformSubjects(nil), rbacv1.GroupKind))

	builtin := &WorkspaceRBACReconciler{subjectPrefix: "primus:", storedRoles: true}
	require.Empty(t, builtin.workspaceSubjects(ws, nil))
	require.Empty(t, builtin.managerSubjects(ws, nil))
}

// The predicate passes an update that starts deletion or changes the team bindings, and nothing else.
func TestWorkspaceRBACPredicateUpdate(t *testing.T) {
	old := workspaceObject("lab")
	old.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}}
	update := func(change func(ws *v1.Workspace)) bool {
		next := old.DeepCopy()
		change(next)
		return workspaceRBACPredicate{}.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: next})
	}
	require.False(t, update(func(ws *v1.Workspace) { ws.Spec.Managers = []string{"alice"} }))
	require.True(t, update(func(ws *v1.Workspace) { ws.Spec.Teams = append(ws.Spec.Teams, v1.WorkspaceTeam{Name: "t2"}) }))
	require.True(t, update(func(ws *v1.Workspace) { ws.Spec.Teams[0].Role = v1.WorkspaceTeamManager }))
	require.True(t, update(func(ws *v1.Workspace) { ws.Spec.Teams = nil }))
	require.True(t, update(func(ws *v1.Workspace) {
		now := metav1.Now()
		ws.DeletionTimestamp = &now
	}))
	require.False(t, workspaceRBACPredicate{}.Update(event.UpdateEvent{ObjectOld: &v1.User{}, ObjectNew: &v1.User{}}))
}
