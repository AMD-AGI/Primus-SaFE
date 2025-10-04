/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	customhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers"
	sshhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
)

func InitHttpHandlers(_ context.Context, mgr ctrlruntime.Manager) (*gin.Engine, error) {
	engine := gin.New()
	engine.Use(apiutils.Logger(), gin.Recovery(), CorsMiddleware())
	engine.NoRoute(func(c *gin.Context) {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage(c.Request.RequestURI+" not found"))
	})

	customHandler, err := customhandler.NewHandler(mgr)
	if err != nil {
		return nil, err
	}
	customhandler.InitCustomRouters(engine, customHandler)
	return engine, nil
}

func InitSshHandlers(ctx context.Context, mgr ctrlruntime.Manager) (*sshhandler.SshHandler, error) {
	return sshhandler.NewSshHandler(ctx, mgr)
}

func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			referer := c.GetHeader("Referer")
			if referer != "" {
				origin = netutil.GetSchemeHost(referer)
			}
		}
		if origin != "" && origin != "*" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Authorization, Accept, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
