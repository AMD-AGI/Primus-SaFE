/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// privilegedWorkspace declares /shared for every scope and keeps /shared/admin, a hostpath volume
// for training, and a PFS volume for every scope but Sandbox, for the holders of the grant.
func privilegedWorkspace() *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, HostPath: "/shared", MountPath: "/shared"},
			{Id: 2, Type: v1.HOSTPATH, HostPath: "/shared/admin", MountPath: "/admin",
				Scopes: []v1.WorkspaceScope{v1.TrainScope}, RequiresPrivilegedGrant: true},
			{Id: 3, Type: v1.PFS, MountPath: "/mnt/ceph", RequiresPrivilegedGrant: true,
				Scopes: []v1.WorkspaceScope{v1.AuthoringScope, v1.TrainScope}},
		}},
	}
}

// An entry that equals, lies under or contains the hostPath of a volume that requires the grant
// is undeclared even though another volume declares it, whatever the workload's scope; entries
// beside it stay declared.
func TestPrivilegedVolumeHostPathsAreUndeclared(t *testing.T) {
	ws := privilegedWorkspace()
	for _, scope := range []v1.WorkspaceScope{v1.TrainScope, v1.AuthoringScope} {
		got := UndeclaredHostPaths(ws, scope, []string{
			"/shared/admin", "/shared/admin/keys", "/shared/admin/", "/shared", "/",
			"/shared/team", "/shared/administrator",
		})
		require.Equal(t, []string{"/shared/admin", "/shared/admin/keys", "/shared/admin/", "/shared", "/"}, got,
			"scope %s", scope)
	}
	for _, p := range []string{"/shared/admin", "/shared", "/shared/admin/x"} {
		require.NotNil(t, PrivilegedVolumeOverlapping(ws, p), p)
	}
	for _, p := range []string{"/shared/team", "/shared/administrator", "shared/admin", "/shared/x/../admin", "/mnt/ceph"} {
		require.Nil(t, PrivilegedVolumeOverlapping(ws, p), p)
	}
	require.Nil(t, PrivilegedVolumeOverlapping(nil, "/shared"))

	// Without the option the same entries are declared as before.
	ws.Spec.Volumes[1].RequiresPrivilegedGrant = false
	require.Empty(t, UndeclaredHostPaths(ws, v1.AuthoringScope, []string{"/shared/admin/keys", "/shared"}))
}

// The refusal names the entry, the volume and the grant.
func TestPrivilegedVolumeRefusal(t *testing.T) {
	ws := privilegedWorkspace()
	reason := PrivilegedVolumeRefusal(ws, []string{"/etc", "/shared/admin/keys"}, "workload/privileged")
	for _, want := range []string{`"/shared/admin/keys"`, "volume 2", "requiresPrivilegedGrant",
		"workspace ws", "requires the workload/privileged grant"} {
		require.True(t, strings.Contains(reason, want), "%q lacks %q", reason, want)
	}
	require.Empty(t, PrivilegedVolumeRefusal(ws, []string{"/etc", "/shared/team"}, "workload/privileged"))
}

// A volume that requires the grant is mounted only into workloads of its scopes that carry the
// apiserver's mark with the value "true"; other volumes ignore the mark.
func TestWorkspaceVolumeMountedInto(t *testing.T) {
	ws := privilegedWorkspace()
	workload := func(kind, mark string) *v1.Workload {
		w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
		w.Spec.GroupVersionKind.Kind = kind
		if mark != "" {
			w.Annotations[v1.WorkloadPrivilegedVolumesAuthzAnnotation] = mark
		}
		return w
	}
	pfs := &ws.Spec.Volumes[2]
	shared := &ws.Spec.Volumes[0]
	require.False(t, WorkspaceVolumeMountedInto(pfs, workload(common.AuthoringKind, "")))
	require.False(t, WorkspaceVolumeMountedInto(pfs, workload(common.AuthoringKind, "True ")))
	require.True(t, WorkspaceVolumeMountedInto(pfs, workload(common.AuthoringKind, v1.TrueStr)))
	require.False(t, WorkspaceVolumeMountedInto(pfs, workload(common.SandboxKind, v1.TrueStr)))
	require.True(t, WorkspaceVolumeMountedInto(shared, workload(common.SandboxKind, "")))

	require.True(t, HasPrivilegedVolume(ws, v1.AuthoringScope))
	require.False(t, HasPrivilegedVolume(ws, v1.SandboxScope))
	require.False(t, HasPrivilegedVolume(nil, v1.AuthoringScope))
}

// The workspace's volumes matter to a workload that names hostpath entries or has not turned
// workspace storage off.
func TestMayMountWorkspaceStorage(t *testing.T) {
	w := &v1.Workload{}
	require.True(t, MayMountWorkspaceStorage(w))
	v1.SetAnnotation(w, v1.UseWorkspaceStorageAnnotation, v1.TrueStr)
	require.True(t, MayMountWorkspaceStorage(w))
	v1.SetAnnotation(w, v1.UseWorkspaceStorageAnnotation, "false")
	require.False(t, MayMountWorkspaceStorage(w))
	w.Spec.Hostpath = []string{"/shared"}
	require.True(t, MayMountWorkspaceStorage(w))
}
