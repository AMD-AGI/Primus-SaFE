/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middle"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitWebShellRouters initializes the web shell related routes.
func InitWebShellRouters(e *gin.Engine, h *SshHandler) {
	group := e.Group(common.PrimusRouterCustomRootPath, middle.Authorize(), middle.Preprocess())
	{
		group.GET(fmt.Sprintf("workloads/:%s/pods/:%s/webshell", common.Name, common.PodId), h.WebShell)
	}
}
