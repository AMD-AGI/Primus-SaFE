/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"strings"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/gin-gonic/gin"
)

// Prepare sets the trimmed value of the 'Name' parameter into the Gin context.
func Prepare(_ ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(common.Name, strings.TrimSpace(c.Param(common.Name)))
	}
}

// Authorize parses the cookie and aborts the request with an API error if parsing fails.
func Authorize(_ ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := ParseCookie(c)
		if err != nil {
			apiutils.AbortWithApiError(c, err)
		}
	}
}
