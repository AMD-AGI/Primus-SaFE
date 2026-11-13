/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

func rbacScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	return scheme
}

func memberUser(name, workspace string, roles ...v1.UserRole) *v1.User {
	user := &v1.User{}
	user.Name = name
	user.Spec.Roles = roles
	if workspace != "" {
		commonuser.AssignWorkspace(user, workspace)
	}
	return user
}

func testReconciler(t *testing.T, objs ...client.Object) *WorkspaceRBACReconciler {
	t.Helper()
	return &WorkspaceRBACReconciler{
		Client:        fake.NewClientBuilder().WithScheme(rbacScheme(t)).WithObjects(objs...).Build(),
		subjectPrefix: "primus:",
		groupMappings: []commonconfig.AuthGroupMapping{{Group: "lab-users", Workspaces: []string{"lab"}, Roles: []string{"default"}}},
		storedRoles:   true,
	}
}

func workspaceObject(name string) *v1.Workspace {
	ws := &v1.Workspace{}
	ws.Name = name
	ws.UID = types.UID("uid-" + name)
	return ws
}

// A workspace reconcile creates the member and manager roles and bindings, a second pass leaves them,
// and a gone workspace deletes them.
func TestReconcileWorkspaceCreatesUpdatesAndDeletes(t *testing.T) {
	ctx := context.Background()
	alice := memberUser("alice", "lab", v1.DefaultRole)
	commonuser.AddManagedWorkspace(alice, "lab")
	frozen := memberUser("frank", "lab", v1.DefaultRole)
	commonuser.AddManagedWorkspace(frozen, "lab")
	frozen.Spec.RestrictedType = v1.UserFrozen
	ws := workspaceObject("lab")
	r := testReconciler(t, ws, alice, frozen, memberUser("bob", "prod", v1.DefaultRole))

	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "lab"}})
	require.NoError(t, err)

	role := &rbacv1.ClusterRole{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: workspaceRoleName("lab")}, role))
	require.Equal(t, rbacManagedByLabelVal, role.Labels[rbacManagedByLabelKey])
	require.Equal(t, []string{"use"}, role.Rules[0].Verbs)
	require.Equal(t, ws.UID, role.OwnerReferences[0].UID)

	binding := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: workspaceBindingName("lab")}, binding))
	require.Equal(t, []rbacv1.Subject{
		{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "primus:alice"},
		{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: "primus:lab-users"},
	}, binding.Subjects)

	managerRole := &rbacv1.ClusterRole{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: workspaceManagerRoleName("lab")}, managerRole))
	require.Equal(t, []string{"use", "manage"}, managerRole.Rules[0].Verbs)
	require.Equal(t, []string{"lab"}, managerRole.Rules[0].ResourceNames)
	managerBinding := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: workspaceManagerBindingName("lab")}, managerBinding))
	require.Equal(t, workspaceManagerRoleName("lab"), managerBinding.RoleRef.Name)
	require.Equal(t, []rbacv1.Subject{
		{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "primus:alice"},
	}, managerBinding.Subjects)

	role.Rules = nil
	require.NoError(t, r.Update(ctx, role))
	binding.Subjects = nil
	binding.RoleRef.Name = "other"
	require.NoError(t, r.Update(ctx, binding))
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "lab"}})
	require.NoError(t, err)
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: workspaceRoleName("lab")}, role))
	require.NotEmpty(t, role.Rules)
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: workspaceBindingName("lab")}, binding))
	require.Equal(t, workspaceRoleName("lab"), binding.RoleRef.Name)

	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "lab"}})
	require.NoError(t, err)

	require.NoError(t, r.Delete(ctx, ws))
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "lab"}})
	require.NoError(t, err)
	require.Error(t, r.Get(ctx, client.ObjectKey{Name: workspaceRoleName("lab")}, &rbacv1.ClusterRole{}))
	require.Error(t, r.Get(ctx, client.ObjectKey{Name: workspaceBindingName("lab")}, &rbacv1.ClusterRoleBinding{}))
	require.Error(t, r.Get(ctx, client.ObjectKey{Name: workspaceManagerRoleName("lab")}, &rbacv1.ClusterRole{}))
	require.Error(t, r.Get(ctx, client.ObjectKey{Name: workspaceManagerBindingName("lab")}, &rbacv1.ClusterRoleBinding{}))
}

// A workspace that has started deletion loses its projected objects.
func TestReconcileDeletesObjectsOfADeletingWorkspace(t *testing.T) {
	ctx := context.Background()
	ws := workspaceObject("lab")
	now := metav1.Now()
	ws.DeletionTimestamp = &now
	ws.Finalizers = []string{"keep"}
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: workspaceRoleName("lab")}}
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: workspaceBindingName("lab")}}
	managerRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: workspaceManagerRoleName("lab")}}
	r := testReconciler(t, ws, role, binding, managerRole)

	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "lab"}})
	require.NoError(t, err)
	require.Error(t, r.Get(ctx, client.ObjectKey{Name: role.Name}, &rbacv1.ClusterRole{}))
	require.Error(t, r.Get(ctx, client.ObjectKey{Name: managerRole.Name}, &rbacv1.ClusterRole{}))
}

