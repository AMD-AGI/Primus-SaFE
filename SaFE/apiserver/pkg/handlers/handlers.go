/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package handlers

import (
	"context"

	"github.com/gin-gonic/gin"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	customhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers"
	image_handlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/image-handlers"
	inference_handlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/inference-handlers"
	sshhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// InitHttpHandlers initializes the HTTP handlers for the API server.
// It creates a new Gin engine, sets up middleware including logging, recovery, and CORS,
// initializes custom API routes.
// Returns the configured Gin engine or an error if initialization fails.
func InitHttpHandlers(_ context.Context, mgr ctrlruntime.Manager) (*gin.Engine, error) {
	engine := gin.New()
	engine.Use(apiutils.Logger(), gin.Recovery())
	engine.NoRoute(func(c *gin.Context) {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage(c.Request.RequestURI+" not found"))
	})
	if commonconfig.IsSSOEnable() {
		if authority.NewSSOToken(mgr.GetClient()) == nil {
			return nil, commonerrors.NewInternalError("failed to new sso token")
		}
	}
	if authority.NewDefaultToken(mgr.GetClient()) == nil {
		return nil, commonerrors.NewInternalError("failed to new default token")
	}
	customHandler, err := customhandler.NewHandler(mgr)
	if err != nil {
		return nil, err
	}
	customhandler.InitCustomRouters(engine, customHandler)
	imageHandler, err := image_handlers.NewImageHandler(mgr)
	if err != nil {
		return nil, err
	}
	image_handlers.InitImageRouter(engine, imageHandler)
	sshHandler, err := InitSshHandlers(context.Background(), mgr)
	if err != nil {
		return nil, err
	}
	sshhandler.InitWebShellRouters(engine, sshHandler)

	// Initialize inference and playground handlers
	if commonconfig.IsDBEnable() {
		inferenceHandler := InitInferenceHandlers(mgr)
		inference_handlers.InitInferenceRouters(engine, inferenceHandler)
	}

	return engine, nil
}

// InitInferenceHandlers initializes the inference handlers for the API server.
// It creates and returns a new inference handler instance configured with the provided manager.
func InitInferenceHandlers(mgr ctrlruntime.Manager) *inference_handlers.Handler {
	dbClient := dbclient.NewClient()
	accessController := authority.NewAccessController(mgr.GetClient())
	return inference_handlers.NewHandler(mgr.GetClient(), dbClient, accessController)
}

// InitSshHandlers initializes the SSH handlers for the API server.
// It creates and returns a new SSH handler instance configured with the provided manager.
// Returns the SSH handler or an error if initialization fails.
func InitSshHandlers(ctx context.Context, mgr ctrlruntime.Manager) (*sshhandler.SshHandler, error) {
	return sshhandler.NewSshHandler(ctx, mgr)
}
