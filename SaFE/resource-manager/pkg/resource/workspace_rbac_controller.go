/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

const (
	rbacManagedByLabelKey = "app.kubernetes.io/managed-by"
	rbacManagedByLabelVal = "primus-safe-resource-manager"

	rbacWorkspaceNamePrefix  = "primus-safe:workspace:"
	rbacWorkspaceRoleSuffix  = ":member"
	rbacWorkspaceBindSuffix  = ":members"
	rbacPlatformRoleName     = "primus-safe:platform:system-admin"
	rbacPlatformBindingName  = "primus-safe:platform:system-admins"
	rbacWorkspaceUseVerb     = "use"
	rbacWorkspaceResource    = "workspaces"
	rbacPlatformReconcileKey = "primus-safe:platform"
)

// WorkspaceRBACReconciler projects workspace membership and the platform system-admin role
// into ClusterRoles and ClusterRoleBindings on the global cluster.
type WorkspaceRBACReconciler struct {
	client.Client
	subjectPrefix string
	groupMappings []commonconfig.AuthGroupMapping
	// storedRoles is true in builtin mode, where a system-admin role stored on a User counts.
	storedRoles bool
}

// SetupWorkspaceRBACController initializes and registers the WorkspaceRBACReconciler with the controller manager.
// Group mappings are read here and kept for the process. A change takes effect when resource-manager starts again.
func SetupWorkspaceRBACController(mgr manager.Manager) error {
	mappings, err := projectedGroupMappings()
	if err != nil {
		return err
	}
	r := &WorkspaceRBACReconciler{
		Client:        mgr.GetClient(),
		subjectPrefix: commonconfig.GetRBACSubjectPrefix(),
		groupMappings: mappings,
		storedRoles:   commonconfig.GetAuthMode() == commonconfig.AuthModeBuiltin,
	}
	managedPredicate := builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetLabels()[rbacManagedByLabelKey] == rbacManagedByLabelVal
	}))
	// The platform objects are reconciled once at start, even when no User or managed object exists yet.
	startup := make(chan event.GenericEvent, 1)
	startup <- event.GenericEvent{Object: &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: rbacPlatformRoleName}}}
	err = ctrlruntime.NewControllerManagedBy(mgr).
		Named("workspace-rbac").
		For(&v1.Workspace{}, builder.WithPredicates(workspaceRBACPredicate{})).
		Watches(&v1.User{}, r.handleUserEvent()).
		Watches(&rbacv1.ClusterRole{}, handler.EnqueueRequestsFromMapFunc(rbacObjectToRequests), managedPredicate).
		Watches(&rbacv1.ClusterRoleBinding{}, handler.EnqueueRequestsFromMapFunc(rbacObjectToRequests), managedPredicate).
		WatchesRawSource(source.Channel(startup, handler.EnqueueRequestsFromMapFunc(rbacObjectToRequests))).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Workspace RBAC Controller successfully")
	return nil
}

// workspaceRBACPredicate passes Workspace creation, deletion and the start of deletion.
type workspaceRBACPredicate struct {
	predicate.Funcs
}

// Update passes a Workspace update only when its deletion starts.
func (workspaceRBACPredicate) Update(e event.UpdateEvent) bool {
	return e.ObjectOld.GetDeletionTimestamp().IsZero() && !e.ObjectNew.GetDeletionTimestamp().IsZero()
}

// projectedGroupMappings returns auth.group_mappings in external mode. Builtin mode projects none.
func projectedGroupMappings() ([]commonconfig.AuthGroupMapping, error) {
	if err := commonconfig.CheckAuthMode(); err != nil {
		return nil, err
	}
	if commonconfig.GetAuthMode() != commonconfig.AuthModeExternal {
		return nil, nil
	}
	return commonconfig.GetAuthGroupMappings()
}

