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

// robustAddonDisplayName is the AddonTemplate label value that identifies Robust.
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
		// POST /login is refused whoever asks, so the built-in sign-in (the password form, the
		// authorize URL of the built-in SSO) is not advertised: a console sent there would end
		// on a 403. The sign-in page is wherever the deployment says, if it says.
		resp.SSOEnable, resp.SSOAuthUrl = false, ""
		// The apiserver refuses to issue API keys here. The identity provider does, at the path the
		// deployment routes to it; the console uses that path only when it is told one.
		resp.APIKeysURL = commonconfig.GetAuthAPIKeysURL()
		if url := commonconfig.GetAuthLoginURL(); url != "" {
			resp.SSOEnable, resp.SSOAuthUrl = true, url
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

// robustInstalled reports whether an AddonTemplate labeled for Robust is present.
// A lookup that cannot be made is reported as not installed.
func (h *Handler) robustInstalled(ctx context.Context) bool {
	if h.Client == nil {
		return false
	}
	list := &v1.AddonTemplateList{}
	if err := h.List(ctx, list, client.MatchingLabels{v1.DisplayNameLabel: robustAddonDisplayName}); err != nil {
		klog.Errorf("list AddonTemplates with %s=%s: %v", v1.DisplayNameLabel, robustAddonDisplayName, err)
		return false
	}
	return len(list.Items) > 0
}
