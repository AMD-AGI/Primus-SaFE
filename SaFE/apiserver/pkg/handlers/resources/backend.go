/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"github.com/gin-gonic/gin"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// GetEnvs retrieves environment configuration values for the system.
func (h *Handler) GetEnvs(c *gin.Context) {
	handle(c, h.getEnvs)
}

// getEnvs lists the environment variables supported by the backend.
func (h *Handler) getEnvs(_ *gin.Context) (interface{}, error) {
	resp := view.GetEnvResponse{
		EnableLog:            commonconfig.IsOpenSearchEnable(),
		EnableLogDownload:    commonconfig.IsS3Enable(),
		EnableSSH:            commonconfig.IsSSHEnable(),
		SSHIP:                commonconfig.GetSSHServerIP(),
		SSHPort:              commonconfig.GetSSHServerPort(),
		AuthMode:             commonconfig.GetAuthMode(),
		SSOEnable:            commonconfig.IsSSOEnable(),
		CDRequireApproval:    commonconfig.IsCDRequireApproval(),
		KubeSprayK8sVersions: v1.KubeSprayK8sVersions(),
	}
	if commonconfig.GetAuthMode() == commonconfig.AuthModeExternal {
		// POST /login is refused whoever asks, so the built-in sign-in (the password form, the
		// authorize URL of the built-in SSO) is not advertised: a console sent there would end
		// on a 403. The sign-in page is wherever the deployment says, if it says.
		resp.SSOEnable, resp.SSOAuthUrl = false, ""
		if url := commonconfig.GetAuthLoginURL(); url != "" {
			resp.SSOEnable, resp.SSOAuthUrl = true, url
		}
	} else if resp.SSOEnable {
		if inst := authority.SSOInstance(); inst != nil {
			resp.SSOAuthUrl = inst.AuthURL()
		}
	}
	return resp, nil
}