// handleUserEvent enqueues the workspaces a User lists before and after the change.
// In builtin mode a User whose system-admin role or restriction changes also enqueues the platform binding.
func (r *WorkspaceRBACReconciler) handleUserEvent() handler.EventHandler {
	enqueuePlatform := func(q v1.RequestWorkQueue, u *v1.User) {
		if r.storedRoles && u.IsSystemAdmin() {
			q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: rbacPlatformReconcileKey}})
		}
	}
	enqueue := func(q v1.RequestWorkQueue, users ...*v1.User) {
		seen := map[string]bool{}
		for _, u := range users {
			for _, ws := range commonuser.GetWorkspace(u) {
				if ws == "" || seen[ws] {
					continue
				}
				seen[ws] = true
				q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: ws}})
			}
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			if u, ok := evt.Object.(*v1.User); ok {
				enqueue(q, u)
				enqueuePlatform(q, u)
			}
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldUser, ok1 := evt.ObjectOld.(*v1.User)
			newUser, ok2 := evt.ObjectNew.(*v1.User)
			if !ok1 || !ok2 {
				return
			}
			if isUserRBACChanged(oldUser, newUser) {
				enqueue(q, oldUser, newUser)
			}
			if isPlatformRoleChanged(oldUser, newUser) {
				enqueuePlatform(q, oldUser)
				enqueuePlatform(q, newUser)
			}
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			if u, ok := evt.Object.(*v1.User); ok {
				enqueue(q, u)
				enqueuePlatform(q, u)
			}
		},
	}
}

// isUserRBACChanged reports whether a User change affects any projected subject.
func isUserRBACChanged(oldUser, newUser *v1.User) bool {
	return oldUser.Spec.RestrictedType != newUser.Spec.RestrictedType ||
		!reflect.DeepEqual(commonuser.GetWorkspace(oldUser), commonuser.GetWorkspace(newUser))
}

// isPlatformRoleChanged reports whether a User change affects the platform binding in builtin mode.
func isPlatformRoleChanged(oldUser, newUser *v1.User) bool {
	return oldUser.IsSystemAdmin() != newUser.IsSystemAdmin() ||
		oldUser.Spec.RestrictedType != newUser.Spec.RestrictedType
}

// rbacObjectToRequests maps a managed ClusterRole or ClusterRoleBinding back to its reconcile key.
func rbacObjectToRequests(_ context.Context, obj client.Object) []reconcile.Request {
	name := obj.GetName()
	key := ""
	switch {
	case name == rbacPlatformRoleName || name == rbacPlatformBindingName:
		key = rbacPlatformReconcileKey
	case strings.HasPrefix(name, rbacWorkspaceNamePrefix):
		rest := strings.TrimPrefix(name, rbacWorkspaceNamePrefix)
		if ws, ok := strings.CutSuffix(rest, rbacWorkspaceBindSuffix); ok {
			key = ws
		} else if ws, ok = strings.CutSuffix(rest, rbacWorkspaceRoleSuffix); ok {
			key = ws
		}
	}
	if key == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: apitypes.NamespacedName{Name: key}}}
}

// Reconcile converges the RBAC objects of one workspace, or of the platform role for the platform key.
func (r *WorkspaceRBACReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	if req.Name == rbacPlatformReconcileKey {
		return ctrlruntime.Result{}, r.reconcilePlatform(ctx)
	}
	return ctrlruntime.Result{}, r.reconcileWorkspace(ctx, req.Name)
}

// reconcileWorkspace converges the member ClusterRole and ClusterRoleBinding of a workspace,
// and deletes them when the workspace is gone or being deleted.
func (r *WorkspaceRBACReconciler) reconcileWorkspace(ctx context.Context, name string) error {
	roleName, bindingName := workspaceRoleName(name), workspaceBindingName(name)
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, workspace); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		workspace = nil
	}
	if workspace == nil || !workspace.GetDeletionTimestamp().IsZero() {
		return r.deleteRBACObjects(ctx, roleName, bindingName)
	}
	users, err := r.listUsers(ctx)
	if err != nil {
		return err
	}
	owner := []metav1.OwnerReference{{
		APIVersion: v1.SchemeGroupVersion.String(),
		Kind:       v1.WorkspaceKind,
		Name:       workspace.Name,
		UID:        workspace.UID,
	}}
	rules := []rbacv1.PolicyRule{{
		APIGroups:     []string{v1.SchemeGroupVersion.Group},
		Resources:     []string{rbacWorkspaceResource},
		ResourceNames: []string{name},
		Verbs:         []string{rbacWorkspaceUseVerb},
	}}
	if err = r.applyClusterRole(ctx, roleName, rules, owner); err != nil {
		return err
	}
	return r.applyClusterRoleBinding(ctx, bindingName, roleName, r.workspaceSubjects(name, users), owner)
}

