/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"github.com/gin-gonic/gin"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// authorizeWorkloadCreate holds the caller to the workload create path's checks for a workload
// this package builds and creates itself (authority.AccessController.AuthorizeWorkloadCreate). It
// fails closed: without an access controller nothing is created.
func (h *Handler) authorizeWorkloadCreate(c *gin.Context, workload *v1.Workload) error {
	if h.accessController == nil {
		return commonerrors.NewInternalError("workload access controller is not initialized")
	}
	return h.accessController.AuthorizeWorkloadCreate(c.Request.Context(), c.GetString(common.UserId), workload)
}
