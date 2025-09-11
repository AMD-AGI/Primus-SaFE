/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func (h *Handler) ListFeatures(c *gin.Context) {
	handle(c, h.listFeatures)
}

// List the features supported by the backend
func (h *Handler) listFeatures(_ *gin.Context) (interface{}, error) {
	return types.ListFeaturesResponse{
		EnableLog:         commonconfig.IsLogEnable(),
		EnableLogDownload: commonconfig.IsS3Enable(),
		EnableSSH:         commonconfig.IsSSHEnable(),
	}, nil
}