// The platform reconcile binds mapped groups and, in builtin mode, stored system-admin users.
func TestReconcilePlatform(t *testing.T) {
	ctx := context.Background()
	admin := memberUser("alice", "", v1.SystemAdminRole)
	frozen := memberUser("frank", "", v1.SystemAdminRole)
	frozen.Spec.RestrictedType = v1.UserFrozen
	r := testReconciler(t, admin, frozen)
	r.groupMappings = []commonconfig.AuthGroupMapping{
		{Group: "admins", Roles: []string{string(v1.SystemAdminRole)}},
		{Group: "", Roles: []string{string(v1.SystemAdminRole)}},
		{Group: "admins", Roles: []string{string(v1.SystemAdminRole)}},
	}

	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: rbacPlatformReconcileKey}})
	require.NoError(t, err)
	role := &rbacv1.ClusterRole{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: rbacPlatformRoleName}, role))
	require.Equal(t, []string{"use", "manage"}, role.Rules[0].Verbs)
	binding := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: rbacPlatformBindingName}, binding))
	require.Equal(t, []string{"primus:alice", "primus:admins"}, []string{binding.Subjects[0].Name, binding.Subjects[1].Name})

	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: rbacPlatformReconcileKey}})
	require.NoError(t, err)

	r.storedRoles = false
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: rbacPlatformReconcileKey}})
	require.NoError(t, err)
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: rbacPlatformBindingName}, binding))
	require.Len(t, binding.Subjects, 1)
	require.Equal(t, rbacv1.GroupKind, binding.Subjects[0].Kind)
}

// A read error from the API server is returned, and a delete error is returned with the object name.
func TestReconcileReportsClientErrors(t *testing.T) {
	ctx := context.Background()
	scheme := rbacScheme(t)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &WorkspaceRBACReconciler{Client: interceptor.NewClient(base, interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return fmt.Errorf("api down")
		},
	}), subjectPrefix: "primus:"}
	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "lab"}})
	require.Error(t, err)

	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: workspaceRoleName("lab")}}
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: workspaceBindingName("lab")}}
	stored := fake.NewClientBuilder().WithScheme(scheme).WithObjects(role, binding).Build()
	r.Client = interceptor.NewClient(stored, interceptor.Funcs{
		Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
			return fmt.Errorf("delete refused")
		},
	})
	err = r.deleteRBACObjects(ctx, role.Name, binding.Name)
	require.ErrorContains(t, err, binding.Name)
}

// User and managed-object events enqueue the workspace, and a builtin system-admin also enqueues the platform binding.
func TestUserAndRBACEventsEnqueue(t *testing.T) {
	r := &WorkspaceRBACReconciler{storedRoles: true}
	h := r.handleUserEvent()
	ctx := context.Background()
	alice := memberUser("alice", "lab", v1.SystemAdminRole)
	bob := memberUser("bob", "lab")
	next := bob.DeepCopy()
	commonuser.AssignWorkspace(next, "prod")
	next.Spec.Roles = []v1.UserRole{v1.SystemAdminRole}

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	h.Create(ctx, event.CreateEvent{Object: alice}, q)
	h.Update(ctx, event.UpdateEvent{ObjectOld: bob, ObjectNew: next}, q)
	h.Delete(ctx, event.DeleteEvent{Object: alice}, q)
	h.Create(ctx, event.CreateEvent{Object: &v1.Workspace{}}, q)
	h.Update(ctx, event.UpdateEvent{ObjectOld: &v1.Workspace{}, ObjectNew: next}, q)

	got := map[string]int{}
	for q.Len() > 0 {
		item, _ := q.Get()
		got[item.Name]++
		q.Done(item)
	}
	require.NotZero(t, got["lab"])
	require.NotZero(t, got["prod"])
	require.NotZero(t, got[rbacPlatformReconcileKey])

	carol := memberUser("carol", "")
	promoted := carol.DeepCopy()
	commonuser.AddManagedWorkspace(promoted, "ops")
	require.True(t, isUserRBACChanged(carol, promoted))
	h.Update(ctx, event.UpdateEvent{ObjectOld: carol, ObjectNew: promoted}, q)
	item, _ := q.Get()
	require.Equal(t, "ops", item.Name)
	q.Done(item)

	require.Nil(t, rbacObjectToRequests(ctx, &rbacv1.ClusterRole{}))
	require.Equal(t, "lab", rbacObjectToRequests(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: workspaceRoleName("lab")}})[0].Name)
	require.Equal(t, "lab", rbacObjectToRequests(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: workspaceBindingName("lab")}})[0].Name)
	require.Equal(t, "lab", rbacObjectToRequests(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: workspaceManagerRoleName("lab")}})[0].Name)
	require.Equal(t, "lab", rbacObjectToRequests(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: workspaceManagerBindingName("lab")}})[0].Name)
	require.Equal(t, rbacPlatformReconcileKey, rbacObjectToRequests(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: rbacPlatformRoleName}})[0].Name)
	require.Equal(t, rbacPlatformReconcileKey, rbacObjectToRequests(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: rbacPlatformBindingName}})[0].Name)

	pred := workspaceRBACPredicate{}
	oldWS, newWS := workspaceObject("lab"), workspaceObject("lab")
	require.False(t, pred.Update(event.UpdateEvent{ObjectOld: oldWS, ObjectNew: newWS}))
	now := metav1.Now()
	newWS.DeletionTimestamp = &now
	require.True(t, pred.Update(event.UpdateEvent{ObjectOld: oldWS, ObjectNew: newWS}))
}