// reconcilePlatform converges the platform system-admin ClusterRole and ClusterRoleBinding.
func (r *WorkspaceRBACReconciler) reconcilePlatform(ctx context.Context) error {
	rules := []rbacv1.PolicyRule{{
		APIGroups: []string{v1.SchemeGroupVersion.Group},
		Resources: []string{rbacWorkspaceResource},
		Verbs:     []string{rbacWorkspaceUseVerb},
	}}
	if err := r.applyClusterRole(ctx, rbacPlatformRoleName, rules, nil); err != nil {
		return err
	}
	var users []v1.User
	if r.storedRoles {
		var err error
		if users, err = r.listUsers(ctx); err != nil {
			return err
		}
	}
	return r.applyClusterRoleBinding(ctx, rbacPlatformBindingName, rbacPlatformRoleName, r.platformSubjects(users), nil)
}

// listUsers returns all Users from the cache.
func (r *WorkspaceRBACReconciler) listUsers(ctx context.Context) ([]v1.User, error) {
	userList := &v1.UserList{}
	if err := r.List(ctx, userList); err != nil {
		return nil, err
	}
	return userList.Items, nil
}

// workspaceSubjects returns the sorted subjects that are members of the workspace.
func (r *WorkspaceRBACReconciler) workspaceSubjects(workspace string, users []v1.User) []rbacv1.Subject {
	var userNames, groupNames []string
	for i := range users {
		if users[i].Spec.RestrictedType == v1.UserFrozen {
			continue
		}
		if commonuser.HasWorkspaceRight(&users[i], workspace) {
			userNames = append(userNames, users[i].Name)
		}
	}
	for _, m := range r.groupMappings {
		if m.Group != "" && containsString(m.Workspaces, workspace) {
			groupNames = append(groupNames, m.Group)
		}
	}
	return r.buildSubjects(userNames, groupNames)
}

// platformSubjects returns the groups mapped to system-admin and, in builtin mode, the users that are
// not frozen and hold system-admin on the User. In external mode a role stored on a User does not count.
func (r *WorkspaceRBACReconciler) platformSubjects(users []v1.User) []rbacv1.Subject {
	var userNames, groupNames []string
	if r.storedRoles {
		for i := range users {
			if users[i].Spec.RestrictedType != v1.UserFrozen && users[i].IsSystemAdmin() {
				userNames = append(userNames, users[i].Name)
			}
		}
	}
	for _, m := range r.groupMappings {
		if m.Group != "" && containsString(m.Roles, string(v1.SystemAdminRole)) {
			groupNames = append(groupNames, m.Group)
		}
	}
	return r.buildSubjects(userNames, groupNames)
}

// buildSubjects turns user and group names into prefixed, de-duplicated subjects, users first, each sorted by name.
func (r *WorkspaceRBACReconciler) buildSubjects(userNames, groupNames []string) []rbacv1.Subject {
	subjects := make([]rbacv1.Subject, 0, len(userNames)+len(groupNames))
	for _, item := range []struct {
		kind  string
		names []string
	}{{rbacv1.UserKind, userNames}, {rbacv1.GroupKind, groupNames}} {
		names := append([]string(nil), item.names...)
		sort.Strings(names)
		for i, n := range names {
			if i > 0 && names[i-1] == n {
				continue
			}
			subjects = append(subjects, rbacv1.Subject{
				Kind:     item.kind,
				APIGroup: rbacv1.GroupName,
				Name:     r.subjectPrefix + n,
			})
		}
	}
	return subjects
}

// applyClusterRole creates the ClusterRole or updates it when its rules, label or owner differ.
func (r *WorkspaceRBACReconciler) applyClusterRole(ctx context.Context,
	name string, rules []rbacv1.PolicyRule, owner []metav1.OwnerReference) error {
	current := &rbacv1.ClusterRole{}
	err := r.Get(ctx, client.ObjectKey{Name: name}, current)
	if apierrors.IsNotFound(err) {
		role := &rbacv1.ClusterRole{ObjectMeta: rbacObjectMeta(name, owner), Rules: rules}
		if err = r.Create(ctx, role); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ClusterRole %s: %v", name, err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current.Rules, rules) && isRBACMetaUpToDate(current, owner) {
		return nil
	}
	updated := current.DeepCopy()
	updated.Rules = rules
	setRBACMeta(updated, owner)
	if err = r.Update(ctx, updated); err != nil {
		return fmt.Errorf("failed to update ClusterRole %s: %v", name, err)
	}
	return nil
}

