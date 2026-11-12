/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// robustAddonDisplayName is the Addon label value that identifies Robust.
const robustAddonDisplayName = "primus-robust"

// GetEnvs retrieves environment configuration values for the system.
func (h *Handler) GetEnvs(c *gin.Context) {
	handle(c, h.getEnvs)
}

// getEnvs lists the environment variables supported by the backend.
func (h *Handler) getEnvs(c *gin.Context) (interface{}, error) {
	resp := view.GetEnvResponse{
		EnableRobust:         h.robustInstalled(c.Request.Context()),
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
		// The console starts an authorize request at identity. connection, accountsUrl and keysUrl
		// come from the deployment. sso.login, when set, is that authorize URL.
		resp.APIKeysURL = commonconfig.GetAuthAPIKeysURL()
		resp.LoginURL = commonconfig.GetAuthLoginURL()
		resp.Connection = commonconfig.GetAuthConnection()
		resp.AccountsURL = commonconfig.GetAuthAccountsURL()
		resp.KeysURL = commonconfig.GetAuthKeysConsoleURL()
		if commonconfig.IsSSOLoginEnable() {
			if inst := authority.SSOInstance(); inst != nil {
				resp.SSOLoginUrl = inst.LoginAuthURL()
				resp.SSOPkce = inst.LoginEnabled()
			}
		}
	} else if resp.SSOEnable {
		if inst := authority.SSOInstance(); inst != nil {
			resp.SSOAuthUrl = inst.AuthURL()
			resp.SSOLoginUrl = inst.LoginAuthURL()
			resp.SSOPkce = inst.LoginEnabled()
		}
	}
	return resp, nil
}

// robustInstalled reports whether an Addon labeled for Robust is present.
// A lookup that cannot be made is reported as not installed.
func (h *Handler) robustInstalled(ctx context.Context) bool {
	if h.Client == nil {
		return false
	}
	list := &v1.AddonList{}
	if err := h.List(ctx, list, client.MatchingLabels{v1.DisplayNameLabel: robustAddonDisplayName}); err != nil {
		klog.Errorf("list Addons with %s=%s: %v", v1.DisplayNameLabel, robustAddonDisplayName, err)
		return false
	}
	return len(list.Items) > 0
}
