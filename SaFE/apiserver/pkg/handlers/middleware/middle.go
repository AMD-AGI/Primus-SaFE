/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// Preprocess sets the trimmed value of the 'Name' parameter into the Gin context.
func Preprocess(_ ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(common.Name, strings.TrimSpace(c.Param(common.Name)))
	}
}

// Authorize parses the cookie and aborts the request with an API error if parsing fails.
func Authorize(_ ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := authority.ParseToken(c)
		if err != nil {
			apiutils.AbortWithApiError(c, err)
			return
		}
		// Writes carry the Audit middleware; reads made with a delegated token are
		// recorded here so that a party acting for a user always leaves a trace.
		if authority.IsDelegatedRequest(c) && c.Request.Method == http.MethodGet {
			start := time.Now()
			c.Next()
			auditDelegatedRead(c, start)
		}
	}
}
