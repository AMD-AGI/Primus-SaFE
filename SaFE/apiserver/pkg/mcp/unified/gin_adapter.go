// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ToGinHandler converts a unified Handler to gin.HandlerFunc.
func ToGinHandler[Req, Resp any](h Handler[Req, Resp]) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req Req
		if err := BindGinRequest(c, &req); err != nil {
			_ = c.Error(err)
			return
		}
		resp, err := h(c.Request.Context(), &req)
		if err != nil {
			_ = c.Error(err)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// GetGinHandler returns the appropriate gin.HandlerFunc for the endpoint.
func (def *EndpointDef[Req, Resp]) GetGinHandler() gin.HandlerFunc {
	if def.MCPOnly {
		return nil
	}
	if def.RawHTTPHandler != nil {
		return gin.HandlerFunc(def.RawHTTPHandler)
	}
	if def.Handler != nil {
		return ToGinHandler(def.Handler)
	}
	return nil
}
