/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package handlers

import (
	"context"

	"github.com/gin-gonic/gin"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	cdhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/cd-handlers"
	datasethandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/dataset-handlers"
	imagehandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/image-handlers"
	model_handlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/model-handlers"
	proxyhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/proxy-handlers"
	reshandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources"
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
	// Initialize internal auth for service-to-service authentication
	if _, err := authority.NewInternalAuth(mgr.GetClient()); err != nil {
		return nil, commonerrors.NewInternalError("failed to initialize internal auth: " + err.Error())
	}
	// Initialize API key authentication if database is enabled
	if commonconfig.IsDBEnable() {
		dbClient := dbclient.NewClient()
		if dbClient != nil {
			authority.NewApiKeyToken(dbClient)
		}
	}
	customHandler, err := reshandler.NewHandler(mgr)
	if err != nil {
		return nil, err
	}
	reshandler.InitCustomRouters(engine, customHandler)
	cdHandler, err := cdhandlers.NewHandler(mgr)
	if err != nil {
		return nil, err
	}
	cdhandlers.InitCDRouters(engine, cdHandler)
	imageHandler, err := imagehandlers.NewImageHandler(mgr)
	if err != nil {
		return nil, err
	}
	imagehandlers.InitImageRouter(engine, imageHandler)
	sshHandler, err := InitSshHandlers(context.Background(), mgr)
	if err != nil {
		return nil, err
	}
	sshhandler.InitWebShellRouters(engine, sshHandler)
	modelHandler := InitModelHandlers(mgr)
	model_handlers.InitInferenceRouters(engine, modelHandler)

	// Initialize proxy handlers
	proxyHandler, err := proxyhandlers.NewProxyHandler()
	if err != nil {
		return nil, err
	}
	proxyhandlers.InitProxyRoutes(engine, proxyHandler)

	// Initialize dataset handlers
	datasetHandler, err := datasethandlers.NewHandler(context.Background(), mgr)
	if err != nil {
		return nil, err
	}
	if datasetHandler != nil {
		datasethandlers.InitDatasetRouters(engine, datasetHandler)
	}

	return engine, nil
}

// InitModelHandlers initializes the model handlers for the API server.
// It creates and returns a new model handler instance configured with the provided manager.
// If database is not enabled, dbClient will be nil and handlers will use K8s API only.
func InitModelHandlers(mgr ctrlruntime.Manager) *model_handlers.Handler {
	var dbClient dbclient.Interface
	if commonconfig.IsDBEnable() {
		dbClient = dbclient.NewClient()
	}
	accessController := authority.NewAccessController(mgr.GetClient())
	return model_handlers.NewHandler(mgr.GetClient(), dbClient, accessController)
}

// InitSshHandlers initializes the SSH handlers for the API server.
// It creates and returns a new SSH handler instance configured with the provided manager.
// Returns the SSH handler or an error if initialization fails.
func InitSshHandlers(ctx context.Context, mgr ctrlruntime.Manager) (*sshhandler.SshHandler, error) {
	return sshhandler.NewSshHandler(ctx, mgr)
}
