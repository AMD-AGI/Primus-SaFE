// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package middleware

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"net/http"
)

func HandleErrors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) <= 0 {
			return
		}
		ctx := c
		for i := range c.Errors {
			err := c.Errors[i]
			if i > 0 {
				log.GlobalLogger().WithContext(ctx).Errorf("error %v: %+v. This is a subsequent error in request. It should immediately return when the first error occurs", i, err.Error())
			}
		}

		err := c.Errors[0]
		if cError, ok := err.Err.(*errors.Error); ok {

			finalMeta := rest.Meta{
				Code:    cError.Code,
				Message: cError.Message,
			}
			log.GlobalLogger().WithContext(ctx).Errorf("Rest interface error FullPath %s RequestPath %s Code %d Message '%s' Error %+v Stack %v", c.FullPath(), c.Request.URL.Path, cError.Code, cError.Message, cError.InnerError, cError.GetStackString())
			c.AbortWithStatusJSON(http.StatusOK, rest.ErrorResp(ctx, finalMeta.Code, finalMeta.Message, nil))
			return
		} else if commonError, ok := err.Err.(*rest.Error); ok {
			log.GlobalLogger().WithContext(ctx).Errorf("Rest interface get tsp model error.FullPath %s. RequestPath %s. Error Code %d.Error Message %s. Inner error %+v.", c.FullPath(), c.Request.URL.Path, commonError.Code, commonError.Message, commonError)
			if commonError.OriginError == nil {
				c.AbortWithStatusJSON(http.StatusOK, rest.ErrorResp(ctx, commonError.Code, commonError.Message, nil))
				return
			}
			c.AbortWithStatusJSON(http.StatusOK, rest.ErrorResp(ctx, commonError.Code, commonError.OriginError.Error(), nil))
		} else {
			log.GlobalLogger().WithContext(ctx).Errorf("Rest interface get unwrapped error.FullPath %s. RequestPath %s. Error %+v.", c.FullPath(), c.Request.URL.Path, err)
			c.AbortWithStatusJSON(http.StatusOK, rest.ErrorResp(ctx, errors.InternalError, "Unknown error", nil))
			return
		}
	}
}
