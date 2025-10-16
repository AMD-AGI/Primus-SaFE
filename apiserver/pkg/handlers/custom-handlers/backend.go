/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func (h *Handler) GetEnvs(c *gin.Context) {
	handle(c, h.getEnvs)
}

// List the environment variables supported by the backend
func (h *Handler) getEnvs(_ *gin.Context) (interface{}, error) {
	return types.GetEnvResponse{
		EnableLog:         commonconfig.IsOpenSearchEnable(),
		EnableLogDownload: commonconfig.IsS3Enable(),
		EnableSSH:         commonconfig.IsSSHEnable(),
		AuthoringImage:    commonconfig.GetAuthoringImage(),
		SSHPort:           commonconfig.GetSSHServerPort(),
	}, nil
}
