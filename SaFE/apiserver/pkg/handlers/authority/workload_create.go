/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"fmt"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// AuthorizeWorkloadCreate makes, for a workload that a handler builds and creates with its own
// client, the checks the workload create path (POST /api/v1/workloads) makes before it creates
// one: the user must be allowed to create workloads in the workload's workspace (a member, the
// workspace's manager or a system administrator), to create them at the workload's priority, and,
// when the workload is privileged, to create privileged workloads there. Pass the workload as it
// will be stored, so that defaults the handler filled in are what is authorized.
func (a *AccessController) AuthorizeWorkloadCreate(ctx context.Context, userId string, workload *v1.Workload) error {
	user, err := a.GetRequestUser(ctx, userId)
	if err != nil {
		return err
	}
	roles := a.GetRoles(ctx, user)
	kinds := []string{
		v1.WorkloadKind,
		fmt.Sprintf("workload/%s", commonworkload.GeneratePriority(workload.Spec.Priority)),
	}
	if v1.IsPrivileged(workload) {
		kinds = append(kinds, WorkloadPrivilegedKind)
	}
	var workspaces []string
	if workload.Spec.Workspace != "" {
		workspaces = []string{workload.Spec.Workspace}
	}
	for _, kind := range kinds {
		if err = a.Authorize(AccessInput{
			Context:      ctx,
			ResourceKind: kind,
			ResourceName: workload.Name,
			Verb:         v1.CreateVerb,
			Workspaces:   workspaces,
			User:         user,
			Roles:        roles,
		}); err != nil {
			return err
		}
	}
	return nil
}
