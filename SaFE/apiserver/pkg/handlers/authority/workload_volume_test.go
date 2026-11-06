/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const pvWorkspace = "pv-ws"

// newPrivilegedVolumeController is an access controller over the built-in roles as the chart ships
// them, a member of pvWorkspace, its manager and a system administrator.
func newPrivilegedVolumeController(t *testing.T) *AccessController {
	t.Helper()
	user := func(name string, roles []v1.UserRole, workspaces, managed []string) *v1.User {
		return &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{
				common.UserWorkspaces: workspaces, common.UserManagedWorkspaces: managed}},
		}
	}
	ws := []string{pvWorkspace}
	objs := append(authtest.ChartRoles(t),
		user("pv-member", []v1.UserRole{v1.DefaultRole}, ws, nil),
		user("pv-manager", []v1.UserRole{v1.DefaultRole}, ws, ws),
		user("pv-admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil),
	)
	sch := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(sch))
	return &AccessController{Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()}
}

// privilegedVolumeWorkspace has a PFS volume for training that requires the grant and a hostpath
// volume over /shared that does not.
func privilegedVolumeWorkspace(requires bool) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: pvWorkspace},
		Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.PFS, MountPath: "/ceph", Scopes: []v1.WorkspaceScope{v1.TrainScope},
				RequiresPrivilegedGrant: requires},
			{Id: 2, Type: v1.HOSTPATH, HostPath: "/shared", MountPath: "/shared"},
			{Id: 3, Type: v1.HOSTPATH, HostPath: "/shared/admin", MountPath: "/admin", RequiresPrivilegedGrant: requires},
		}},
	}
}

// pvWorkload is a workload of the given kind in pvWorkspace that runs as owner ("" for the
// requester) and already carries a mark it must not keep unless the check sets it.
func pvWorkload(kind, owner string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w", Labels: map[string]string{},
		Annotations: map[string]string{v1.WorkloadPrivilegedVolumesAuthzAnnotation: v1.TrueStr}}}
	w.Spec.Workspace = pvWorkspace
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: kind}
	if owner != "" {
		w.Labels[v1.UserIdLabel] = owner
	}
	return w
}

// The mark is set only when the workspace has a volume requiring the grant for the workload's
// scope, the requester holds workload/privileged in the workspace and, when the workload runs as
// another user, so does that user; otherwise any mark the workload carried is removed and the
// workload is not refused.
func TestAuthorizeWorkloadPrivilegedVolumes(t *testing.T) {
	a := newPrivilegedVolumeController(t)
	for _, tt := range []struct {
		name, requester, owner, kind string
		requires, want               bool
	}{
		{"member", "pv-member", "", common.PytorchJobKind, true, false},
		{"manager", "pv-manager", "", common.PytorchJobKind, true, true},
		{"admin", "pv-admin", "", common.PytorchJobKind, true, true},
		{"admin for itself as owner", "pv-admin", "pv-admin", common.PytorchJobKind, true, true},
		{"admin for a member", "pv-admin", "pv-member", common.PytorchJobKind, true, false},
		{"admin for the manager", "pv-admin", "pv-manager", common.PytorchJobKind, true, true},
		{"admin for an unknown user", "pv-admin", "nobody", common.PytorchJobKind, true, false},
		{"manager, no such volume", "pv-manager", "", common.PytorchJobKind, false, false},
		{"manager, inference has only the hostpath one", "pv-manager", "", common.DeploymentKind, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := pvWorkload(tt.kind, tt.owner)
			err := a.AuthorizeWorkloadPrivilegedVolumes(AccessInput{Context: context.Background(), UserId: tt.requester},
				w, privilegedVolumeWorkspace(tt.requires))
			require.NoError(t, err)
			require.Equal(t, tt.want, v1.HasAnnotation(w, v1.WorkloadPrivilegedVolumesAuthzAnnotation))
		})
	}
	// Without a workspace (storage turned off) there is nothing to decide.
	w := pvWorkload(common.PytorchJobKind, "")
	require.NoError(t, a.AuthorizeWorkloadPrivilegedVolumes(AccessInput{Context: context.Background(),
		UserId: "pv-manager"}, w, nil))
	require.False(t, v1.HasAnnotation(w, v1.WorkloadPrivilegedVolumesAuthzAnnotation))
}

// A member who names a path reaching a volume that requires the grant is refused with the reason,
// although /shared declares it; the manager is admitted and marked for the hostpath.
func TestAuthorizeWorkloadHostPathRefusesPrivilegedVolume(t *testing.T) {
	a := newPrivilegedVolumeController(t)
	w := pvWorkload(common.PytorchJobKind, "")
	w.Spec.Hostpath = []string{"/shared/team", "/shared/admin/keys"}
	err := a.AuthorizeWorkloadHostPath(AccessInput{Context: context.Background(), UserId: "pv-member"},
		w, privilegedVolumeWorkspace(true))
	require.True(t, apierrors.IsForbidden(err), "want 403, got %v", err)
	require.Contains(t, err.Error(), `the hostpath "/shared/admin/keys" reaches volume 3`)
	require.Contains(t, err.Error(), "requiresPrivilegedGrant")

	require.NoError(t, a.AuthorizeWorkloadHostPath(AccessInput{Context: context.Background(), UserId: "pv-manager"},
		w, privilegedVolumeWorkspace(true)))
	require.Equal(t, v1.TrueStr, v1.GetAnnotation(w, v1.WorkloadHostPathAuthzAnnotation))
}
