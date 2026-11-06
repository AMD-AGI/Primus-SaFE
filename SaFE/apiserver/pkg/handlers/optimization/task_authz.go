/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"github.com/gin-gonic/gin"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// A task runs as a workload in its workspace on behalf of the user who submitted it, so the
// routes that read or act on a task check the caller as they would for that workload: the
// workload's owner is the task's submitter and its workspace is the task's workspace. With the
// built-in roles, the submitter, an administrator of the task's workspace and a system
// administrator may do everything; a member of the task's workspace and a read-only system
// administrator may read it; nobody else may see it.
//
// Each route uses the verb the workload route that does the same thing uses:
//
//	read the task, its events or artifacts, or apply it   get     (GET /workloads/:name)
//	interrupt it                                          delete  (POST /workloads/:name/stop)
//	retry it                                              resume  (POST /workloads/:name/resume)
//	delete it                                             delete  (DELETE /workloads/:name)
//	list tasks                                            list    (GET /workloads)

const accessControllerMissing = "the access controller is not initialized"

// authorizeTask checks that the request user may perform verb on the task.
func (h *Handler) authorizeTask(c *gin.Context, task *dbclient.OptimizationTask, verb v1.RoleVerb) error {
	if h.accessCtrl == nil {
		return commonerrors.NewInternalError(accessControllerMissing)
	}
	var workspaces []string
	if task.Workspace != "" {
		workspaces = []string{task.Workspace}
	}
	return h.accessCtrl.Authorize(authority.AccessInput{
		Context:       c.Request.Context(),
		ResourceKind:  v1.WorkloadKind,
		ResourceOwner: task.UserID,
		ResourceName:  task.ID,
		Verb:          verb,
		Workspaces:    workspaces,
		UserId:        c.GetString(common.UserId),
	})
}

// taskListScope returns the tasks the request user may list: nil (every task) when the user's
// roles grant list on any workload, otherwise the user's own tasks, if the roles grant that, and
// the tasks of each workspace the user belongs to or manages in which they grant it.
func (h *Handler) taskListScope(c *gin.Context) (*dbclient.OptimizationTaskScope, error) {
	if h.accessCtrl == nil {
		return nil, commonerrors.NewInternalError(accessControllerMissing)
	}
	ctx := c.Request.Context()
	user, err := h.accessCtrl.GetRequestUser(ctx, c.GetString(common.UserId))
	if err != nil {
		return nil, err
	}
	roles := h.accessCtrl.GetRoles(ctx, user)
	allowed := func(owner string, workspaces ...string) (bool, error) {
		return h.accessCtrl.Allowed(authority.AccessInput{
			Context:       ctx,
			ResourceKind:  v1.WorkloadKind,
			ResourceOwner: owner,
			Verb:          v1.ListVerb,
			Workspaces:    workspaces,
			User:          user,
			Roles:         roles,
		})
	}

	everyTask, err := allowed("")
	if err != nil {
		return nil, err
	}
	if everyTask {
		return nil, nil
	}
	scope := &dbclient.OptimizationTaskScope{}
	ok, err := allowed(user.Name)
	if err != nil {
		return nil, err
	}
	if ok {
		scope.UserID = user.Name
	}
	seen := map[string]bool{}
	for _, ws := range append(commonuser.GetWorkspace(user), commonuser.GetManagedWorkspace(user)...) {
		if ws == "" || seen[ws] {
			continue
		}
		seen[ws] = true
		ok, err := allowed("", ws)
		if err != nil {
			return nil, err
		}
		if ok {
			scope.Workspaces = append(scope.Workspaces, ws)
		}
	}
	return scope, nil
}
