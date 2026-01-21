// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ToGinHandler converts a unified Handler to gin.HandlerFunc.
// It handles request binding, handler execution, and response formatting.
func ToGinHandler[Req, Resp any](h Handler[Req, Resp]) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req Req

		// Parse request based on struct tags
		if err := BindGinRequest(c, &req); err != nil {
			_ = c.Error(err)
			return
		}

		// Execute handler with request context
		resp, err := h(c.Request.Context(), &req)
		if err != nil {
			_ = c.Error(err)
			return
		}

		// Return success response
		c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), resp))
	}
}

// GetGinHandler returns the appropriate gin.HandlerFunc for the endpoint.
// Priority: RawHTTPHandler > Handler
func (def *EndpointDef[Req, Resp]) GetGinHandler() gin.HandlerFunc {
	// MCPOnly endpoints should not have HTTP handlers
	if def.MCPOnly {
		return nil
	}

	// Priority: RawHTTPHandler > Handler
	if def.RawHTTPHandler != nil {
		return gin.HandlerFunc(def.RawHTTPHandler)
	}

	if def.Handler != nil {
		return ToGinHandler(def.Handler)
	}

	return nil
}
