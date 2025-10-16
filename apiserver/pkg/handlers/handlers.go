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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	customhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers"
	image_handlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/image-handlers"
	sshhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
)

// InitHttpHandlers: initializes the HTTP handlers for the API server.
// It creates a new Gin engine, sets up middleware including logging, recovery, and CORS,
// initializes custom API routes.
// Returns the configured Gin engine or an error if initialization fails.
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
	imageHanlder, err := image_handlers.NewImageHandler(mgr)
	if err != nil {
		return nil, err
	}
	image_handlers.InitImageRouter(engine, imageHanlder)
	sshHandler, err := InitSshHandlers(context.Background(), mgr)
	if err != nil {
		return nil, err
	}
	sshhandler.InitWebShellRouters(engine, sshHandler)
	return engine, nil
}

// InitSshHandlers: initializes the SSH handlers for the API server.
// It creates and returns a new SSH handler instance configured with the provided manager.
// Returns the SSH handler or an error if initialization fails.
func InitSshHandlers(ctx context.Context, mgr ctrlruntime.Manager) (*sshhandler.SshHandler, error) {
	return sshhandler.NewSshHandler(ctx, mgr)
}

// CorsMiddleware: provides Cross-Origin Resource Sharing (CORS) support for the API.
// It sets appropriate CORS headers based on the request origin.
// For OPTIONS requests, it returns an error with httpcode 204
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
			c.Writer.Header().Set("Access-Control-Allow-Credentials", v1.TrueStr)
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+
			"Content-Length, Authorization, Accept, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