// applyClusterRoleBinding creates the ClusterRoleBinding or updates it when its subjects, label or owner differ.
// A binding that refers to another role is recreated, since roleRef is immutable.
func (r *WorkspaceRBACReconciler) applyClusterRoleBinding(ctx context.Context,
	name, roleName string, subjects []rbacv1.Subject, owner []metav1.OwnerReference) error {
	roleRef := rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: common.ClusterRoleKind, Name: roleName}
	current := &rbacv1.ClusterRoleBinding{}
	err := r.Get(ctx, client.ObjectKey{Name: name}, current)
	if err == nil && current.RoleRef != roleRef {
		if err = r.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ClusterRoleBinding %s: %v", name, err)
		}
		err = apierrors.NewNotFound(rbacv1.Resource("clusterrolebindings"), name)
	}
	if apierrors.IsNotFound(err) {
		binding := &rbacv1.ClusterRoleBinding{ObjectMeta: rbacObjectMeta(name, owner), RoleRef: roleRef, Subjects: subjects}
		if err = r.Create(ctx, binding); err != nil {
			return fmt.Errorf("failed to create ClusterRoleBinding %s: %v", name, err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if reflect.DeepEqual(normalizeSubjects(current.Subjects), subjects) && isRBACMetaUpToDate(current, owner) {
		return nil
	}
	updated := current.DeepCopy()
	updated.Subjects = subjects
	setRBACMeta(updated, owner)
	if err = r.Update(ctx, updated); err != nil {
		return fmt.Errorf("failed to update ClusterRoleBinding %s: %v", name, err)
	}
	return nil
}

// deleteRBACObjects deletes the named ClusterRole and ClusterRoleBinding, ignoring ones already gone.
func (r *WorkspaceRBACReconciler) deleteRBACObjects(ctx context.Context, roleName, bindingName string) error {
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: bindingName}}
	if err := r.Delete(ctx, binding); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ClusterRoleBinding %s: %v", bindingName, err)
	}
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: roleName}}
	if err := r.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ClusterRole %s: %v", roleName, err)
	}
	return nil
}

// rbacObjectMeta returns the metadata of a managed RBAC object.
func rbacObjectMeta(name string, owner []metav1.OwnerReference) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            name,
		Labels:          map[string]string{rbacManagedByLabelKey: rbacManagedByLabelVal},
		OwnerReferences: owner,
	}
}

// isRBACMetaUpToDate reports whether the object carries the managed-by label and exactly the expected owner.
func isRBACMetaUpToDate(obj client.Object, owner []metav1.OwnerReference) bool {
	if obj.GetLabels()[rbacManagedByLabelKey] != rbacManagedByLabelVal {
		return false
	}
	if len(obj.GetOwnerReferences()) == 0 && len(owner) == 0 {
		return true
	}
	return reflect.DeepEqual(obj.GetOwnerReferences(), owner)
}

// setRBACMeta sets the managed-by label and the owner references on the object.
func setRBACMeta(obj client.Object, owner []metav1.OwnerReference) {
	v1.SetLabel(obj, rbacManagedByLabelKey, rbacManagedByLabelVal)
	obj.SetOwnerReferences(owner)
}

// normalizeSubjects returns a non-nil empty slice for an empty subject list so that it compares equal to a built one.
func normalizeSubjects(subjects []rbacv1.Subject) []rbacv1.Subject {
	if len(subjects) == 0 {
		return []rbacv1.Subject{}
	}
	return subjects
}

// workspaceRoleName returns the name of the member ClusterRole of a workspace.
func workspaceRoleName(workspace string) string {
	return rbacWorkspaceNamePrefix + workspace + rbacWorkspaceRoleSuffix
}

// workspaceBindingName returns the name of the member ClusterRoleBinding of a workspace.
func workspaceBindingName(workspace string) string {
	return rbacWorkspaceNamePrefix + workspace + rbacWorkspaceBindSuffix
}
