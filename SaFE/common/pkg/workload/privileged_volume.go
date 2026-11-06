/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"fmt"
	"path/filepath"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// WorkloadPrivilegedGrant names, in messages, the grant the volumes with requiresPrivilegedGrant
// take: the workload/privileged resource of the workspace's roles.
const WorkloadPrivilegedGrant = "workload/privileged"

// HasPrivilegedVolumesGrant reports whether the apiserver found, when the workload was created,
// that its creator held the workload/privileged grant in the workspace, so that the volumes of the
// workspace with requiresPrivilegedGrant are mounted into it.
func HasPrivilegedVolumesGrant(workload *v1.Workload) bool {
	return workload != nil &&
		v1.GetAnnotation(workload, v1.WorkloadPrivilegedVolumesAuthzAnnotation) == v1.TrueStr
}

// WorkspaceVolumeMountedInto reports whether a volume of the workload's workspace is mounted into
// the workload: the volume applies to the workload's scope and, when it requires the
// workload/privileged grant, the workload carries the mark the apiserver sets for that grant.
func WorkspaceVolumeMountedInto(vol *v1.WorkspaceVolume, workload *v1.Workload) bool {
	if !vol.AppliesToScope(GetScope(workload)) {
		return false
	}
	return !vol.RequiresPrivilegedGrant || HasPrivilegedVolumesGrant(workload)
}

// HasPrivilegedVolume reports whether the workspace has a volume that requires the
// workload/privileged grant and applies to workloads of the given scope.
func HasPrivilegedVolume(workspace *v1.Workspace, scope v1.WorkspaceScope) bool {
	if workspace == nil {
		return false
	}
	for i := range workspace.Spec.Volumes {
		if vol := &workspace.Spec.Volumes[i]; vol.RequiresPrivilegedGrant && vol.AppliesToScope(scope) {
			return true
		}
	}
	return false
}

// MayMountWorkspaceStorage reports whether the workload may mount anything its workspace's volumes
// decide on: it names hostpath entries, or it has not turned workspace storage off (the admission
// webhook turns it on when the request leaves it unset).
func MayMountWorkspaceStorage(workload *v1.Workload) bool {
	return len(workload.Spec.Hostpath) > 0 ||
		!v1.HasAnnotation(workload, v1.UseWorkspaceStorageAnnotation) || v1.IsEnableWorkspaceStorage(workload)
}

// PrivilegedVolumeOverlapping returns the hostpath volume of the workspace with
// requiresPrivilegedGrant whose hostPath the hostpath entry equals, lies under or contains, or nil
// when there is none. Scopes do not matter: such an entry reaches what only the holders of the
// workload/privileged grant may mount, whatever volume declares it, so it counts as undeclared
// (UndeclaredHostPaths). A relative entry or one with a ".." element is undeclared anyway and
// returns nil.
func PrivilegedVolumeOverlapping(workspace *v1.Workspace, hostpath string) *v1.WorkspaceVolume {
	if workspace == nil || !filepath.IsAbs(hostpath) || hasDotDot(hostpath) {
		return nil
	}
	cleaned := filepath.Clean(hostpath)
	for i := range workspace.Spec.Volumes {
		vol := &workspace.Spec.Volumes[i]
		if vol.Type != v1.HOSTPATH || !vol.RequiresPrivilegedGrant || !filepath.IsAbs(vol.HostPath) ||
			hasDotDot(vol.HostPath) {
			continue
		}
		root := filepath.Clean(vol.HostPath)
		if _, ok := below(root, cleaned); ok {
			return vol
		}
		if _, ok := below(cleaned, root); ok {
			return vol
		}
	}
	return nil
}

// PrivilegedVolumeRefusal returns why a creator without the workload/privileged grant may not name
// the first of hostpaths that reaches a volume with requiresPrivilegedGrant
// (PrivilegedVolumeOverlapping), or "" when none does. grant names the grant for the message.
func PrivilegedVolumeRefusal(workspace *v1.Workspace, hostpaths []string, grant string) string {
	for _, hostpath := range hostpaths {
		if vol := PrivilegedVolumeOverlapping(workspace, hostpath); vol != nil {
			return fmt.Sprintf("the hostpath %q reaches volume %d (hostPath %s) of workspace %s, which "+
				"requiresPrivilegedGrant: it is mounted only into workloads whose creator holds the %s "+
				"grant in the workspace (workspace-admin, system-admin), and mounting it requires the %s grant "+
				"in the workspace", hostpath, vol.Id, vol.HostPath, workspace.Name, grant, grant)
		}
	}
	return ""
}
