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
		SSOBridgeAllowlist:   commonconfig.GetSSOBridgeAllowlist(),
		CDRequireApproval:    commonconfig.IsCDRequireApproval(),
		AccessContact:        commonconfig.GetConsoleAccessContact(),
		KubeSprayK8sVersions: v1.KubeSprayK8sVersions(),
	}
	if commonconfig.GetAuthMode() == commonconfig.AuthModeExternal {
		// The console starts an authorize request at identity. connection names one identity source.
		// sso.enable selects it: okta-amd or local. sso.login, when set, is that authorize URL.
		resp.APIKeysURL = commonconfig.GetAuthAPIKeysURL()
		resp.LoginURL = commonconfig.GetAuthLoginURL()
		if resp.SSOEnable {
			resp.Connection = "okta-amd"
		} else {
			resp.Connection = "local"
		}
		if commonconfig.IsSSOLoginEnable() {
			if inst := authority.SSOInstance(); inst != nil {
				resp.SSOLoginUrl = inst.LoginAuthURL()
				resp.SSOPkce = inst.LoginEnabled()
			}
		}
	} else if resp.SSOEnable {
		if inst := authority.SSOInstance(); inst != nil {
			resp.SSOLoginUrl = inst.LoginAuthURL()
			resp.SSOPkce = inst.LoginEnabled()
		}
	}
	return resp, nil
}
