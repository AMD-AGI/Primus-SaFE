/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// AuthorizeWorkloadPrivilegedVolumes decides whether the volumes of the workload's workspace that
// require the workload/privileged grant (requiresPrivilegedGrant) are mounted into the workload.
// When the workspace has such a volume for the workload's scope and the requesting user holds
// workload/privileged in the workload's workspace (built-in: workspace-admin and system-admin), the
// workload is marked with WorkloadPrivilegedVolumesAuthzAnnotation, which is what the dispatcher
// mounts those volumes on. A user without the grant is not refused: the workload is created and
// runs without those volumes, as it would in a scope they do not apply to. The mark is removed
// first, so a workload never carries it unless this check set it.
//
// in carries the request: Context, User or UserId, and Roles when already known. The resource,
// verb and workspace are set here.
func (a *AccessController) AuthorizeWorkloadPrivilegedVolumes(in AccessInput,
	workload *v1.Workload, workspace *v1.Workspace) error {
	v1.RemoveAnnotation(workload, v1.WorkloadPrivilegedVolumesAuthzAnnotation)
	if !commonworkload.HasPrivilegedVolume(workspace, commonworkload.GetScope(workload)) {
		return nil
	}
	in.ResourceKind = WorkloadPrivilegedKind
	in.ResourceName = workload.Name
	in.ResourceOwner = ""
	in.Resource = nil
	in.Verb = v1.CreateVerb
	in.Workspaces = []string{workload.Spec.Workspace}
	// Not holding the grant is not a refused request here, so Allowed rather than Authorize.
	allowed, err := a.Allowed(in)
	if err != nil || !allowed {
		return err
	}
	v1.SetAnnotation(workload, v1.WorkloadPrivilegedVolumesAuthzAnnotation, v1.TrueStr)
	return nil
}
