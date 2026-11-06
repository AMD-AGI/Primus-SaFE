/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// authorizeWorkloadCreate applies the authorization of the workload create path
// (POST /api/v1/workloads) to a workload this package builds and creates itself: the
// caller must be allowed to create workloads in the workload's workspace (a member,
// the workspace's manager or a system administrator), at the workload's priority, and
// with the workload/privileged grant when the workload is privileged. The workload is
// checked as it will be stored, so defaults filled in by the handler are what is
// authorized. It fails closed: without an access controller nothing is created.
func (h *Handler) authorizeWorkloadCreate(c *gin.Context, workload *v1.Workload) error {
	if h.accessController == nil {
		return commonerrors.NewInternalError("workload access controller is not initialized")
	}
	ctx := c.Request.Context()
	user, err := h.accessController.GetRequestUser(ctx, c.GetString(common.UserId))
	if err != nil {
		return err
	}
	roles := h.accessController.GetRoles(ctx, user)
	kinds := []string{
		v1.WorkloadKind,
		fmt.Sprintf("workload/%s", commonworkload.GeneratePriority(workload.Spec.Priority)),
	}
	if v1.IsPrivileged(workload) {
		kinds = append(kinds, authority.WorkloadPrivilegedKind)
	}
	var workspaces []string
	if workload.Spec.Workspace != "" {
		workspaces = []string{workload.Spec.Workspace}
	}
	for _, kind := range kinds {
		if err = h.accessController.Authorize(authority.AccessInput{
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
