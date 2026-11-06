/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// AuthorizeWorkloadHostPath decides whether the requesting user may create a workload with its
// hostpath entries. An entry that is a hostpath volume the workspace declares for the workload's
// scope, or a directory under one, needs nothing more. Any other entry mounts a node path nobody
// declared for the workspace, which is as much as running privileged, so it needs the
// workload/privileged grant in the workload's workspace (built-in: workspace-admin and
// system-admin). When that grant is what admits the workload, the workload is marked with
// WorkloadHostPathAuthzAnnotation, which is how the admission webhook knows; the mark is removed
// first, so a workload never carries it unless this check set it. Sandboxes are left to the
// webhook, which admits only declared paths for them whatever the grant.
//
// in carries the request: Context, User or UserId, and Roles when already known. The resource,
// verb and workspace are set here.
func (a *AccessController) AuthorizeWorkloadHostPath(in AccessInput, workload *v1.Workload, workspace *v1.Workspace) error {
	v1.RemoveAnnotation(workload, v1.WorkloadHostPathAuthzAnnotation)
	if len(workload.Spec.Hostpath) == 0 || commonworkload.IsSandBox(workload) {
		return nil
	}
	undeclared := commonworkload.UndeclaredHostPaths(workspace,
		commonworkload.GetScope(workload), workload.Spec.Hostpath)
	if len(undeclared) == 0 {
		return nil
	}
	in.ResourceKind = WorkloadPrivilegedKind
	in.ResourceName = workload.Name
	in.ResourceOwner = ""
	in.Resource = nil
	in.Verb = v1.CreateVerb
	in.Workspaces = []string{workload.Spec.Workspace}
	if err := a.Authorize(in); err != nil {
		if apierrors.IsForbidden(err) {
			return commonerrors.NewForbidden(fmt.Sprintf("the hostpath %v is not declared by workspace %s "+
				"for %s workloads; mounting it requires the %s grant in the workspace",
				undeclared, workload.Spec.Workspace, workload.SpecKind(), WorkloadPrivilegedKind))
		}
		return err
	}
	v1.SetAnnotation(workload, v1.WorkloadHostPathAuthzAnnotation, v1.TrueStr)
	return nil
}
