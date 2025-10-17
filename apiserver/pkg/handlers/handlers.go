/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package handlers

import (
	"context"

	"github.com/gin-gonic/gin"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	customhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers"
	image_handlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/image-handlers"
	sshhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// InitHttpHandlers: initializes the HTTP handlers for the API server.
// It creates a new Gin engine, sets up middleware including logging, recovery, and CORS,
// initializes custom API routes.
// Returns the configured Gin engine or an error if initialization fails.
func InitHttpHandlers(_ context.Context, mgr ctrlruntime.Manager) (*gin.Engine, error) {
	engine := gin.New()
	engine.Use(apiutils.Logger(), gin.Recovery())
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
