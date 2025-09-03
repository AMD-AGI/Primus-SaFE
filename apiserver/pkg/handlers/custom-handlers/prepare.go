/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
)

func Prepare(_ ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(types.Name, strings.TrimSpace(c.Param(types.Name)))
		err := authority.ParseCookie(c)
		if err != nil {
			apiutils.AbortWithApiError(c, err)
		}
	}
}
