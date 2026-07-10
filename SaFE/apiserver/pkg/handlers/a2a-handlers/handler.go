/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2ahandlers

import (
	"github.com/gin-gonic/gin"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// a2aResourceKind is the resource kind used for A2A resource-level authorization.
const a2aResourceKind = "a2a"

// Handler holds dependencies for A2A API handlers.
type Handler struct {
	dbClient         dbclient.Interface
	accessController *authority.AccessController
}

// NewHandler creates a new A2A handler. The access controller is optional so
// existing callers and read-only tests can construct a handler without it, but
// write operations require it to enforce resource-level authorization.
func NewHandler(dbClient dbclient.Interface, accessController ...*authority.AccessController) *Handler {
	var ac *authority.AccessController
	if len(accessController) > 0 {
		ac = accessController[0]
	}
	return &Handler{dbClient: dbClient, accessController: ac}
}

// authorizeA2A enforces resource-level RBAC for A2A write operations.
func (h *Handler) authorizeA2A(c *gin.Context, verb v1.RoleVerb) error {
	if h.accessController == nil {
		return commonerrors.NewInternalError("A2A access controller is not initialized")
	}
	return h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: a2aResourceKind,
		Verb:         verb,
		UserId:       c.GetString(common.UserId),
	})
}