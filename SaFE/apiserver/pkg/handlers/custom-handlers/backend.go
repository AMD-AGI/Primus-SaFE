/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// GetEnvs retrieves environment configuration values for the system.
func (h *Handler) GetEnvs(c *gin.Context) {
	handle(c, h.getEnvs)
}

// getEnvs lists the environment variables supported by the backend.
func (h *Handler) getEnvs(_ *gin.Context) (interface{}, error) {
	resp := types.GetEnvResponse{
		EnableLog:         commonconfig.IsOpenSearchEnable(),
		EnableLogDownload: commonconfig.IsS3Enable(),
		EnableSSH:         commonconfig.IsSSHEnable(),
		SSHIP:             commonconfig.GetSSHServerIP(),
		SSHPort:           commonconfig.GetSSHServerPort(),
		SSOEnable:         commonconfig.IsSSOEnable(),
	}
	if resp.SSOEnable {
		inst := authority.SSOInstance()
		if inst != nil {
			resp.SSOAuthUrl = inst.AuthURL()
		}
	}
	return resp, nil
}
